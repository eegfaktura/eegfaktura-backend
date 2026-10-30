package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type revokeSent struct {
	meter, participant string
	reason             *string
}

// revoke posts a revoke for pid in TE001006 (online) and records what goes to eda-xp.
func revoke(t *testing.T, pid string, body string) (*httptest.ResponseRecorder, []revokeSent) {
	t.Helper()
	var sent []revokeSent
	orig := mqttclient.RevokeMeteringPoint
	t.Cleanup(func() { mqttclient.RevokeMeteringPoint = orig })
	mqttclient.RevokeMeteringPoint = func(eeg *model.Eeg, meter *model.MeteringPoint, consentEnd int64, reason *string) error {
		sent = append(sent, revokeSent{meter: meter.MeteringPoint, participant: meter.ParticipantId, reason: reason})
		return nil
	}
	req, _ := http.NewRequest("POST", "/meteringpoint/"+pid+"/revokemeters", strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"pid": pid})
	w := httptest.NewRecorder()
	newTestMeteringHandler(t).requestRevokeMeteringPoint()(w, req, &middleware.PlatformClaims{Username: "test"}, "TE001006")
	return w, sent
}

func revokeRequest(meter string, reason string) string {
	return fmt.Sprintf(`{"meteringPoints":[{"meter":%q,"direction":"CONSUMPTION"}],"from":1793574000000,"reason":%q}`,
		meter, reason)
}

const notOwnBody = `{"error":{"code":5012,"error":"Mindestens ein Zählpunkt gehört nicht zu diesem Mitglied.",
	"message":"Mindestens ein Zählpunkt gehört nicht zu diesem Mitglied."}}`

// The member's own active metering point is revoked with the reason.
func TestRevokeOwnMeter(t *testing.T) {
	const zp = "AT0030000000000000000000000953101"
	ownerChange(t, zp, "2025-01-01", "2025-06-15", "2025-06-16", 0)
	newOwner := "c1050001-0000-0000-0000-" + zp[len(zp)-12:]

	w, sent := revoke(t, newOwner, revokeRequest(zp, "Auszug"))

	assert.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, []revokeSent{{meter: zp, participant: newOwner, reason: revokeReasonOf("Auszug")}}, sent)
}

// A metering point of another member and an unknown one are refused alike; nothing is sent.
func TestRevokeRefusesMeterOfAnotherMember(t *testing.T) {
	const own, foreign = "AT0030000000000000000000000953102", "AT0030000000000000000000000953103"
	ownerChange(t, own, "2025-01-01", "2025-06-15", "2025-06-16", 0)
	ownerChange(t, foreign, "2025-01-01", "2025-06-15", "2025-06-16", 0)
	member := "c1050001-0000-0000-0000-" + own[len(own)-12:]

	for _, meter := range []string{foreign, "AT0030000000000000000000000953199"} {
		w, sent := revoke(t, member, revokeRequest(meter, ""))

		assert.Equal(t, http.StatusBadRequest, w.Code, meter)
		assert.JSONEq(t, notOwnBody, w.Body.String(), meter)
		assert.Empty(t, sent, meter)
	}
}

const notActiveBody = `{"error":{"code":5013,"error":"Keiner der Zählpunkte hat eine aktive Datenfreigabe dieses Mitglieds.",
	"message":"Keiner der Zählpunkte hat eine aktive Datenfreigabe dieses Mitglieds."}}`

// After an owner change the old member's entry is its own, but the active entry is the new member's: the old member
// cannot revoke the new member's consent. Nothing is sent, and the answer says so instead of a success.
func TestRevokeByOldOwnerIsRefused(t *testing.T) {
	const zp = "AT0030000000000000000000000953104"
	ownerChange(t, zp, "2025-01-01", "2025-06-15", "2025-06-16", 0)
	oldOwner := "c1050000-0000-0000-0000-" + zp[len(zp)-12:]

	w, sent := revoke(t, oldOwner, revokeRequest(zp, ""))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, notActiveBody, w.Body.String())
	assert.Empty(t, sent)
}

// One metering point with the member's active consent and one without: the whole request is refused, nothing is sent
// (no partial revoke the user would take for a complete one).
func TestRevokePartlyActiveIsRefusedWhole(t *testing.T) {
	const active, ended = "AT0030000000000000000000000953105", "AT0030000000000000000000000953106"
	ownerChange(t, active, "2025-01-01", "2025-06-15", "2025-06-16", 0)
	ownerChange(t, ended, "2025-01-01", "2025-06-15", "2025-06-16", 0)
	member := "c1050001-0000-0000-0000-" + active[len(active)-12:]
	// the member also owns the old (inactive) entry of the second metering point
	_, err := testDB.DbInstance.Exec(`UPDATE base.meteringpoint SET participant_id = $1 WHERE metering_point_id = $2
		AND participant_id = $3`, member, ended, "c1050000-0000-0000-0000-"+ended[len(ended)-12:])
	require.NoError(t, err)

	body := fmt.Sprintf(`{"meteringPoints":[{"meter":%q,"direction":"CONSUMPTION"},{"meter":%q,"direction":"CONSUMPTION"}],"from":1793574000000}`,
		active, ended)
	w, sent := revoke(t, member, body)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, notActiveBody, w.Body.String())
	assert.Empty(t, sent)
}

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TE001006 has one member (Peter, 04583ce3-…) with the ACTIVE metering point AT…953013 (seed.sql).
const (
	umTenant       = "TE001006"
	umActiveMeter  = "AT0030000000000000000000000953013"
	umMemberJSON   = `"firstname":"Uma","lastname":"Meldung","businessRole":"EEG_PRIVATE","role":"EEG_USER"`
	umNoDebitIban  = `"accountInfo":{"owner":"Uma Meldung","sepaDirectDebit":"NONE"}`
	umRegisterPath = "/participant"
)

func registerForTest(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, umRegisterPath, strings.NewReader(body))
	db, err := database.GetDB(context.Background())
	require.NoError(t, err)
	w := httptest.NewRecorder()
	NewParticipantHandler(db).registerParticipant()(w, req, &middleware.PlatformClaims{Username: "test"}, umTenant)
	return w
}

func errorBody(t *testing.T, w *httptest.ResponseRecorder) HttpError {
	t.Helper()
	var body struct {
		Error HttpError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body.Error
}

func deleteMembersNamed(t *testing.T, lastname string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.metering_partition_factor WHERE participant_id IN
			(SELECT id FROM base.participant WHERE tenant = $1 AND lastname = $2)`, umTenant, lastname)
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.meteringpoint WHERE participant_id IN
			(SELECT id FROM base.participant WHERE tenant = $1 AND lastname = $2)`, umTenant, lastname)
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.participant WHERE tenant = $1 AND lastname = $2`, umTenant, lastname)
	})
}

// A4: a member without direct debit and without IBAN ("Kein SEPA", platform#143) is stored with an empty IBAN, not
// NULL: production keeps NOT NULL on base.bankaccount.iban.
func TestRegisterParticipant_withoutIbanStoresEmptyIban(t *testing.T) {
	deleteMembersNamed(t, "Meldung")
	w := registerForTest(t, `{`+umMemberJSON+`,`+umNoDebitIban+`}`)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var iban *string
	require.NoError(t, testDB.DbInstance.Get(&iban, `SELECT b.iban FROM base.bankaccount b
		JOIN base.participant p ON p.id = b.participant_id WHERE p.tenant = $1 AND p.lastname = 'Meldung'`, umTenant))
	require.NotNil(t, iban, "IBAN is NULL")
	assert.Equal(t, "", *iban)
}

// A2: registering a member with a metering point that is active at another member answered the generic 1104. Now
// code 1207 with a German text naming the metering point; the member is not stored.
func TestRegisterParticipant_meterActiveAtOtherMember(t *testing.T) {
	deleteMembersNamed(t, "Meldung")
	w := registerForTest(t, `{`+umMemberJSON+`,`+umNoDebitIban+`,"meters":[{"meteringPoint":"`+umActiveMeter+
		`","direction":"CONSUMPTION","status":"NEW","processState":"NEW"}]}`)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	e := errorBody(t, w)
	assert.Equal(t, 1207, e.Code)
	assert.Equal(t, fmt.Sprintf(model.MeterActiveAtOtherParticipantText, umActiveMeter), e.Message)

	var n int
	require.NoError(t, testDB.DbInstance.Get(&n,
		`SELECT count(*) FROM base.participant WHERE tenant = $1 AND lastname = 'Meldung'`, umTenant))
	assert.Equal(t, 0, n, "the member is not stored")
}

// A3: adding the metering point with "+" at another member answered 400 / 1111 with the database text. Now 409 / 1207.
func TestCreateMeteringPoint_activeAtOtherMemberIs409(t *testing.T) {
	deleteMembersNamed(t, "Meldung")
	require.Equal(t, http.StatusCreated, registerForTest(t, `{`+umMemberJSON+`,`+umNoDebitIban+`}`).Code)
	var participant string
	require.NoError(t, testDB.DbInstance.Get(&participant,
		`SELECT id FROM base.participant WHERE tenant = $1 AND lastname = 'Meldung'`, umTenant))
	calls := countRegistrations(t)

	body := `{"meteringPoint":"` + umActiveMeter + `","direction":"CONSUMPTION","status":"INIT","processState":"NEW"}`
	req, _ := http.NewRequest("PUT", "/meteringpoint/"+participant+"/create", strings.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"pid": participant})
	w := httptest.NewRecorder()
	newTestMeteringHandler(t).createMeteringPoint()(w, req, &middleware.PlatformClaims{Username: "test"}, umTenant)

	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	e := errorBody(t, w)
	assert.Equal(t, 1207, e.Code)
	assert.Equal(t, fmt.Sprintf(model.MeterActiveAtOtherParticipantText, umActiveMeter), e.Message)
	assert.Equal(t, 0, *calls, "nothing is sent to the grid operator")
}

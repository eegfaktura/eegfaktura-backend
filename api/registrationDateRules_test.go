package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/database"
	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platform#272, review: register follows the save rules for `from`, checks the grid operator before it stores the
// date; a new member of an online EEG is not saved with a requested date beyond the window. Today is 26.11.2026.

func TestRegisterMeteringPoint_fromWhileRunningAndReceiver(t *testing.T) {
	fixToday(t, adToday)
	h := newTestMeteringHandler(t)
	const meter = "AT0030000000000000000000000272701"
	register := func(f adFixture, from int64) *httptest.ResponseRecorder {
		body := `{"meteringPoint":"` + meter + `","direction":"CONSUMPTION","from":` + strconv.FormatInt(from, 10) + `}`
		return call(h.registerMeteringPoint(), "POST", body, map[string]string{"pid": f.participant}, f.tenant)
	}
	day := func(d int) int64 { return time.Date(2027, time.January, d, 12, 0, 0, 0, time.UTC).UnixMilli() }

	t.Run("running registration: another day rejected, nothing stored or sent", func(t *testing.T) {
		f := newAdFixture(t, "TE002741", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "INIT", "PENDING", "2027-01-05", nil)
		sent := captureRegistrations(t)

		assertErrorCode(t, register(f, day(8)), 1209, meter)
		registered, _ := f.dates(meter)
		assert.Equal(t, "2027-01-05", registered)
		assert.Empty(t, sent)
	})

	t.Run("running registration: the same day or none is sent again", func(t *testing.T) {
		f := newAdFixture(t, "TE002741", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "INIT", "PENDING", "2027-01-05", nil)
		sent := captureRegistrations(t)

		w := register(f, day(5))
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		w = register(f, 0)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		assert.Equal(t, civil.DateFor(2027, 1, 5), sent[meter])
	})

	t.Run("no grid operator: rejected before the date is stored", func(t *testing.T) {
		f := newAdFixture(t, "TE002742", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "INIT", "REJECTED", "2026-12-01", nil)
		f.exec(`UPDATE base.eeg SET gridoperator_code = '' WHERE tenant = $1`, f.tenant)
		sent := captureRegistrations(t)

		w := register(f, day(8))

		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		registered, _ := f.dates(meter)
		assert.Equal(t, "2026-12-01", registered)
		assert.Empty(t, sent)
	})
}

func TestRegisterParticipant_requestedDateWindow(t *testing.T) {
	fixToday(t, adToday)
	db, err := database.GetDB(context.Background())
	require.NoError(t, err)
	f := newAdFixture(t, "TE002743", true, "ACTIVE", "2026-01-01")
	const id = "c2720000-0000-4000-8000-002743000001"
	register := func(registeredSince string) *httptest.ResponseRecorder {
		body := `{"id":"` + id + `","firstname":"Neu","lastname":"Mitglied","businessRole":"EEG_PRIVATE","role":"EEG_USER",
			"participantSince":"2026-12-01","meters":[{"meteringPoint":"AT0030000000000000000000000272801",
			"direction":"CONSUMPTION","status":"INIT","processState":"NEW","registeredSince":"` + registeredSince + `"}]}`
		return call(NewParticipantHandler(db).registerParticipant(), "POST", body, nil, f.tenant)
	}
	count := func() int {
		var n int
		require.NoError(t, testDB.DbInstance.Get(&n, `SELECT count(*) FROM base.participant WHERE id = $1`, id))
		return n
	}

	assertErrorCode(t, register("2027-01-14"), 1208, "13.01.2027")
	assert.Equal(t, 0, count(), "member not saved")
}

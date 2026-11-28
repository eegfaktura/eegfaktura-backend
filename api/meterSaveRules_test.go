package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platform#272: save rules of the REST updates for „Aktiv ab“ (online EEG). Today is fixed to 26.11.2026.
func TestUpdateMeteringPoint_dateRules(t *testing.T) {
	fixToday(t, adToday)
	h := newTestMeteringHandler(t)
	update := func(f adFixture, meter, status, processState, registered, active string) *httptest.ResponseRecorder {
		body := `{"meteringPoint":"` + meter + `","direction":"CONSUMPTION","status":"` + status + `","processState":"` +
			processState + `","registeredSince":"` + registered + `","participantState":{"activeSince":` + active + `}}`
		return call(h.updateMeteringPoint(), "PUT", body, map[string]string{"pid": f.participant, "mid": meter}, f.tenant)
	}
	const meter = "AT0030000000000000000000000272401"

	t.Run("running registration: changed date rejected, same day accepted", func(t *testing.T) {
		for _, state := range []string{"INIT", "PENDING", "APPROVED"} {
			f := newAdFixture(t, "TE002731", true, "ACTIVE", "2026-01-01")
			f.meter(meter, "INIT", state, "2027-01-05", nil)

			w := update(f, meter, "INIT", state, "2027-01-06", "null")
			assertErrorCode(t, w, 1209, meter)
			registered, _ := f.dates(meter)
			assert.Equal(t, "2027-01-05", registered, state)

			w = update(f, meter, "INIT", state, "2027-01-05", "null")
			require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
		}
	})

	t.Run("not active: activeSince is not written, the requested date is", func(t *testing.T) {
		f := newAdFixture(t, "TE002731", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "INIT", "REJECTED", "2026-12-01", "2026-11-01")

		w := update(f, meter, "INIT", "REJECTED", "2027-01-13", `"2026-12-24"`)

		require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
		registered, active := f.dates(meter)
		assert.Equal(t, "2027-01-13", registered)
		assert.Equal(t, "2026-11-01", active, "activesince left alone")
		var body model.MeteringPoint
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.NotNil(t, body.State)
		assert.Equal(t, civil.NullDate{Date: civil.DateFor(2026, 11, 1), Valid: true}, body.State.ActiveSince,
			"the answer carries the stored date")
	})

	t.Run("not active: requested date beyond the window rejected", func(t *testing.T) {
		f := newAdFixture(t, "TE002731", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "INIT", "NEW", "2026-12-01", nil)

		w := update(f, meter, "INIT", "NEW", "2027-01-14", "null")

		assertErrorCode(t, w, 1208, "13.01.2027")
		registered, _ := f.dates(meter)
		assert.Equal(t, "2026-12-01", registered)
	})

	t.Run("active: changed activeSince rejected, same day accepted, admin path stays open", func(t *testing.T) {
		f := newAdFixture(t, "TE002731", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "ACTIVE", "ACTIVE", "2026-01-01", "2026-02-01")

		w := update(f, meter, "ACTIVE", "ACTIVE", "2026-01-01", `"2026-03-01"`)
		assertErrorCode(t, w, 1209, meter)
		_, active := f.dates(meter)
		assert.Equal(t, "2026-02-01", active)

		// e.g. a tariff change: web sends the whole metering point with the unchanged date
		w = update(f, meter, "ACTIVE", "ACTIVE", "2026-01-01", `"2026-02-01"`)
		require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())

		// „Aktiv seit ändern“ of the admin tool (gRPC UpdateEegRequest_ACTIVESINCE)
		db, err := database.GetDB(context.Background())
		require.NoError(t, err)
		d := civil.DateFor(2026, 3, 1)
		require.NoError(t, db.UpdateActiveSinceDate(context.Background(), f.tenant, f.participant, meter, "admin", &d))
		_, active = f.dates(meter)
		assert.Equal(t, "2026-03-01", active)
	})

	t.Run("offline: unchanged", func(t *testing.T) {
		f := newAdFixture(t, "TE002732", false, "ACTIVE", "2026-01-01")
		f.meter(meter, "ACTIVE", "ACTIVE", "2026-01-01", "2026-02-01")

		w := update(f, meter, "ACTIVE", "ACTIVE", "2028-01-01", `"2026-03-01"`)

		require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
		registered, active := f.dates(meter)
		assert.Equal(t, "2028-01-01", registered)
		assert.Equal(t, "2026-03-01", active)
	})
}

func TestUpdateMeteringPointPartial_dateRules(t *testing.T) {
	fixToday(t, adToday)
	h := newTestMeteringHandler(t)
	const meter = "AT0030000000000000000000000272501"
	update := func(f adFixture, value string) *httptest.ResponseRecorder {
		body := `{"path":"registeredSince","value":"` + value + `"}`
		return call(h.updateMeteringPointPartial(), "PUT", body, map[string]string{"pid": f.participant, "mid": meter}, f.tenant)
	}

	t.Run("running registration: changed date rejected, same day accepted", func(t *testing.T) {
		f := newAdFixture(t, "TE002733", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "INIT", "PENDING", "2027-01-05", nil)

		assertErrorCode(t, update(f, "2027-01-06"), 1209, meter)
		w := update(f, "2027-01-05")
		require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
	})

	t.Run("not active: within the window saved, beyond rejected", func(t *testing.T) {
		f := newAdFixture(t, "TE002733", true, "ACTIVE", "2026-01-01")
		f.meter(meter, "INIT", "NEW", "2026-12-01", nil)

		w := update(f, "2027-01-13")
		require.Equal(t, http.StatusAccepted, w.Code, w.Body.String())
		assertErrorCode(t, update(f, "2027-01-14"), 1208, "13.01.2027")
		registered, _ := f.dates(meter)
		assert.Equal(t, "2027-01-13", registered)
	})
}

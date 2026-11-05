package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	siTenant      = "TE000231"
	siOtherTenant = "TE000232"
)

// POST /eeg/settlementinterval (konzept-abrechnungsintervall.md): rule, record and the contract with web.
func TestChangeSettlementInterval(t *testing.T) {
	ctx := context.Background()
	db, err := database.GetDB(ctx)
	require.NoError(t, err)
	exec := func(stmt string, args ...interface{}) {
		_, err := execTestSQL(ctx, db, stmt, args...)
		require.NoError(t, err)
	}
	removeFixture := func() {
		for _, table := range []string{"base.settlement_interval_change", "base.eeg"} {
			exec(`DELETE FROM `+table+` WHERE tenant IN ($1, $2)`, siTenant, siOtherTenant)
		}
	}
	removeFixture()
	t.Cleanup(removeFixture)
	for tenant, communityId := range map[string]string{siTenant: "AT00300000000TC000231000000000001",
		siOtherTenant: "AT00300000000TC000232000000000001"} {
		exec(`INSERT INTO base.eeg
			(tenant, name, description, "rcNumber", area, gridoperator_code, gridoperator_name, "communityId",
			 street, "streetNumber", city, zip, email, "settlementInterval")
			VALUES ($1, 'INTERVALL', 'Intervall-Test', $2, 'LOCAL', 'AT003000', 'Netz OÖ', $3,
			 'Weg', '1', 'Linz', '4020', 'intervall@example.org', 'MONTHLY')`, tenant, tenant, communityId)
	}
	// every subtest starts from MONTHLY without a record, whatever ran before
	reset := func() {
		exec(`UPDATE base.eeg SET "settlementInterval" = 'MONTHLY' WHERE tenant IN ($1, $2)`, siTenant, siOtherTenant)
		exec(`DELETE FROM base.settlement_interval_change WHERE tenant IN ($1, $2)`, siTenant, siOtherTenant)
	}

	orig := middleware.VerifyTokenClaims
	t.Cleanup(func() { middleware.VerifyTokenClaims = orig })
	middleware.VerifyTokenClaims = func(_ context.Context, token string) (*middleware.PlatformClaims, error) {
		c := &middleware.PlatformClaims{Username: "admin-" + token, Tenants: []string{strings.ToLower(siTenant)},
			AccessGroups: middleware.AccessGroups{"/EEG_ADMIN"}}
		switch token {
		case "user":
			c.AccessGroups = middleware.AccessGroups{"/EEG_USER"}
		case "other":
			c.Tenants = []string{siOtherTenant}
		}
		return c, nil
	}
	router := InitEegRouter(mux.NewRouter(), db)
	call := func(token, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", middleware.BEARER_SCHEMA+token)
		req.Header.Set("tenant", siTenant)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	post := func(token, body string) *httptest.ResponseRecorder {
		return call(token, "/eeg/settlementinterval", body)
	}
	decode := func(w *httptest.ResponseRecorder) map[string]interface{} {
		var m map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &m), w.Body.String())
		return m
	}
	errorOf := func(w *httptest.ResponseRecorder) HttpError {
		var resp struct {
			Error HttpError `json:"error"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp), w.Body.String())
		return resp.Error
	}
	interval := func() string {
		eeg, err := db.GetEegById(ctx, siTenant)
		require.NoError(t, err)
		return eeg.SettlementInterval
	}
	const billedToMar = `"billingState":{"finalPeriods":["Abr_YM-2026-3","Abr_YM-2026-2"],"runningPeriods":[]}`
	const billedToFeb = `"billingState":{"finalPeriods":["Abr_YM-2026-2"],"runningPeriods":[]}`

	t.Run("access", func(t *testing.T) {
		reset()
		assert.Equal(t, http.StatusUnauthorized, post("user", `{"settlementInterval":"QUARTER","dryRun":true,`+billedToMar+`}`).Code)
		assert.Equal(t, http.StatusForbidden, post("other", `{"settlementInterval":"QUARTER","dryRun":true,`+billedToMar+`}`).Code)
	})
	t.Run("400", func(t *testing.T) {
		reset()
		for body, code := range map[string]int{
			`{"settlementInterval":"YEARLY","dryRun":true,` + billedToMar + `}`:  1060,
			`{"settlementInterval":"","dryRun":true,` + billedToMar + `}`:        1060,
			`{"settlementInterval":"QUARTER","dryRun":true}`:                     1060,
			`{"settlementInterval":"QUARTER","dryRun":true,"billingState":null}`: 1060,
			// eegfaktura-dev#54 #9: incomplete or unreadable state, data after the body
			`{"settlementInterval":"ANNUAL","dryRun":false,"billingState":{}}`:                                                                     1060,
			`{"settlementInterval":"ANNUAL","dryRun":false,"billingState":{"finalPeriods":null,"runningPeriods":[]}}`:                              1060,
			`{"settlementInterval":"ANNUAL","dryRun":false,"billingState":{"finalPeriods":[],"runningPeriods":null}}`:                              1060,
			`{"settlementInterval":"ANNUAL","dryRun":false,"billingState":{"finalPeriods":["foo"],"runningPeriods":[]}}`:                           1060,
			`{"settlementInterval":"ANNUAL","dryRun":false,"billingState":{"finalPeriods":["Abr_YM-2026-1"],"runningPeriods":["Abr_YM-2026-13"]}}`: 1060,
			`{"settlementInterval":"ANNUAL","dryRun":false,"billingState":{"finalPeriods":["Abr_Y-9999"],"runningPeriods":[]}}`:                    1060,
			`{"settlementInterval":"QUARTER","dryRun":false,` + billedToMar + `}garbage`:                                                           2000,
			`{"settlementInterval":"QUARTER","dryRun":false,` + billedToMar + `}}`:                                                                 2000,
			`{"settlementInterval":"QUARTER","dryRun":false,` + billedToMar + `} {}`:                                                               2000,
			`{"settlementInterval":"MONTHLY","dryRun":true,` + billedToMar + `}`:                                                                   1061,
			`{"settlementInterval":`: 2000,
		} {
			w := post("admin", body)
			require.Equal(t, http.StatusBadRequest, w.Code, body)
			assert.Equal(t, code, errorOf(w).Code, body)
		}
		assert.Equal(t, "MONTHLY", interval())
	})
	t.Run("dry run allowed", func(t *testing.T) {
		reset()
		w := post("admin", `{"settlementInterval":"QUARTER","dryRun":true,`+billedToMar+"}\n")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, map[string]interface{}{"allowed": true, "effectiveFrom": "2026-04-01", "lastFinalPeriod": "Abr_YM-2026-3",
			"message": "Das neue Intervall gilt ab 1. April 2026.", "earliestFrom": nil, "eeg": nil}, decode(w))
		assert.Equal(t, "MONTHLY", interval())
	})
	t.Run("dry run blocked (R2, rule 6, R3)", func(t *testing.T) {
		reset()
		w := post("admin", `{"settlementInterval":"QUARTER","dryRun":true,`+billedToFeb+`}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, map[string]interface{}{"allowed": false, "effectiveFrom": nil, "lastFinalPeriod": "Abr_YM-2026-2",
			"message":      "Zuerst muss noch der Zeitraum März 2026 im bisherigen Intervall abgerechnet werden. Ein Wechsel ist dann ab 1. April 2026 möglich.",
			"earliestFrom": "2026-04-01", "eeg": nil}, decode(w))

		// rule 6 (eegfaktura-dev#54 #2): February between two final months blocks
		w = post("admin", `{"settlementInterval":"QUARTER","dryRun":true,"billingState":{"finalPeriods":["Abr_YM-2026-1","Abr_YM-2026-3"],"runningPeriods":[]}}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, map[string]interface{}{"allowed": false, "effectiveFrom": nil, "lastFinalPeriod": "Abr_YM-2026-3",
			"message": "Februar 2026 ist noch nicht endgültig abgerechnet.", "earliestFrom": nil, "eeg": nil}, decode(w))

		w = post("admin", `{"settlementInterval":"QUARTER","dryRun":true,"billingState":{"finalPeriods":["Abr_YM-2026-3"],"runningPeriods":["Abr_YM-2026-4"]}}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		m := decode(w)
		assert.Equal(t, false, m["allowed"])
		assert.Nil(t, m["earliestFrom"])
		assert.Equal(t, "Für April 2026 läuft gerade eine Berechnung. Bitte nach deren Ende erneut versuchen.", m["message"])
	})
	t.Run("blocked change: 409 with the reason", func(t *testing.T) {
		reset()
		w := post("admin", `{"settlementInterval":"QUARTER","dryRun":false,`+billedToFeb+`}`)
		require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
		e := errorOf(w)
		assert.Equal(t, 1062, e.Code)
		assert.Equal(t, "Zuerst muss noch der Zeitraum März 2026 im bisherigen Intervall abgerechnet werden. Ein Wechsel ist dann ab 1. April 2026 möglich.", e.Message)
		assert.Equal(t, "MONTHLY", interval())
	})
	t.Run("generic update drops settlementInterval", func(t *testing.T) {
		reset()
		w := call("admin", "/eeg", `{"settlementInterval":"ANNUAL","description":"Intervall-Test 2"}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		assert.Equal(t, "MONTHLY", decode(w)["settlementInterval"])
		assert.Equal(t, "MONTHLY", interval())
	})
	t.Run("allowed change: stored, recorded, EEG returned", func(t *testing.T) {
		reset()
		w := post("admin", `{"settlementInterval":"QUARTER","dryRun":false,`+billedToMar+`}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		m := decode(w)
		assert.Equal(t, true, m["allowed"])
		assert.Equal(t, "2026-04-01", m["effectiveFrom"])
		require.IsType(t, map[string]interface{}{}, m["eeg"])
		assert.Equal(t, "QUARTER", m["eeg"].(map[string]interface{})["settlementInterval"])
		assert.Equal(t, "QUARTER", interval())

		var by []string
		require.NoError(t, db.Select(ctx, &by, `SELECT changed_by FROM base.settlement_interval_change WHERE tenant = $1`, siTenant))
		assert.Equal(t, []string{"admin-admin"}, by)
		other, err := db.GetEegById(ctx, siOtherTenant)
		require.NoError(t, err)
		assert.Equal(t, "MONTHLY", other.SettlementInterval, "the other tenant is not touched")
	})
}

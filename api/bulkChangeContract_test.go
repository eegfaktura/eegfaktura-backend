package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"github.com/gorilla/mux"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contract of /bulkchange/* with eegfaktura-admin: the handlers' answers equal the fixtures in
// testdata/bulkchange (see its README).

const (
	bcTenant = "TE000441"
	bcIban   = "AT611904300234573201"
)

func bcId(n string) string { return "d4410000-0000-4000-8000-0000000000" + n }

func bcExec(t *testing.T, q string, args ...interface{}) {
	t.Helper()
	_, err := testDB.DbInstance.Exec(q, args...)
	require.NoError(t, err)
}

// bcSetup builds the scenario of the fixtures: four members of TE000441 for "reference ← 'Lastschrift-' + member
// number, where empty".
func bcSetup(t *testing.T, tenants ...string) {
	t.Helper()
	tenants = append(tenants, bcTenant)
	clean := func() {
		bcExec(t, `DELETE FROM base.participant WHERE tenant = ANY($1)`, pq.Array(tenants))
		bcExec(t, `DELETE FROM base.tariff WHERE tenant = ANY($1)`, pq.Array(tenants))
		bcExec(t, `DELETE FROM base.eeg WHERE tenant = ANY($1)`, pq.Array(tenants))
	}
	clean()
	t.Cleanup(clean)
	bcExec(t, `INSERT INTO base.eeg (tenant, name, description, "rcNumber", area, gridoperator_code, gridoperator_name,
		"communityId", street, "streetNumber", city, zip, email, iban, bic, creditor_id)
		VALUES ($1, 'EEG Massenänderung', 'Bulk', 'RC100441', 'LOCAL', 'AT009999', 'Test Operator',
		'AT00999900000TC100441000000000001', 'Weg', '1', 'Linz', '4020', 'bulk@example.org', $2, 'BKAUATWW', 'AT99ZZZ0441')`,
		bcTenant, bcIban)
	for _, m := range []struct {
		n           string
		number, ref interface{}
	}{{"01", "1", nil}, {"02", "2", "Lastschrift-2"}, {"03", "3", "X-3"}, {"04", nil, nil}} {
		bcExec(t, `INSERT INTO base.participant (id, tenant, "participantNumber", firstname, lastname, status, "createdBy",
			"lastModifiedBy") VALUES ($1, $2, $3, 'Anna', 'Muster', 'ACTIVE', 'test', 'test')`, bcId(m.n), bcTenant, m.number)
		bcExec(t, `INSERT INTO base.bankaccount (participant_id, iban, mandate_reference, mandate_date, sepa_direct_debit)
			VALUES ($1, $2, $3, '2025-01-01', 'CORE')`, bcId(m.n), bcIban, m.ref)
	}
}

// bcServe sends a request through the router with a token of the given kind ("" = no token).
func bcServe(t *testing.T, method, path, token, tenant, body string) *httptest.ResponseRecorder {
	t.Helper()
	orig := middleware.VerifyTokenClaims
	t.Cleanup(func() { middleware.VerifyTokenClaims = orig })
	middleware.VerifyTokenClaims = func(_ context.Context, tok string) (*middleware.PlatformClaims, error) {
		switch tok {
		case "superuser":
			return &middleware.PlatformClaims{Username: "support-user", RealmAccess: middleware.RealmRoles{Roles: []string{"superuser"}}}, nil
		case "superuser-no-name":
			return &middleware.PlatformClaims{Username: " ", RealmAccess: middleware.RealmRoles{Roles: []string{"superuser"}}}, nil
		case "admin":
			return &middleware.PlatformClaims{Username: "eeg-admin", Tenants: []string{bcTenant},
				AccessGroups: middleware.AccessGroups{"/EEG_ADMIN"}}, nil
		}
		return nil, errors.New("invalid token")
	}
	db, err := database.GetDB(context.Background())
	require.NoError(t, err)
	r := InitBulkChangeRouter(mux.NewRouter(), db)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tenant != "" {
		req.Header.Set("tenant", tenant)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func bcFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "bulkchange", name))
	require.NoError(t, err)
	return string(b)
}

func TestBulkChangeContract_communities(t *testing.T) {
	bcSetup(t)
	w := bcServe(t, http.MethodGet, "/bulkchange/communities", "superuser", "", "")
	require.Equal(t, http.StatusOK, w.Code)
	var got []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	var want []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(bcFixture(t, "communities.response.json")), &want))
	assert.Contains(t, got, want[0])
	for _, c := range got {
		assert.Len(t, c, 3, "tenant, name and rcNumber only: %v", c)
	}
}

// Active tariffs of the types the rules can set, latest version, sorted by type and name; nothing of other
// communities, no archived or AKONTO tariff.
func TestBulkChangeContract_tariffs(t *testing.T) {
	bcSetup(t, "TE000442")
	tariff := func(n, tenant, typ, name string, version int, status string, fee float64) {
		bcExec(t, `INSERT INTO base.tariff (id, tenant, type, name, version, status, "joiningFee") VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			"d4419000-0000-4000-8000-0000000000"+n, tenant, typ, name, version, status, fee)
	}
	tariff("01", bcTenant, "EEG", "Alter Name", 1, "INACTIVE", 0)
	tariff("01", bcTenant, "EEG", "Mitgliedsbeitrag", 2, "ACTIVE", 50)
	tariff("02", bcTenant, "VZP", "Bezug", 1, "ACTIVE", 0)
	tariff("03", bcTenant, "EZP", "Einspeisung", 1, "ACTIVE", 0)
	tariff("04", bcTenant, "AKONTO", "Akonto", 1, "ACTIVE", 0)
	tariff("05", bcTenant, "EEG", "Archiviert", 1, "ARCHIVED", 0)
	tariff("06", "TE000442", "EEG", "Fremd", 1, "ACTIVE", 0)

	w := bcServe(t, http.MethodGet, "/bulkchange/tariffs", "superuser", strings.ToLower(bcTenant), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, bcFixture(t, "tariffs.response.json"), w.Body.String())

	bcExec(t, `INSERT INTO base.eeg (tenant, name, "rcNumber", area, gridoperator_code, gridoperator_name, "communityId",
		street, "streetNumber", city, zip, email) VALUES ('TE000442', 'Ohne', 'RC100442', 'LOCAL', 'AT009999', 'Op',
		'AT00999900000TC100442000000000001', 'Weg', '1', 'Linz', '4020', 'x@example.org')`)
	bcExec(t, `DELETE FROM base.tariff WHERE tenant = 'TE000442'`)
	w = bcServe(t, http.MethodGet, "/bulkchange/tariffs", "superuser", "TE000442", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `[]`, w.Body.String(), "no tariffs: an empty list")
}

// A rule without hits answers empty lists, never null (the admin page reads them as lists).
func TestBulkChangeContract_previewEmpty(t *testing.T) {
	bcSetup(t)
	body := `{"rule": {"field": "METER_TARIFF", "action": "CLEAR", "condition": "ALL", "direction": "GENERATION"}}`
	w := bcServe(t, http.MethodPost, "/bulkchange/preview", "superuser", bcTenant, body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"rows":[]`)
	assert.NotContains(t, w.Body.String(), "null")
	assert.JSONEq(t, bcFixture(t, "preview.empty.response.json"), w.Body.String())
}

func TestBulkChangeContract_previewAndApply(t *testing.T) {
	bcSetup(t)
	// the tenant in another case reaches the same community
	w := bcServe(t, http.MethodPost, "/bulkchange/preview", "superuser", strings.ToLower(bcTenant), bcFixture(t, "preview.request.json"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, bcFixture(t, "preview.response.json"), w.Body.String())
	assert.NotContains(t, w.Body.String(), bcIban, "never an IBAN")

	w = bcServe(t, http.MethodPost, "/bulkchange/apply", "superuser", bcTenant, bcFixture(t, "apply.request.json"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, bcFixture(t, "apply.response.json"), w.Body.String())
	assert.NotContains(t, w.Body.String(), bcIban)

	var by string
	require.NoError(t, testDB.DbInstance.Get(&by, `SELECT "lastModifiedBy" FROM base.participant WHERE id = $1`, bcId("01")))
	assert.Equal(t, "support-user", by, "the user of the token")
}

func TestBulkChangeContract_errorBody(t *testing.T) {
	bcSetup(t)
	tx, err := testDB.DbInstance.Beginx()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	// a second backend replica holds the tenant lock (classid of database.bulkLockTenant)
	_, err = tx.Exec(`SELECT pg_advisory_xact_lock($1, hashtext($2))`, 0x42430001, bcTenant)
	require.NoError(t, err)

	w := bcServe(t, http.MethodPost, "/bulkchange/apply", "superuser", bcTenant, bcFixture(t, "apply.request.json"))
	require.Equal(t, http.StatusConflict, w.Code)
	assert.JSONEq(t, bcFixture(t, "error.response.json"), w.Body.String())
}

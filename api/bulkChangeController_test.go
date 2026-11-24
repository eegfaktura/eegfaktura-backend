package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bcCode(t *testing.T, w interface{ Bytes() []byte }) int {
	t.Helper()
	var body struct {
		Error HttpError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Bytes(), &body))
	return body.Error.Code
}

// 401/403 before 404 before 400: the token first, then the community, then the body.
func TestBulkChange_authAndTenantOrder(t *testing.T) {
	bcSetup(t)
	const badBody = `{"rule": {"field": "iban"}}`
	cases := []struct {
		name, method, path, token, tenant, body string
		want                                    int
	}{
		{"no token", http.MethodPost, "/bulkchange/preview", "", bcTenant, badBody, http.StatusForbidden},
		{"invalid token", http.MethodPost, "/bulkchange/preview", "bad", bcTenant, badBody, http.StatusUnauthorized},
		{"admin group without superuser", http.MethodPost, "/bulkchange/preview", "admin", bcTenant, badBody, http.StatusForbidden},
		{"admin group: apply", http.MethodPost, "/bulkchange/apply", "admin", bcTenant, badBody, http.StatusForbidden},
		{"admin group: communities", http.MethodGet, "/bulkchange/communities", "admin", bcTenant, "", http.StatusForbidden},
		{"no token: communities", http.MethodGet, "/bulkchange/communities", "", "", "", http.StatusForbidden},
		{"admin group: tariffs", http.MethodGet, "/bulkchange/tariffs", "admin", bcTenant, "", http.StatusForbidden},
		{"invalid token: tariffs", http.MethodGet, "/bulkchange/tariffs", "bad", bcTenant, "", http.StatusUnauthorized},
		{"superuser, unknown community: tariffs", http.MethodGet, "/bulkchange/tariffs", "superuser", "TE000499", "", http.StatusNotFound},
		{"superuser, unknown community", http.MethodPost, "/bulkchange/preview", "superuser", "TE000499", badBody, http.StatusNotFound},
		{"superuser, no tenant header", http.MethodPost, "/bulkchange/apply", "superuser", "", badBody, http.StatusNotFound},
		{"superuser, bad rule", http.MethodPost, "/bulkchange/preview", "superuser", bcTenant, badBody, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := bcServe(t, c.method, c.path, c.token, c.tenant, c.body)
			assert.Equal(t, c.want, w.Code, w.Body.String())
		})
	}
}

// Everything outside the matrix or the request shape is 400 (6100/6101/6102), nothing is written.
func TestBulkChange_badRequests(t *testing.T) {
	bcSetup(t, "TE000442")
	bcExec(t, `INSERT INTO base.eeg (tenant, name, "rcNumber", area, gridoperator_code, gridoperator_name, "communityId",
		street, "streetNumber", city, zip, email) VALUES ('TE000442', 'Andere', 'RC100442', 'LOCAL', 'AT009999', 'Op',
		'AT00999900000TC100442000000000001', 'Weg', '1', 'Linz', '4020', 'x@example.org')`)
	bcExec(t, `INSERT INTO base.participant (id, tenant, firstname, "createdBy", "lastModifiedBy")
		VALUES ('d4420000-0000-4000-8000-000000000001', 'TE000442', 'Fremd', 'test', 'test')`)
	rule := `{"field": "DEBIT_TYPE", "action": "SET", "value": "NONE", "condition": "ALL"}`
	item := func(id string) string {
		return `{"participantId": "` + id + `", "expectedOld": "CORE", "expectedNew": "NONE"}`
	}
	apply := func(items ...string) string {
		return `{"rule": ` + rule + `, "items": [` + strings.Join(items, ",") + `]}`
	}
	cases := map[string]struct {
		path, body string
		code       int
	}{
		"field partFact":             {"preview", `{"rule": {"field": "partFact", "action": "SET", "value": "1", "condition": "ALL"}}`, 6100},
		"field allocationFactor":     {"preview", `{"rule": {"field": "allocationFactor", "action": "SET", "value": "1", "condition": "ALL"}}`, 6100},
		"field participantSince":     {"preview", `{"rule": {"field": "participantSince", "action": "SET", "value": "2026-01-01", "condition": "ALL"}}`, 6100},
		"unknown key iban":           {"preview", `{"rule": {"field": "DEBIT_TYPE", "action": "CLEAR", "condition": "ALL", "iban": "AT1"}}`, 6100},
		"unknown key tenant":         {"preview", `{"tenant": "TE000442", "rule": ` + rule + `}`, 6100},
		"no rule":                    {"preview", `{}`, 6100},
		"not json":                   {"preview", `rule`, 6100},
		"two json values":            {"preview", `{"rule": ` + rule + `} {}`, 6100},
		"apply without items":        {"apply", apply(), 6100},
		"apply null item":            {"apply", apply("null"), 6100},
		"apply duplicate id":         {"apply", apply(item(bcId("01")), item(strings.ToUpper(bcId("01")))), 6101},
		"apply unknown id":           {"apply", apply(item(bcId("01")), item(bcId("99"))), 6102},
		"apply id of another tenant": {"apply", apply(item(bcId("01")), item("d4420000-0000-4000-8000-000000000001")), 6102},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			w := bcServe(t, http.MethodPost, "/bulkchange/"+c.path, "superuser", bcTenant, c.body)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Equal(t, c.code, bcCode(t, w.Body))
		})
	}
	var n int
	require.NoError(t, testDB.DbInstance.Get(&n, `SELECT count(*) FROM base.bankaccount b JOIN base.participant p ON p.id = b.participant_id
		WHERE p.tenant = $1 AND b.sepa_direct_debit = 'NONE'`, bcTenant))
	assert.Zero(t, n, "nothing written")
}

// Apply needs the user name of the token for lastModifiedBy/modifiedBy: without it 403, nothing written.
func TestBulkChange_applyWithoutUserName(t *testing.T) {
	bcSetup(t)
	w := bcServe(t, http.MethodPost, "/bulkchange/apply", "superuser-no-name", bcTenant, bcFixture(t, "apply.request.json"))
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Equal(t, 6106, bcCode(t, w.Body))
	var ref *string
	require.NoError(t, testDB.DbInstance.Get(&ref, `SELECT mandate_reference FROM base.bankaccount WHERE participant_id = $1`, bcId("01")))
	assert.Nil(t, ref, "nothing written")

	w = bcServe(t, http.MethodPost, "/bulkchange/preview", "superuser-no-name", bcTenant, bcFixture(t, "preview.request.json"))
	assert.Equal(t, http.StatusOK, w.Code, "the preview writes nothing and needs no user name")
}

type bulkApplyFails struct {
	database.Database
	err error
}

func (b bulkApplyFails) BulkEeg(context.Context, string) (*model.BulkEeg, error) {
	return &model.BulkEeg{Tenant: bcTenant}, nil
}

func (b bulkApplyFails) BulkApply(context.Context, *model.BulkEeg, string, *model.BulkApplyRequest) (*model.BulkApplyResult, error) {
	return nil, b.err
}

// R1-2: a database failure answers 500 with a neutral text; after a failed COMMIT it does not claim "nothing changed".
func TestBulkChange_serverErrorTexts(t *testing.T) {
	prev := logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))
	t.Cleanup(func() { logrus.StandardLogger().ReplaceHooks(prev) })
	hook := logrustest.NewLocal(logrus.StandardLogger())
	for _, c := range []struct {
		err  error
		want string
	}{
		{model.ErrBulkChange(errors.New("pq: connection reset")), "es wurde nichts geändert"},
		{model.ErrBulkChange(fmt.Errorf("%w: %v", database.ErrBulkOutcomeUnknown, "pq: commit")), "Ergebnis der Massenänderung ist unklar"},
	} {
		hook.Reset()
		h := &BulkChangeHandler{db: bulkApplyFails{err: c.err}}
		req := httptest.NewRequest(http.MethodPost, "/bulkchange/apply", strings.NewReader(bcFixture(t, "apply.request.json")))
		w := httptest.NewRecorder()
		h.apply()(w, req, &middleware.PlatformClaims{Username: "support-user"}, bcTenant)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), c.want)
		assert.NotContains(t, w.Body.String(), "pq:", "no database detail")
		errorLines := 0
		for _, e := range hook.AllEntries() {
			if e.Level == logrus.ErrorLevel {
				errorLines++
				assert.ErrorIs(t, e.Data[logrus.ErrorKey].(error), c.err, "the real error is logged")
			}
		}
		assert.Equal(t, 1, errorLines, "one error line (Q3)")
	}
}

// R1-4: a body over 1 MiB is 413 with its own code and text, not "request invalid".
func TestBulkChange_bodyTooLarge(t *testing.T) {
	bcSetup(t)
	body := `{"rule": {"field": "DEBIT_TYPE", "value": "` + strings.Repeat("x", 1<<20) + `"}}`
	for _, path := range []string{"/bulkchange/preview", "/bulkchange/apply"} {
		w := bcServe(t, http.MethodPost, path, "superuser", bcTenant, body)
		require.Equal(t, http.StatusRequestEntityTooLarge, w.Code, path)
		assert.Equal(t, 6107, bcCode(t, w.Body))
	}
}

// The CSV is built in eegfaktura-admin from the preview: the backend has no such route.
func TestBulkChange_noCsvRoute(t *testing.T) {
	for _, path := range []string{"/bulkchange/csv", "/bulkchange/preview.csv", "/bulkchange/export"} {
		w := bcServe(t, http.MethodGet, path, "superuser", bcTenant, "")
		assert.Equal(t, http.StatusNotFound, w.Code, path)
	}
	w := bcServe(t, http.MethodGet, "/bulkchange/preview", "superuser", bcTenant, "")
	assert.Equal(t, http.StatusNotFound, w.Code, "the preview is POST only")
}

// No EDA message: a run sends nothing to the grid operator (the outgoing processes fail the test if called).
func TestBulkChange_sendsNoEdaMessage(t *testing.T) {
	bcSetup(t)
	origReg, origEnergy, origRevoke, origFactor := mqttclient.RegistrationForParticipation, mqttclient.RequestingEnergyData,
		mqttclient.RevokeMeteringPoint, mqttclient.ChangePartitionFactor
	t.Cleanup(func() {
		mqttclient.RegistrationForParticipation, mqttclient.RequestingEnergyData = origReg, origEnergy
		mqttclient.RevokeMeteringPoint, mqttclient.ChangePartitionFactor = origRevoke, origFactor
	})
	sent := 0
	mqttclient.RegistrationForParticipation = func(*model.Eeg, *model.MeteringPoint, *int64) error { sent++; return nil }
	mqttclient.RequestingEnergyData = func(*model.Eeg, *model.MeteringPoint, int64, int64) error { sent++; return nil }
	mqttclient.RevokeMeteringPoint = func(*model.Eeg, *model.MeteringPoint, int64, *string) error { sent++; return nil }
	mqttclient.ChangePartitionFactor = func(*model.Eeg, []*model.ChangePartitionFactorRequest) error { sent++; return nil }

	w := bcServe(t, http.MethodPost, "/bulkchange/apply", "superuser", bcTenant, bcFixture(t, "apply.request.json"))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Zero(t, sent)
}

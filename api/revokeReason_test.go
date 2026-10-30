package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The optional reason of a revoke goes to the grid operator in CMRevoke ProcessDirectory/Reason (max 50 characters).
func TestRevokeReason(t *testing.T) {
	fifty := strings.Repeat("ä", 50) // characters, not bytes

	for _, tc := range []struct {
		name, raw string
		want      *string
	}{
		{"empty is none", "", nil},
		{"blanks are none", "  \t ", nil},
		{"control characters only are none", "\u0001\n", nil},
		{"trimmed", "  Auszug des Mitglieds ", revokeReasonOf("Auszug des Mitglieds")},
		{"control characters and line breaks become one space", "Auszug\u0001des\r\nMitglieds\t(Test)",
			revokeReasonOf("Auszug des Mitglieds (Test)")},
		{"non-breaking space at the ends is trimmed", " Auszug ", revokeReasonOf("Auszug")},
		{"50 characters", fifty, revokeReasonOf(fifty)},
		{"50 characters after cleaning", " " + fifty + "\n", revokeReasonOf(fifty)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := revokeReason(tc.raw)
			require.Nil(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := revokeReason(fifty + "x")
	require.NotNil(t, err)
	assert.Equal(t, 5011, err.Code)
	assert.EqualError(t, err, "Die Begründung ist zu lang (höchstens 50 Zeichen).")
}

func revokeBody(t *testing.T, fields map[string]interface{}) *strings.Reader {
	t.Helper()
	body := map[string]interface{}{
		"meteringPoints": []map[string]string{{"meter": "AT0030000000000000000000099999998", "direction": "CONSUMPTION"}},
		"from":           1700000000000,
	}
	for k, v := range fields {
		body[k] = v
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)
	return strings.NewReader(string(b))
}

// An input error of a revoke is refused with 400 before anything is looked up or sent (no database here).
func revokeWithoutDB(t *testing.T, fields map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	const pid = "ea1142dc-03da-11ee-b82b-5a985b4b0306"
	req, _ := http.NewRequest("POST", "/meteringpoint/"+pid+"/revokemeters", revokeBody(t, fields))
	req = mux.SetURLVars(req, map[string]string{"pid": pid})
	w := httptest.NewRecorder()
	NewMeteringHandler(nil).requestRevokeMeteringPoint()(w, req, &middleware.PlatformClaims{Username: "test"}, "TE000006")
	return w
}

// A reason over 50 characters is refused with its own code; it is not cut.
func TestRevokeRefusesLongReason(t *testing.T) {
	w := revokeWithoutDB(t, map[string]interface{}{"reason": strings.Repeat("x", 51)})

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":{"code":5011,"error":"Die Begründung ist zu lang (höchstens 50 Zeichen).",
		"message":"Die Begründung ist zu lang (höchstens 50 Zeichen)."}}`, w.Body.String())
}

// Without "from" the consent end used to go out as 01.01.1970; it is refused now.
func TestRevokeRefusesMissingConsentEnd(t *testing.T) {
	for _, from := range []interface{}{nil, 0, -1} {
		w := revokeWithoutDB(t, map[string]interface{}{"from": from})

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.JSONEq(t, `{"error":{"code":5012,"error":"Das Ende der Datenfreigabe fehlt.",
			"message":"Das Ende der Datenfreigabe fehlt."}}`, w.Body.String())
	}
}

func revokeReasonOf(s string) *string { return &s }

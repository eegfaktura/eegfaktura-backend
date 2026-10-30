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
		{"trimmed", "  Auszug des Mitglieds ", ptr("Auszug des Mitglieds")},
		{"50 characters", fifty, ptr(fifty)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := revokeReason(tc.raw)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	_, err := revokeReason(fifty + "x")
	assert.EqualError(t, err, "the reason may have at most 50 characters, it has 51")
}

// A reason over 50 characters is refused with 400 before anything is looked up or sent; it is not cut.
func TestRevokeRefusesLongReason(t *testing.T) {
	const pid = "ea1142dc-03da-11ee-b82b-5a985b4b0306"
	body, _ := json.Marshal(map[string]interface{}{
		"meteringPoints": []map[string]string{{"meter": "AT0030000000000000000000099999998", "direction": "CONSUMPTION"}},
		"from":           1700000000000,
		"reason":         strings.Repeat("x", 51),
	})
	req, _ := http.NewRequest("POST", "/meteringpoint/"+pid+"/revokemeters", strings.NewReader(string(body)))
	req = mux.SetURLVars(req, map[string]string{"pid": pid})
	w := httptest.NewRecorder()

	// no database: the handler must answer before it touches one
	NewMeteringHandler(nil).requestRevokeMeteringPoint()(w, req, &middleware.PlatformClaims{Username: "test"}, "TE000006")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at most 50 characters")
}

func ptr(s string) *string { return &s }

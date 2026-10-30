package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"at.ourproject/vfeeg-backend/database"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// The probes as wired by InitRouters, with the real database.Ping.
func TestInitRouters_probes(t *testing.T) {
	db, _, err := database.InitMockDatabase()
	require.NoError(t, err)
	// Nothing listens on port 1: the dedicated ping fails at once.
	viper.Set("database.host", "127.0.0.1")
	viper.Set("database.port", 1)
	defer viper.Reset()
	r := InitRouters(db)

	tests := []struct {
		path   string
		status int
		body   string
	}{
		{path: "/healthz", status: http.StatusOK, body: "ok"},
		{path: "/readyz", status: http.StatusServiceUnavailable, body: "unavailable"},
		{path: "/eeg", status: http.StatusForbidden}, // still needs a token
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tt.path, nil))
			require.Equal(t, tt.status, w.Code)
			if tt.body != "" {
				require.Equal(t, tt.body, w.Body.String())
			}
		})
	}
}

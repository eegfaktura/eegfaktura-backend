package middleware

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseBearerTokenFromHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	// missing header -> error
	_, err := ParseBearerTokenFromHeader(req)
	require.Error(t, err)

	// wrong schema
	req.Header.Set("Authorization", "Token abc")
	_, err = ParseBearerTokenFromHeader(req)
	require.Error(t, err)

	// correct bearer
	token := "sometoken123"
	req.Header.Set("Authorization", BEARER_SCHEMA+token)
	got, err := ParseBearerTokenFromHeader(req)
	require.NoError(t, err)
	require.Equal(t, token, got)
}

func TestParseBasicCredentialsFromHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	// missing header
	_, _, err := ParseBasicCredentialsFromHeader(req)
	require.Error(t, err)

	// wrong schema
	req.Header.Set("Authorization", BEARER_SCHEMA+"x")
	_, _, err = ParseBasicCredentialsFromHeader(req)
	require.Error(t, err)

	// invalid base64
	req.Header.Set("Authorization", BASIC_SCHEMA+"not-base64")
	_, _, err = ParseBasicCredentialsFromHeader(req)
	require.Error(t, err)

	// valid credentials
	creds := "user:password"
	enc := base64.StdEncoding.EncodeToString([]byte(creds))
	req.Header.Set("Authorization", BASIC_SCHEMA+enc)
	u, p, err := ParseBasicCredentialsFromHeader(req)
	require.NoError(t, err)
	require.Equal(t, "user", u)
	require.Equal(t, "password", p)

	// valid base64 but missing colon
	enc2 := base64.StdEncoding.EncodeToString([]byte("useronly"))
	req.Header.Set("Authorization", BASIC_SCHEMA+enc2)
	_, _, err = ParseBasicCredentialsFromHeader(req)
	require.Error(t, err)
}

package middleware

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// The tests in this file run real tokens through the production verifier path
// (verifyAndExtractClaims with the go-oidc verifier, VerifyTokenClaims unset).
// A local issuer serves the discovery document and the JWKS like Keycloak:
// it advertises Keycloak's list of signing algorithms and publishes an RSA
// and an EC key.

// keycloakSigningAlgs is id_token_signing_alg_values_supported of a Keycloak realm.
var keycloakSigningAlgs = []string{"PS384", "RS384", "EdDSA", "ES384", "HS256", "HS512", "ES256",
	"RS256", "HS384", "ES512", "PS256", "PS512", "RS512"}

type testIssuer struct {
	url    string
	rsaKey *rsa.PrivateKey
	ecKey  *ecdsa.PrivateKey
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func newTestIssuer(t *testing.T) *testIssuer {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	iss := &testIssuer{rsaKey: rsaKey, ecKey: ecKey}

	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	iss.url = srv.URL

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                iss.url,
			"authorization_endpoint":                iss.url + "/auth",
			"token_endpoint":                        iss.url + "/token",
			"jwks_uri":                              iss.url + "/certs",
			"id_token_signing_alg_values_supported": keycloakSigningAlgs,
		})
	})
	mux.HandleFunc("/certs", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{
			{"kty": "RSA", "kid": "rsa1", "use": "sig", "alg": "RS256",
				"n": b64url(rsaKey.N.Bytes()), "e": b64url(big.NewInt(int64(rsaKey.E)).Bytes())},
			{"kty": "EC", "kid": "ec1", "use": "sig", "alg": "ES256", "crv": "P-256",
				"x": b64url(ecKey.X.FillBytes(make([]byte, 32))), "y": b64url(ecKey.Y.FillBytes(make([]byte, 32)))},
		}})
	})
	return iss
}

// useVerifier installs a verifier for the test issuer the way InitKeycloak does.
func (iss *testIssuer) useVerifier(t *testing.T) {
	t.Helper()
	provider, err := oidc.NewProvider(context.Background(), iss.url)
	require.NoError(t, err)
	origV, origHook := verifier, VerifyTokenClaims
	t.Cleanup(func() { verifier, VerifyTokenClaims = origV, origHook })
	verifier = provider.Verifier(verifierConfig("app"))
	VerifyTokenClaims = nil
}

func (iss *testIssuer) claims() jwt.MapClaims {
	now := time.Now()
	return jwt.MapClaims{
		"iss":           iss.url,
		"sub":           "sub-1",
		"aud":           "account",
		"iat":           now.Unix(),
		"exp":           now.Add(5 * time.Minute).Unix(),
		"azp":           "at.ourproject.vfeeg.app",
		"tenant":        []string{"te100100"},
		"access_groups": []string{"/EEG_ADMIN"},
	}
}

func sign(t *testing.T, m jwt.SigningMethod, kid string, key any, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(m, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

func TestVerifyAndExtractClaims_Tokens(t *testing.T) {
	iss := newTestIssuer(t)
	iss.useVerifier(t)

	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(&iss.rsaKey.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	with := func(mod func(c jwt.MapClaims)) jwt.MapClaims {
		c := iss.claims()
		mod(c)
		return c
	}
	valid := sign(t, jwt.SigningMethodRS256, "rsa1", iss.rsaKey, iss.claims())

	tests := []struct {
		name  string
		token string
		ok    bool
	}{
		{"valid RS256", valid, true},
		{"aud as array", sign(t, jwt.SigningMethodRS256, "rsa1", iss.rsaKey,
			with(func(c jwt.MapClaims) { c["aud"] = []string{"account", "at.ourproject.vfeeg.admin"} })), true},
		{"expired", sign(t, jwt.SigningMethodRS256, "rsa1", iss.rsaKey,
			with(func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Minute).Unix() })), false},
		{"no exp", sign(t, jwt.SigningMethodRS256, "rsa1", iss.rsaKey,
			with(func(c jwt.MapClaims) { delete(c, "exp") })), false},
		{"other issuer", sign(t, jwt.SigningMethodRS256, "rsa1", iss.rsaKey,
			with(func(c jwt.MapClaims) { c["iss"] = "https://evil.example/realms/x" })), false},
		{"alg none", sign(t, jwt.SigningMethodNone, "rsa1", jwt.UnsafeAllowNoneSignatureType, iss.claims()), false},
		{"HS256 keyed with the public key", sign(t, jwt.SigningMethodHS256, "rsa1", pubPEM, iss.claims()), false},
		{"HS256 keyed with the public key, no kid", sign(t, jwt.SigningMethodHS256, "", pubPEM, iss.claims()), false},
		// RS256 only: other asymmetric algorithms the realm advertises are refused,
		// even when signed by a key from the JWKS
		{"RS384 with the realm key", sign(t, jwt.SigningMethodRS384, "rsa1", iss.rsaKey, iss.claims()), false},
		{"PS256 with the realm key", sign(t, jwt.SigningMethodPS256, "rsa1", iss.rsaKey, iss.claims()), false},
		{"ES256 with a realm EC key", sign(t, jwt.SigningMethodES256, "ec1", iss.ecKey, iss.claims()), false},
		{"wrong signature", sign(t, jwt.SigningMethodRS256, "rsa1", otherKey, iss.claims()), false},
		{"tampered payload", tamper(t, valid), false},
		{"malformed header", "%%%." + strings.SplitN(valid, ".", 2)[1], false},
		{"huge garbage header", strings.Repeat("A", 1<<20) + "." + strings.SplitN(valid, ".", 2)[1], false},
		{"not a jwt", "token", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			claims, err := verifyAndExtractClaims(context.Background(), tc.token)
			if !tc.ok {
				require.Error(t, err)
				require.Nil(t, claims)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{"te100100"}, claims.Tenants)
			require.Equal(t, "at.ourproject.vfeeg.app", claims.Authorized)
			require.True(t, claims.AccessGroups.IsAdmin())
			require.Equal(t, "sub-1", claims.Subject)
		})
	}
}

// tamper swaps the tenant in the payload and keeps the original signature.
func tamper(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	changed := strings.Replace(string(payload), "te100100", "te999999", 1)
	require.NotEqual(t, string(payload), changed)
	parts[1] = b64url([]byte(changed))
	return strings.Join(parts, ".")
}

func TestProtect_RealVerifier_TenantCaseInsensitive(t *testing.T) {
	iss := newTestIssuer(t)
	iss.useVerifier(t)
	token := sign(t, jwt.SigningMethodRS256, "rsa1", iss.rsaKey, iss.claims()) // tenant claim "te100100"

	tests := []struct {
		header     string
		wantStatus int
		wantTenant string
	}{
		{"TE100100", http.StatusOK, "TE100100"},
		{"te100100", http.StatusOK, "TE100100"},
		{"Te100100", http.StatusOK, "TE100100"},
		{"TE999999", http.StatusForbidden, ""},
		{"", http.StatusForbidden, ""},
	}
	for _, tc := range tests {
		t.Run("tenant="+tc.header, func(t *testing.T) {
			var gotTenant string
			h := Protect(func(w http.ResponseWriter, r *http.Request, claims *PlatformClaims, tenant string) {
				gotTenant = tenant
				w.WriteHeader(http.StatusOK)
			})
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", BEARER_SCHEMA+token)
			req.Header.Set("tenant", tc.header)
			rw := httptest.NewRecorder()
			h(rw, req)
			require.Equal(t, tc.wantStatus, rw.Code)
			require.Equal(t, tc.wantTenant, gotTenant)
		})
	}
}

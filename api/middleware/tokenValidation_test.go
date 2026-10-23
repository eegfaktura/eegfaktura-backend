package middleware

// Regression tests for the bearer-token validation of the real production
// path (platform#191): InitKeycloak builds the go-oidc verifier against a
// local OIDC provider (discovery + JWKS from httptest), tokens are signed in
// the test with a generated RSA key. No live Keycloak is needed.
//
// The tests pin today's behaviour; they must stay unchanged across a change
// of the JWT library.

import (
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	tvRealm = "test"
	tvKid   = "test-kid"
)

var (
	tvOnce   sync.Once
	tvKey    *rsa.PrivateKey
	tvIssuer string
)

// tvSetup starts the fake provider and runs the real InitKeycloak once per
// test binary. The provider advertises the same id_token signing algorithms
// as Keycloak 26 (incl. HS* and RS512), so the verifier gets the same
// algorithm list as in production.
func tvSetup(t *testing.T) {
	t.Helper()
	tvOnce.Do(func() {
		var err error
		tvKey, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		mux := http.NewServeMux()
		srv := httptest.NewServer(mux) // lives for the whole test binary
		tvIssuer = srv.URL + "/realms/" + tvRealm
		mux.HandleFunc("/realms/"+tvRealm+"/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                 tvIssuer,
				"authorization_endpoint": tvIssuer + "/protocol/openid-connect/auth",
				"token_endpoint":         tvIssuer + "/protocol/openid-connect/token",
				"jwks_uri":               tvIssuer + "/protocol/openid-connect/certs",
				"id_token_signing_alg_values_supported": []string{
					"PS384", "RS384", "EdDSA", "ES384", "HS256", "HS512", "ES256",
					"RS256", "HS384", "ES512", "PS256", "PS512", "RS512",
				},
			})
		})
		mux.HandleFunc("/realms/"+tvRealm+"/protocol/openid-connect/certs", func(w http.ResponseWriter, r *http.Request) {
			pub := tvKey.PublicKey
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kid": tvKid, "kty": "RSA", "alg": "RS256", "use": "sig",
				"n": b64(pub.N.Bytes()),
				"e": b64(big.NewInt(int64(pub.E)).Bytes()),
			}}})
		})

		dir, err := os.MkdirTemp("", "kc-test")
		if err != nil {
			panic(err)
		}
		defer os.RemoveAll(dir)
		cfg := fmt.Sprintf(`{
			"api": {"resource": "api-client", "secret": "x", "realm": %q, "auth-server-url": %q},
			"app": {"resource": "app-client", "realm": %q, "auth-server-url": %q}
		}`, tvRealm, srv.URL, tvRealm, srv.URL)
		path := filepath.Join(dir, "keycloak.json")
		if err := os.WriteFile(path, []byte(cfg), 0o600); err != nil {
			panic(err)
		}
		prev, had := os.LookupEnv("KEYCLOAK_CONFIG")
		os.Setenv("KEYCLOAK_CONFIG", path)
		InitKeycloak()
		if had {
			os.Setenv("KEYCLOAK_CONFIG", prev)
		} else {
			os.Unsetenv("KEYCLOAK_CONFIG")
		}
	})
	// Use the real verifier, not a test hook of another test.
	orig := VerifyTokenClaims
	VerifyTokenClaims = nil
	t.Cleanup(func() { VerifyTokenClaims = orig })
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// tvClaims returns a valid claim set; tests change single entries.
func tvClaims() map[string]any {
	now := time.Now()
	return map[string]any{
		"iss":                tvIssuer,
		"sub":                "sub-123",
		"aud":                "account",
		"azp":                "app-client",
		"exp":                now.Add(5 * time.Minute).Unix(),
		"iat":                now.Unix(),
		"tenant":             []string{"TE100100"},
		"access_groups":      []string{"/EEG_ADMIN"},
		"preferred_username": "admin",
	}
}

// tvSign builds a compact JWS. alg selects the signature: RS256/RS512 with
// key, HS256 with secret, none without signature.
func tvSign(t *testing.T, alg string, key *rsa.PrivateKey, secret []byte, claims map[string]any) string {
	t.Helper()
	h, err := json.Marshal(map[string]string{"alg": alg, "typ": "JWT", "kid": tvKid})
	require.NoError(t, err)
	p, err := json.Marshal(claims)
	require.NoError(t, err)
	input := b64(h) + "." + b64(p)
	var sig []byte
	switch alg {
	case "RS256":
		d := sha256.Sum256([]byte(input))
		sig, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, d[:])
	case "RS512":
		d := sha512.Sum512([]byte(input))
		sig, err = rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA512, d[:])
	case "HS256":
		m := hmac.New(sha256.New, secret)
		m.Write([]byte(input))
		sig = m.Sum(nil)
	case "none":
	default:
		t.Fatalf("unknown alg %s", alg)
	}
	require.NoError(t, err)
	return input + "." + b64(sig)
}

func tvVerify(t *testing.T, token string) (*PlatformClaims, error) {
	t.Helper()
	return verifyAndExtractClaims(t.Context(), token)
}

func TestTokenValidation_ValidRS256Accepted(t *testing.T) {
	tvSetup(t)
	c, err := tvVerify(t, tvSign(t, "RS256", tvKey, nil, tvClaims()))
	require.NoError(t, err)
	require.Equal(t, []string{"TE100100"}, c.Tenants)
	require.Equal(t, "sub-123", c.Subject)
	require.Equal(t, "app-client", c.Authorized)
	require.True(t, c.AccessGroups.IsAdmin())
}

func TestTokenValidation_Expiry(t *testing.T) {
	tvSetup(t)
	cases := []struct {
		name   string
		exp    any // nil: claim removed
		accept bool
	}{
		{"expired 1 minute ago", time.Now().Add(-time.Minute).Unix(), false},
		{"expired 1 second ago (no leeway)", time.Now().Add(-time.Second).Unix(), false},
		// exp only a few seconds ahead is accepted: no negative leeway / clock offset.
		{"expires in 3 seconds", time.Now().Add(3 * time.Second).Unix(), true},
		{"no exp", nil, false},
		{"exp 0", 0, false},
		// Claim decoding into PlatformClaims (int64 exp) rejects a fractional exp.
		{"exp fractional", float64(time.Now().Add(time.Minute).Unix()) + 0.5, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := tvClaims()
			if tc.exp == nil {
				delete(claims, "exp")
			} else {
				claims["exp"] = tc.exp
			}
			_, err := tvVerify(t, tvSign(t, "RS256", tvKey, nil, claims))
			if tc.accept {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestTokenValidation_NotBefore(t *testing.T) {
	tvSetup(t)
	// go-oidc allows 5 minutes clock skew on nbf.
	claims := tvClaims()
	claims["nbf"] = time.Now().Add(time.Minute).Unix()
	_, err := tvVerify(t, tvSign(t, "RS256", tvKey, nil, claims))
	require.NoError(t, err)

	claims["nbf"] = time.Now().Add(10 * time.Minute).Unix()
	_, err = tvVerify(t, tvSign(t, "RS256", tvKey, nil, claims))
	require.Error(t, err)
}

func TestTokenValidation_WrongKeyRejected(t *testing.T) {
	tvSetup(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	_, err = tvVerify(t, tvSign(t, "RS256", other, nil, tvClaims()))
	require.Error(t, err)
}

func TestTokenValidation_TamperedPayloadRejected(t *testing.T) {
	tvSetup(t)
	good := tvSign(t, "RS256", tvKey, nil, tvClaims())
	claims := tvClaims()
	claims["tenant"] = []string{"TE999999"}
	forged := tvSign(t, "RS256", tvKey, nil, claims)
	// payload of the forged token with the signature of the good one
	g, f := strings.Split(good, "."), strings.Split(forged, ".")
	_, err := tvVerify(t, f[0]+"."+f[1]+"."+g[2])
	require.Error(t, err)
}

func TestTokenValidation_AlgNoneRejected(t *testing.T) {
	tvSetup(t)
	_, err := tvVerify(t, tvSign(t, "none", nil, nil, tvClaims()))
	require.Error(t, err)
}

func TestTokenValidation_HS256WithPublicKeyRejected(t *testing.T) {
	tvSetup(t)
	der, err := x509.MarshalPKIXPublicKey(&tvKey.PublicKey)
	require.NoError(t, err)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	for name, secret := range map[string][]byte{"pem": pemKey, "der": der} {
		t.Run(name, func(t *testing.T) {
			_, err := tvVerify(t, tvSign(t, "HS256", nil, secret, tvClaims()))
			require.Error(t, err)
		})
	}
}

// RS512 with the realm key is accepted today: go-oidc takes the algorithms
// from the provider discovery (Keycloak advertises RS512) and drops only the
// HMAC ones. This is pinned here so a library change does not alter it
// silently; it is not a statement that it is wanted.
func TestTokenValidation_RS512WithRealmKeyAccepted(t *testing.T) {
	tvSetup(t)
	_, err := tvVerify(t, tvSign(t, "RS512", tvKey, nil, tvClaims()))
	require.NoError(t, err)
}

func TestTokenValidation_WrongIssuerRejected(t *testing.T) {
	tvSetup(t)
	for _, iss := range []any{"https://evil.example.org/realms/" + tvRealm, tvIssuer + "/", nil} {
		claims := tvClaims()
		if iss == nil {
			delete(claims, "iss")
		} else {
			claims["iss"] = iss
		}
		_, err := tvVerify(t, tvSign(t, "RS256", tvKey, nil, claims))
		require.Error(t, err, "iss=%v", iss)
	}
}

func TestTokenValidation_Audience(t *testing.T) {
	tvSetup(t)
	cases := []struct {
		name   string
		aud    any // nil: claim removed
		accept bool
	}{
		// SkipClientIDCheck: the audience value is not checked.
		{"string", "account", true},
		{"other client", "some-other-client", true},
		{"absent", nil, true},
		// Claim decoding into PlatformClaims (string aud) rejects an array.
		{"array", []string{"account", "app-client"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := tvClaims()
			if tc.aud == nil {
				delete(claims, "aud")
			} else {
				claims["aud"] = tc.aud
			}
			_, err := tvVerify(t, tvSign(t, "RS256", tvKey, nil, claims))
			if tc.accept {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// Tenant forms through the Protect middleware (REST path).
func TestTokenValidation_TenantClaim(t *testing.T) {
	tvSetup(t)
	cases := []struct {
		name   string
		tenant any // nil: claim removed
		header string
		status int
	}{
		{"array exact", []string{"TE100100"}, "TE100100", http.StatusOK},
		{"array mixed case vs header", []string{"Te100100"}, "te100100", http.StatusOK},
		{"array several", []string{"RC100200", "te100100"}, "TE100100", http.StatusOK},
		{"array other tenant", []string{"TE999999"}, "TE100100", http.StatusForbidden},
		{"empty array", []string{}, "TE100100", http.StatusForbidden},
		{"missing", nil, "TE100100", http.StatusForbidden},
		{"string instead of array", "TE100100", "TE100100", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := tvClaims()
			if tc.tenant == nil {
				delete(claims, "tenant")
			} else {
				claims["tenant"] = tc.tenant
			}
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", BEARER_SCHEMA+tvSign(t, "RS256", tvKey, nil, claims))
			req.Header.Set("tenant", tc.header)
			rw := httptest.NewRecorder()
			var gotTenant string
			Protect(func(w http.ResponseWriter, r *http.Request, c *PlatformClaims, tenant string) {
				gotTenant = tenant
				w.WriteHeader(http.StatusOK)
			})(rw, req)
			require.Equal(t, tc.status, rw.Code)
			if tc.status == http.StatusOK {
				require.Equal(t, "TE100100", gotTenant)
			}
		})
	}
}

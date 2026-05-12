package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	err := os.Setenv("KEYCLOAK_CONFIG", "../../keycloak.json")
	if err != nil {
		panic(err)
	}
	InitKeycloak()
	code := m.Run()
	os.Exit(code)
}

func TestProtectMiddleware_ValidToken_AllowsRequest(t *testing.T) {
	orig := VerifyTokenClaims
	defer func() { VerifyTokenClaims = orig }()

	VerifyTokenClaims = func(ctx context.Context, token string) (*PlatformClaims, error) {
		return &PlatformClaims{
			Tenants:      []string{"TE100100"},
			AccessGroups: AccessGroups{"/EEG_ADMIN"},
			RealmAccess: struct {
				Roles []string `json:"roles"`
			}(struct{ Roles []string }{Roles: []string{}}),
		}, nil
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", BEARER_SCHEMA+"token")
	req.Header.Set("tenant", "TE100100")

	rw := httptest.NewRecorder()
	h := Protect(func(w http.ResponseWriter, r *http.Request, claims *PlatformClaims, tenant string) {
		w.WriteHeader(http.StatusOK)
	})

	h(rw, req)
	require.Equal(t, http.StatusOK, rw.Code)
}

func TestProtectMiddleware_InvalidToken_ReturnsUnauthorized(t *testing.T) {
	orig := VerifyTokenClaims
	defer func() { VerifyTokenClaims = orig }()

	VerifyTokenClaims = func(ctx context.Context, token string) (*PlatformClaims, error) {
		return nil, http.ErrNoCookie // arbitrary error to simulate verification failure
	}

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", BEARER_SCHEMA+"token")
	req.Header.Set("tenant", "TE100100")

	rw := httptest.NewRecorder()
	h := Protect(func(w http.ResponseWriter, r *http.Request, claims *PlatformClaims, tenant string) {
		w.WriteHeader(http.StatusOK)
	})

	h(rw, req)
	require.Equal(t, http.StatusUnauthorized, rw.Code)
}

func TestConditionProtect_RoutesToAdminOrUser(t *testing.T) {
	orig := VerifyTokenClaims
	defer func() { VerifyTokenClaims = orig }()

	// Admin case
	VerifyTokenClaims = func(ctx context.Context, token string) (*PlatformClaims, error) {
		return &PlatformClaims{
			Tenants:      []string{"TEA"},
			AccessGroups: AccessGroups{"/EEG_ADMIN"},
			RealmAccess: struct {
				Roles []string `json:"roles"`
			}(struct{ Roles []string }{Roles: []string{}}),
		}, nil
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", BEARER_SCHEMA+"token")
	req.Header.Set("tenant", "TEA")
	rw := httptest.NewRecorder()

	adminCalled := false
	userCalled := false
	h := ConditionProtect(func(w http.ResponseWriter, r *http.Request, claims *PlatformClaims, tenant string) {
		adminCalled = true
		w.WriteHeader(http.StatusOK)
	}, func(w http.ResponseWriter, r *http.Request, claims *PlatformClaims, tenant string) {
		userCalled = true
		w.WriteHeader(http.StatusOK)
	})

	h(rw, req)
	require.True(t, adminCalled)
	require.False(t, userCalled)

	// User case
	VerifyTokenClaims = func(ctx context.Context, token string) (*PlatformClaims, error) {
		return &PlatformClaims{
			Tenants:      []string{"TEA"},
			AccessGroups: AccessGroups{"/EEG_USER"},
			RealmAccess: struct {
				Roles []string `json:"roles"`
			}(struct{ Roles []string }{Roles: []string{}}),
		}, nil
	}
	rw2 := httptest.NewRecorder()
	h(rw2, req)
	require.True(t, userCalled)
}

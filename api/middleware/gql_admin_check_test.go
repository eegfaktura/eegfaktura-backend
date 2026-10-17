package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
	"github.com/stretchr/testify/require"
)

const gqlAdminCheckMsg = "gql-admin-check: non-admin token"

func runGQLProtect(t *testing.T, claims *PlatformClaims) (int, []*logrus.Entry) {
	t.Helper()
	orig := VerifyTokenClaims
	defer func() { VerifyTokenClaims = orig }()
	VerifyTokenClaims = func(ctx context.Context, token string) (*PlatformClaims, error) {
		return claims, nil
	}

	hook := logtest.NewGlobal()
	defer logrus.StandardLogger().ReplaceHooks(make(logrus.LevelHooks))

	req := httptest.NewRequest("POST", "/query", nil)
	req.Header.Set("Authorization", BEARER_SCHEMA+"token")
	req.Header.Set("tenant", "TE100100")
	rw := httptest.NewRecorder()

	GQLProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rw, req)

	var hits []*logrus.Entry
	for _, e := range hook.AllEntries() {
		if e.Message == gqlAdminCheckMsg {
			hits = append(hits, e)
		}
	}
	return rw.Code, hits
}

func TestGQLProtect_NonAdminIsLoggedAndPasses(t *testing.T) {
	code, hits := runGQLProtect(t, &PlatformClaims{
		Tenants:        []string{"te100100"},
		Username:       "member@example.org",
		Email:          "member@example.org",
		AccessGroups:   AccessGroups{"/EEG_USER"},
		Authorized:     "some-client",
		StandardClaims: jwt.StandardClaims{Subject: "sub-123"},
	})

	require.Equal(t, http.StatusOK, code)
	require.Len(t, hits, 1)
	e := hits[0]
	require.Equal(t, logrus.WarnLevel, e.Level)
	require.Equal(t, "TE100100", e.Data["tenant"])
	require.Equal(t, "some-client", e.Data["azp"])
	// preferred_username looks like an e-mail -> sub is logged instead
	require.Equal(t, "sub-123", e.Data["user"])
	require.Equal(t, true, e.Data["access_groups_present"])
	for _, v := range e.Data {
		require.NotContains(t, fmt.Sprint(v), "@")
	}
}

func TestGQLProtect_NoAccessGroupsIsLogged(t *testing.T) {
	code, hits := runGQLProtect(t, &PlatformClaims{
		Tenants:    []string{"TE100100"},
		Username:   "service-account-x",
		Authorized: "x",
	})

	require.Equal(t, http.StatusOK, code)
	require.Len(t, hits, 1)
	require.Equal(t, "service-account-x", hits[0].Data["user"])
	require.Equal(t, false, hits[0].Data["access_groups_present"])
}

func TestGQLProtect_AdminIsNotLogged(t *testing.T) {
	code, hits := runGQLProtect(t, &PlatformClaims{
		Tenants:      []string{"TE100100"},
		AccessGroups: AccessGroups{"/EEG_ADMIN"},
	})
	require.Equal(t, http.StatusOK, code)
	require.Empty(t, hits)
}

func TestGQLProtect_SuperuserIsNotLogged(t *testing.T) {
	code, hits := runGQLProtect(t, &PlatformClaims{
		RealmAccess: RealmRoles{Roles: []string{"superuser"}},
	})
	require.Equal(t, http.StatusOK, code)
	require.Empty(t, hits)
}

func TestGQLProtect_WrongTenantStillRejected(t *testing.T) {
	code, hits := runGQLProtect(t, &PlatformClaims{
		Tenants:      []string{"TE999999"},
		AccessGroups: AccessGroups{"/EEG_USER"},
	})
	require.Equal(t, http.StatusUnauthorized, code)
	require.Empty(t, hits)
}

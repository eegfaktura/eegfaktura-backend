package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ProtectSuperuser (bulk change): only the realm role superuser passes, with or without the admin access group;
// the admin group alone is not enough. Missing token 403, invalid token 401 as in Protect.
func TestProtectSuperuser(t *testing.T) {
	orig := VerifyTokenClaims
	t.Cleanup(func() { VerifyTokenClaims = orig })

	claimsFor := map[string]*PlatformClaims{
		"superuser":         {Username: "support", RealmAccess: RealmRoles{Roles: []string{"superuser"}}},
		"superuser+admin":   {Username: "support", AccessGroups: AccessGroups{"/EEG_ADMIN"}, RealmAccess: RealmRoles{Roles: []string{"superuser"}}},
		"admin":             {Username: "eegadmin", Tenants: []string{"TE000001"}, AccessGroups: AccessGroups{"/EEG_ADMIN"}},
		"admin+other-roles": {Username: "eegadmin", Tenants: []string{"TE000001"}, AccessGroups: AccessGroups{"/EEG_ADMIN"}, RealmAccess: RealmRoles{Roles: []string{"offline_access", "Superuser"}}},
	}
	VerifyTokenClaims = func(_ context.Context, token string) (*PlatformClaims, error) {
		if c, ok := claimsFor[token]; ok {
			cp := *c
			return &cp, nil
		}
		return nil, errors.New("bad token")
	}

	cases := []struct {
		name, auth   string
		want         int
		wantTenant   string
		wantUsername string
	}{
		{name: "superuser without admin group", auth: "Bearer superuser", want: http.StatusOK, wantTenant: "TE000001", wantUsername: "support"},
		{name: "superuser with admin group", auth: "Bearer superuser+admin", want: http.StatusOK, wantTenant: "TE000001", wantUsername: "support"},
		{name: "admin group without superuser", auth: "Bearer admin", want: http.StatusForbidden},
		{name: "role names are exact", auth: "Bearer admin+other-roles", want: http.StatusForbidden},
		{name: "invalid token", auth: "Bearer nope", want: http.StatusUnauthorized},
		{name: "missing token", auth: "", want: http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/bulkchange/communities", nil)
			if c.auth != "" {
				req.Header.Set("Authorization", c.auth)
			}
			req.Header.Set("tenant", "te000001")
			var gotTenant, gotUser string
			rw := httptest.NewRecorder()
			ProtectSuperuser(func(w http.ResponseWriter, _ *http.Request, claims *PlatformClaims, tenant string) {
				gotTenant, gotUser = tenant, claims.Username
				w.WriteHeader(http.StatusOK)
			})(rw, req)
			assert.Equal(t, c.want, rw.Code)
			assert.Equal(t, c.wantTenant, gotTenant, "tenant upper-cased, handler only on success")
			assert.Equal(t, c.wantUsername, gotUser)
		})
	}
}

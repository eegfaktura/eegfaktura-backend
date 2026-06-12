package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt"
	"github.com/sirupsen/logrus"
)

var (
	// VerifyTokenClaims pluggable verification function used by middlewares and tests.
	// InitKeycloak should set this to the real verifier at startup.
	// Tests override it to avoid network calls.
	VerifyTokenClaims func(ctx context.Context, token string) (*PlatformClaims, error)
)

type PlatformClaims struct {
	Tenants      []string     `json:"tenant"`
	Username     string       `json:"preferred_username"`
	Email        string       `json:"email"`
	AccessGroups AccessGroups `json:"access_groups"`
	Authorized   string       `json:"azp"`
	RealmAccess  RealmRoles   `json:"realm_access"`
	jwt.StandardClaims
}

type RealmRoles struct {
	Roles []string `json:"roles"`
}

func (rr RealmRoles) HasRole(role string) bool {
	return hasRole(rr.Roles, role)
}

type AccessGroups []string

type VerifyError struct {
	StatusCode int
	Err        error
}

func (eve *VerifyError) Error() string {
	return fmt.Sprintf("status %d: err %v", eve.StatusCode, eve.Err)
}

//func (ag AccessGroups) IsAdmin() bool {
//	for _, s := range ag {
//		if s == "/EEG_ADMIN" {
//			return true
//		}
//	}
//	return false
//}
//
//func (ag AccessGroups) IsUser() bool {
//	for _, s := range ag {
//		if s == "/EEG_USER" {
//			return true
//		}
//	}
//	return false
//}

// verifyAndExtractClaims tries the test-hook VerifyTokenClaims (if present) and
// falls back to the actual OIDC verifier otherwise. It always returns a populated
// PlatformClaims or an error.
func verifyAndExtractClaims(ctx context.Context, token string) (*PlatformClaims, error) {
	// If a test hook / custom verifier is provided, use it.
	if VerifyTokenClaims != nil {
		return VerifyTokenClaims(ctx, token)
	}

	// Fallback to OIDC verifier
	if verifier == nil {
		return nil, fmt.Errorf("oidc verifier not initialized")
	}
	idToken, err := verifier.Verify(ctx, token)
	if err != nil {
		return nil, err
	}
	claims := &PlatformClaims{}
	if err := idToken.Claims(claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func GQLProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwtToken, err := ParseBearerTokenFromHeader(r)
		if err != nil {
			logrus.Printf("No Access_token in request or invalid Authorization: %v\n", err)
			w.WriteHeader(http.StatusForbidden)
			return
		}

		claims, err := verifyAndExtractClaims(context.Background(), jwtToken)
		if err != nil {
			logrus.WithField("error", "JWT-Token").Errorf("%v", err)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(err.Error()))
			return
		}

		//fmt.Printf("Claims: %+v\n", claims)
		tenant := r.Header.Get("tenant")
		if len(tenant) == 0 {
			tenant = r.Header.Get("X-Tenant")
		}
		superuser := hasRole(claims.RealmAccess.Roles, "superuser")
		if !superuser {
			if contains(claims.Tenants, tenant) == false {
				logrus.WithField("tenant", tenant).Warnf("Unauthorized access with tenant %s", tenant)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		// put it in context
		ctx := context.WithValue(
			context.WithValue(r.Context(), tenantCtxKey, strings.ToUpper(tenant)),
			superUserCtxKey, superuser)

		logrus.Printf("Access granted for tenant %s (%s)", tenant, r.URL.Path)
		// and call the next with our new context
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func retrieveClaims(r *http.Request) (string, *PlatformClaims, error) {
	jwtToken, err := ParseBearerTokenFromHeader(r)
	if err != nil {
		logrus.WithField("error", "JWT-Token").Printf("No Access_token in request or invalid Authorization: %v\n", err)
		return "", nil, &VerifyError{http.StatusForbidden, errors.New("no access token available")}
	}

	claims, err := verifyAndExtractClaims(context.Background(), jwtToken)
	if err != nil {
		logrus.WithField("error", "JWT-Token").Errorf("%v", err)
		return "", nil, &VerifyError{http.StatusUnauthorized, err}
	}

	tenant := r.Header.Get("tenant")
	if len(tenant) == 0 {
		tenant = r.Header.Get("X-Tenant")
	}
	superuser := hasRole(claims.RealmAccess.Roles, "superuser")
	if !superuser {
		if contains(claims.Tenants, tenant) == false {
			logrus.WithField("tenant", tenant).Warnf("Unauthorized access with tenant %s", tenant)
			return tenant, nil, &VerifyError{http.StatusForbidden, errors.New("unauthorized access")}
		}
	}

	return tenant, claims, nil
}

// ConditionProtect Routes respectively to AccessGroups. Distinguish between admin route and user Route. ToDo: check body for refactoring.
func ConditionProtect(admin JWTHandlerFunc, user JWTHandlerFunc) http.HandlerFunc {
	toUpper := func(ss []string) []string {
		rss := make([]string, len(ss))
		for i, s := range ss {
			rss[i] = strings.ToUpper(s)
		}
		return rss
	}

	return func(w http.ResponseWriter, r *http.Request) {

		tenant, claims, err := retrieveClaims(r)
		if err != nil {
			w.WriteHeader(err.(*VerifyError).StatusCode)
			return
		}

		claims.Tenants = toUpper(claims.Tenants)
		if hasRole(claims.RealmAccess.Roles, "admin") {
			admin(w, r, claims, strings.ToUpper(tenant))
		} else if hasRole(claims.RealmAccess.Roles, "user") {
			user(w, r, claims, strings.ToUpper(tenant))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}
}

//func UserMiddelware(handler TenantHandlerFunc) http.HandlerFunc {
//	return func(w http.ResponseWriter, r *http.Request) {
//
//		tenant := r.Header.Get("X-Tenant")
//		userId := r.Header.Get("X-User-ID")
//		mail := r.Header.Get("X-Mail")
//
//		if len(tenant) <= 0 {
//			logrus.WithField("tenant", tenant).Warn("unauthorized tenant")
//			http.Error(w, "forbidden", http.StatusForbidden)
//			return
//		}
//
//		handler(w, r, &BackendClaims{Tenant: strings.ToUpper(tenant), UserId: userId, Mail: mail})
//	}
//}

func Protect(handler JWTHandlerFunc) http.HandlerFunc {
	return verifyRequest(handler)
}

func ProtectApi(handler JWTHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, err := ParseBasicCredentialsFromHeader(r)
		if err != nil {
			logrus.WithField("error", err.Error()).Warn("invalid Authorization header (basic)")
			// Distinguish between missing header and invalid schema/encoding if needed
			if errors.Is(err, ErrNoAuthorization) {
				w.WriteHeader(http.StatusForbidden)
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
			return
		}

		idToken, err := kcClientAPI.AuthenticateUserWithPassword(username, password)
		if err != nil {
			logrus.WithError(err).Error("authentication failed")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var claims PlatformClaims
		if err := idToken.Claims(&claims); err != nil {
			logrus.WithError(err).Error("failed to parse claims")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		tenant := r.Header.Get("X-Tenant")
		if !contains(claims.Tenants, tenant) {
			logrus.WithField("tenant", tenant).Warn("unauthorized tenant")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		handler(w, r, &claims, strings.ToUpper(tenant))
	}
}

func verifyRequest(handler JWTHandlerFunc) func(w http.ResponseWriter, r *http.Request) {
	toUpper := func(ss []string) []string {
		rss := make([]string, len(ss))
		for i, s := range ss {
			rss[i] = strings.ToUpper(s)
		}
		return rss
	}

	return func(w http.ResponseWriter, r *http.Request) {
		tenant, claims, err := retrieveClaims(r)
		if err != nil {
			w.WriteHeader(err.(*VerifyError).StatusCode)
			return
		}

		if hasRole(claims.RealmAccess.Roles, "admin") {
			claims.Tenants = toUpper(claims.Tenants)
			handler(w, r, claims, strings.ToUpper(tenant))
		} else {
			logrus.WithField("tenant", tenant).Warnf("Unauthorized access with tenant %s - Request has no role admin", tenant)
			w.WriteHeader(http.StatusUnauthorized)
		}
	}
}

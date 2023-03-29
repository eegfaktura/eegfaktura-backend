package middleware

import (
	"context"
	log "github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

var tenantCtxKey = &contextKey{"tenant"}

type contextKey struct {
	name string
}

func GQLMiddleware(publicKeyPath string) func(http.Handler) http.Handler {
	pub, err := loadEncryptionKey(publicKeyPath)
	if err != nil {
		log.Fatal(err)
	}
	jwtUtil := &AccessTokenGenJWT{PublicKey: pub.key}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			jwtToken := r.Header.Get("Authorization")
			if len(jwtToken) == 0 {
				log.Printf("No Access_token in request!\n")
				w.WriteHeader(http.StatusForbidden)
				return
			}

			if strings.HasPrefix(jwtToken, BEARER_SCHEMA) {
				jwtToken = jwtToken[len(BEARER_SCHEMA):]
			} else {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			claims, err := jwtUtil.ExtractClaims(jwtToken)
			if err != nil {
				log.Printf("Error while parsing token: %s\n", err)
				w.WriteHeader(http.StatusForbidden)
				return
			}

			tenant := r.Header.Get("tenant")
			if contains(claims.Tenants, tenant) == false {
				w.WriteHeader(http.StatusForbidden)
				return
			}

			// put it in context
			ctx := context.WithValue(r.Context(), tenantCtxKey, strings.ToUpper(tenant))

			// and call the next with our new context
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

func ForContextTenant(ctx context.Context) string {
	raw, _ := ctx.Value(tenantCtxKey).(string)
	return raw
}

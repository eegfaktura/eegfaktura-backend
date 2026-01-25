package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/sirupsen/logrus"
)

var (
	kcConfig map[string]*keycloakConfig
	verifier *oidc.IDTokenVerifier
)

func InitKeycloak() {
	var err error
	kcConfig, err = readKeycloakConfig()
	if err != nil {
		logrus.WithField("error", "Init Keycloak").Errorf("E: %v", err)
		panic(err)
	}

	if _, ok := kcConfig["api"]; !ok {
		logrus.WithField("error", "Init Keycloak").Errorf("E: %v", errors.New("no client-id 'at.ourproject.vfeeg.api' available"))
		panic(err)
	}

	clientIDApi := kcConfig["api"].ClientId
	clientSecretApi := kcConfig["api"].Secret

	realmApi := kcConfig["api"].Realm
	host := strings.TrimRight(kcConfig["api"].Host, "/")
	issuerUrl := kcConfig["api"].IssuerUrl

	c := &http.Client{Timeout: time.Duration(1) * time.Second}
	kcClientAPI, err = NewKeycloakClient(fmt.Sprintf("%s/realms/%s", host, realmApi), clientIDApi, clientSecretApi, issuerUrl, c)
	if err != nil {
		panic(err)
	}

	/**
	set up jwt token verifier
	*/
	clientIDApp := kcConfig["app"].ClientId
	realmApp := kcConfig["app"].Realm
	issuerUrl = kcConfig["app"].IssuerUrl

	hostApp := strings.TrimRight(kcConfig["app"].Host, "/")

	ctx := context.Background()
	if issuerUrl != "" {
		ctx = oidc.InsecureIssuerURLContext(ctx, issuerUrl)
	}

	providerUriApp := fmt.Sprintf("%s/realms/%s", hostApp, realmApp)
	provider, err := oidc.NewProvider(ctx, providerUriApp)
	if err != nil {
		logrus.Errorf("E: %v", err)
	}
	verifier = provider.Verifier(&oidc.Config{ClientID: clientIDApp, SkipClientIDCheck: true})
}

func readKeycloakConfig() (map[string]*keycloakConfig, error) {
	kcPath, ok := os.LookupEnv("KEYCLOAK_CONFIG")
	if !ok {
		kcPath = "./keycloak.json"
	}
	kcConfigFile, err := os.Open(kcPath)
	if err != nil {
		return nil, err
	}
	defer kcConfigFile.Close()

	payload, err := io.ReadAll(kcConfigFile)
	if err != nil {
		return nil, err
	}

	kcConfig := map[string]*keycloakConfig{}
	err = json.Unmarshal(payload, &kcConfig)
	return kcConfig, err
}

type keycloakConfig struct {
	ClientId  string `json:"resource"`
	Secret    string `json:"secret,omitempty"`
	Realm     string `json:"realm"`
	Host      string `json:"auth-server-url"`
	IssuerUrl string `json:"issuer_url,omitempty"`
}

//func InitKeycloak() {
//	kcConfig, err := readKeycloakConfig()
//	if err != nil {
//		logrus.WithField("error", "Init Keycloak").Errorf("E: %v", err)
//		panic(err)
//	}
//
//	clientID := kcConfig.ClientId
//	realm := kcConfig.Realm
//	host := strings.TrimRight(kcConfig.Host, "/")
//
//	ctx := context.Background()
//	provider, err := oidc.NewProvider(ctx, fmt.Sprintf("%s/realms/%s", host, realm))
//	if err != nil {
//		logrus.Errorf("E: %v", err)
//		//panic(err)
//	}
//	verifier = provider.Verifier(&oidc.Config{ClientID: clientID, SkipClientIDCheck: true})
//}

//func readKeycloakConfig() (*keycloakConfig, error) {
//	kcPath, ok := os.LookupEnv("KEYCLOAK_CONFIG")
//	if !ok {
//		kcPath = "./keycloak.json"
//	}
//	kcConfigFile, err := os.Open(kcPath)
//	if err != nil {
//		return nil, err
//	}
//	defer kcConfigFile.Close()
//
//	payload, err := io.ReadAll(kcConfigFile)
//	if err != nil {
//		return nil, err
//	}
//
//	kcConfig := map[string]*keycloakConfig{}
//	err = json.Unmarshal(payload, &kcConfig)
//
//	return kcConfig["app"], err
//}

func GQLProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jwtToken := r.Header.Get("Authorization")
		if len(jwtToken) == 0 {
			logrus.Printf("No Access_token in request!\n")
			w.WriteHeader(http.StatusForbidden)
			return
		}

		if strings.HasPrefix(jwtToken, BEARER_SCHEMA) {
			jwtToken = jwtToken[len(BEARER_SCHEMA):]
		} else {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		idToken, err := verifier.Verify(context.Background(), jwtToken)
		if err != nil {
			logrus.WithField("error", "JWT-Token").Errorf("%v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		claims := PlatformClaims{}
		if err := idToken.Claims(&claims); err != nil {
			logrus.WithField("error", "Claims").Errorf("%v", err)
			w.WriteHeader(http.StatusUnauthorized)
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

		// and call the next with our new context
		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}

func hasRole(roles []string, role string) bool {
	return slices.Contains(roles, role)
}

//func Protect(handler JWTHandlerFunc) http.HandlerFunc {
//	return func(w http.ResponseWriter, r *http.Request) {
//		jwtToken := r.Header.Get("Authorization")
//		if len(jwtToken) == 0 {
//			logrus.WithField("error", "JWT-Token").Printf("No Access_token in request!\n")
//			w.WriteHeader(http.StatusForbidden)
//			return
//		}
//
//		if strings.HasPrefix(jwtToken, BEARER_SCHEMA) {
//			jwtToken = jwtToken[len(BEARER_SCHEMA):]
//		} else {
//			w.WriteHeader(http.StatusBadRequest)
//			return
//		}
//
//		idToken, err := verifier.Verify(context.Background(), jwtToken)
//		if err != nil {
//			logrus.WithField("error", "JWT-Token").Errorf("%v", err)
//			w.WriteHeader(http.StatusForbidden)
//			return
//		}
//
//		claims := PlatformClaims{}
//		if err := idToken.Claims(&claims); err != nil {
//			logrus.WithField("error", "Claims").Errorf("%v", err)
//			w.WriteHeader(http.StatusForbidden)
//			return
//		}
//
//		tenant := r.Header.Get("tenant")
//		if contains(claims.Tenants, tenant) == false {
//			logrus.WithField("tenant", tenant).Warnf("Unauthorized access with tenant %s", tenant)
//			w.WriteHeader(http.StatusForbidden)
//			return
//		}
//
//		handler(w, r, &claims, strings.ToUpper(tenant))
//	}
//}

// ConditionProtect Routes respectivly to AccessGroups. Distinguish between admin route and user Route. ToDo: check body for refactoring.
func ConditionProtect(admin JWTHandlerFunc, user JWTHandlerFunc) http.HandlerFunc {
	toUpper := func(ss []string) []string {
		rss := make([]string, len(ss))
		for i, s := range ss {
			rss[i] = strings.ToUpper(s)
		}
		return rss
	}

	return func(w http.ResponseWriter, r *http.Request) {
		jwtToken := r.Header.Get("Authorization")
		if len(jwtToken) == 0 {
			logrus.WithField("error", "JWT-Token").Printf("No Access_token in request!\n")
			w.WriteHeader(http.StatusForbidden)
			return
		}

		if strings.HasPrefix(jwtToken, BEARER_SCHEMA) {
			jwtToken = jwtToken[len(BEARER_SCHEMA):]
		} else {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		idToken, err := verifier.Verify(context.Background(), jwtToken)
		if err != nil {
			logrus.WithField("error", "JWT-Token").Errorf("%v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		claims := PlatformClaims{}
		if err := idToken.Claims(&claims); err != nil {
			logrus.WithField("error", "Claims").Errorf("%v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

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

		claims.Tenants = toUpper(claims.Tenants)
		if claims.AccessGroups.IsAdmin() {
			admin(w, r, &claims, strings.ToUpper(tenant))
		} else if claims.AccessGroups.IsUser() {
			user(w, r, &claims, strings.ToUpper(tenant))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}
}

func Protect(handler JWTHandlerFunc) http.HandlerFunc {
	return verifyRequest(handler)
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
		jwtToken := r.Header.Get("Authorization")
		if len(jwtToken) == 0 {
			logrus.WithField("error", "JWT-Token").Printf("No Access_token in request!\n")
			w.WriteHeader(http.StatusForbidden)
			return
		}

		if strings.HasPrefix(jwtToken, BEARER_SCHEMA) {
			jwtToken = jwtToken[len(BEARER_SCHEMA):]
		} else {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		idToken, err := verifier.Verify(context.Background(), jwtToken)
		if err != nil {
			logrus.WithField("error", "JWT-Token").Errorf("%v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		claims := PlatformClaims{}
		if err := idToken.Claims(&claims); err != nil {
			logrus.WithField("error", "Claims").Errorf("%v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

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

		if claims.AccessGroups.IsAdmin() {
			claims.Tenants = toUpper(claims.Tenants)
			handler(w, r, &claims, strings.ToUpper(tenant))
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}

	}
}

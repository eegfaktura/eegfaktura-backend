package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"os"
	"strings"
)

var (
	verifier *oidc.IDTokenVerifier
)

type keycloakConfig struct {
	ClientId string `json:"resource"`
	Realm    string `json:"realm"`
	Host     string `json:"auth-server-url"`
}

func init() {
	kcConfig, err := readKeycloakConfig()
	if err != nil {
		logrus.WithField("error", "Init Keycloak").Errorf("E: %v", err)
		panic(err)
	}

	clientID := kcConfig.ClientId
	realm := kcConfig.Realm
	host := strings.TrimRight(kcConfig.Host, "/")

	ctx := context.Background()
	providerUri := fmt.Sprintf("%s/realms/%s", host, realm)
	println(providerUri)
	provider, err := oidc.NewProvider(ctx, fmt.Sprintf("%s/realms/%s", host, realm))
	if err != nil {
		logrus.Errorf("E: %v", err)
		//panic(err)
	}
	verifier = provider.Verifier(&oidc.Config{ClientID: clientID, SkipClientIDCheck: true})
}

func readKeycloakConfig() (*keycloakConfig, error) {
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

	kcConfig := keycloakConfig{}
	err = json.Unmarshal(payload, &kcConfig)
	return &kcConfig, err
}

func Protect(handler JWTHandlerFunc) http.HandlerFunc {
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
			w.WriteHeader(http.StatusForbidden)
			return
		}

		claims := PlatformClaims{}
		if err := idToken.Claims(&claims); err != nil {
			logrus.WithField("error", "Claims").Errorf("%v", err)
			w.WriteHeader(http.StatusForbidden)
			return
		}

		tenant := r.Header.Get("tenant")
		if contains(claims.Tenants, tenant) == false {
			logrus.WithField("tenant", tenant).Warnf("Unauthorized access with tenant %s", tenant)
			w.WriteHeader(http.StatusForbidden)
			return
		}

		handler(w, r, &claims, strings.ToUpper(tenant))
	}
}

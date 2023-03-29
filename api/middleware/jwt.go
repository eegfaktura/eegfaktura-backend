package middleware

import (
	"context"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"strings"
)

type JwtValidateMiddleware struct {
	jwtUtil *AccessTokenGenJWT
}

const (
	BASIC_SCHEMA  string = "Basic "
	BEARER_SCHEMA string = "Bearer "
)

func NewJWTMiddleware(jwtUtil *AccessTokenGenJWT) *JwtValidateMiddleware {
	return &JwtValidateMiddleware{
		jwtUtil: jwtUtil,
	}
}

func JWTMiddleware(publicKeyPath string /*jwtUtil *AccessTokenGenJWT*/) func(JWTHandlerFunc) http.HandlerFunc {
	pub, err := loadEncryptionKey(publicKeyPath)
	if err != nil {
		log.Fatal(err)
	}
	jwtUtil := &AccessTokenGenJWT{PublicKey: pub.key}

	return func(handler JWTHandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
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
			handler(w, r, claims, strings.ToUpper(tenant))
		}
	}
}

func (m *JwtValidateMiddleware) WrapHandler(secured SecHandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		jwtToken := r.Header.Get("access_token")
		if len(jwtToken) == 0 {
			log.Printf("No Access_token in request!")
			w.WriteHeader(http.StatusForbidden)
			return
		}

		claims, err := m.jwtUtil.ExtractClaims(jwtToken)
		if err != nil {
			fmt.Printf("Error while parsing token: %s\n", err)
			w.WriteHeader(http.StatusForbidden)
			return
		}

		vars := mux.Vars(r)
		if vars == nil {
			vars = make(map[string]string)
		}

		ctx := context.WithValue(context.Background(), "upstream-user-agent", r.Header.Get("User-Agent"))
		if err := secured(ctx, w, r, vars, claims); err != nil {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(err.Error()))
		}

	}
}

// PublicKeyResult is a wrapper for the public key and the kid that identifies it
type PublicKeyResult struct {
	key *rsa.PublicKey
	kid string
}

func loadEncryptionKey(encryptionKeyPath string) (*PublicKeyResult, *KeyLoadError) {

	keyData, err := ioutil.ReadFile(encryptionKeyPath)
	if err != nil {
		return nil, &KeyLoadError{Operation: "read", Err: "Failed to read encryption key from file: " + encryptionKeyPath}
	}

	block, _ := pem.Decode(keyData)

	var pub interface{}
	if pub, err = x509.ParsePKIXPublicKey(block.Bytes); err != nil {
		if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
			pub = cert.PublicKey
		} else {
			return nil, &KeyLoadError{Operation: "parse", Err: "Failed to parse encryption key PEM"}
		}
	}

	kid := fmt.Sprintf("%x", sha1.Sum(keyData))

	publicKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, &KeyLoadError{Operation: "cast", Err: "Failed to cast key to rsa.PublicKey"}
	}

	return &PublicKeyResult{publicKey, kid}, nil
}

// TokenError describes an error that can occur during JWT generation
type TokenError struct {
	// Err is a description of the error that occurred.
	Desc string

	// From is optionally the original error from which this one was caused.
	From error
}

// KeyLoadError describes an error that can occur during key loading
type KeyLoadError struct {
	// Operation is the operation which caused the error, such as
	// "read", "parse" or "cast".
	Operation string

	// Err is a description of the error that occurred during the operation.
	Err string
}

func (e *KeyLoadError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Operation + ": " + e.Err
}

func (tokenErr *TokenError) Error() string {
	if tokenErr == nil {
		return "<nil>"
	}
	err := tokenErr.Desc
	if tokenErr.From != nil {
		err += " (" + tokenErr.From.Error() + ")"
	}
	return err
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if strings.ToUpper(v) == strings.ToUpper(str) {
			return true
		}
	}
	return false
}

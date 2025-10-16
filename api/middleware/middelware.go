package middleware

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"net/http"

	"github.com/golang-jwt/jwt"
)

type AccessGroups []string

func (ag AccessGroups) IsAdmin() bool {
	for _, s := range ag {
		if s == "/EEG_ADMIN" {
			return true
		}
	}
	return false
}

func (ag AccessGroups) IsUser() bool {
	for _, s := range ag {
		if s == "/EEG_USER" {
			return true
		}
	}
	return false
}

type PlatformClaims struct {
	Tenants      []string     `json:"tenant"`
	Username     string       `json:"preferred_username"`
	Email        string       `json:"email"`
	AccessGroups AccessGroups `json:"access_groups"`
	RealmAccess  struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
	jwt.StandardClaims
}

type SecHandlerFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string, claims *PlatformClaims) error

type Middleware interface {
	WrapHandler(handler SecHandlerFunc) http.HandlerFunc
}

// JWTHandlerFunc Protected HTTP Callback function containing JWT Claims and the tenant.
type JWTHandlerFunc func(http.ResponseWriter, *http.Request, *PlatformClaims, string)

type JWTConditionalFunc func(admin JWTHandlerFunc, user JWTHandlerFunc)

type JWTWrapperFunc func(handlerFunc JWTHandlerFunc) http.HandlerFunc

type AccessTokenGenJWT struct {
	PublicKey *rsa.PublicKey
}

func (c *AccessTokenGenJWT) ExtractClaims(accesstoken string) (*PlatformClaims, error) {
	token, err := jwt.ParseWithClaims(accesstoken, &PlatformClaims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, errors.New(fmt.Sprintf("parseWithClaims different algorithms used - %v", t.Method.Alg()))
		}
		return c.PublicKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("couldn't ParseWithClaims in parseToken %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token not valid in parseToken")
	}

	return token.Claims.(*PlatformClaims), nil

}

func IsSuperuser(roles []string) bool {
	return hasRole(roles, "superuser")
}

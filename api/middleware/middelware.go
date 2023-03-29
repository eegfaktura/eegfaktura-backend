package middleware

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt"
	"net/http"
)

type PlatformClaims struct {
	Tenants  []string `json:"tenant"`
	Username string   `json:"preferred_username"`
	jwt.StandardClaims
}

type SecHandlerFunc func(ctx context.Context, w http.ResponseWriter, r *http.Request, vars map[string]string, claims *PlatformClaims) error

type Middleware interface {
	WrapHandler(handler SecHandlerFunc) http.HandlerFunc
}

type JWTHandlerFunc func(http.ResponseWriter, *http.Request, *PlatformClaims, string)

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

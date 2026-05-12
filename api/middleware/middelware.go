package middleware

import (
	"net/http"
)

type BackendClaims struct {
	UserId       string       `json:"user_id"`
	Mail         string       `json:"mail"`
	Tenant       string       `json:"tenant"`
	AccessGroups AccessGroups `json:"access_groups"`
	IsAdmin      bool         `json:"is_admin"`
}

// JWTHandlerFunc Protected HTTP Callback function containing JWT Claims and the tenant.
type JWTHandlerFunc func(http.ResponseWriter, *http.Request, *PlatformClaims, string)
type TenantHandlerFunc func(http.ResponseWriter, *http.Request, *BackendClaims)

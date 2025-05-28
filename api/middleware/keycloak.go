package middleware

import (
	httputil2 "at.ourproject/vfeeg-backend/api/middleware/httputil"
	"context"
	"github.com/coreos/go-oidc/v3/oidc"
	"net/http"
)

type KeycloakClient struct {
	oidc         *oidc.Provider
	verifier     *oidc.IDTokenVerifier
	clientID     string
	clientSecret string
	client       *http.Client
}

func NewKeycloakClient(issuer, clientID, clientSecret string, client *http.Client) (*KeycloakClient, error) {
	kc := &KeycloakClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		client:       client,
	}
	var err error
	kc.oidc, err = oidc.NewProvider(oidc.ClientContext(context.Background(), client), issuer)
	if err != nil {
		return nil, err
	}
	kc.verifier = kc.oidc.Verifier(&oidc.Config{ClientID: clientID, SkipClientIDCheck: true})
	return kc, nil
}

type Credentials struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

func (kc *KeycloakClient) Authenticate() (*httputil2.ClientCreds, error) {
	resp, err := httputil2.PostFormUrlencoded(kc.client, kc.oidc.Endpoint().TokenURL, nil, map[string][]string{
		"grant_type":    {"client_credentials"},
		"client_id":     {kc.clientID},
		"client_secret": {kc.clientSecret},
	})
	if err != nil {
		return nil, err
	}
	creds := &httputil2.ClientCreds{}
	if err = httputil2.DecodeJSONResponse(resp, creds); err != nil {
		return nil, err
	}
	return creds, nil
}

func (kc *KeycloakClient) AuthenticateUserWithPassword(username, password string) (token *oidc.IDToken, err error) {
	params := map[string][]string{
		"grant_type":    {"password"},
		"client_id":     {kc.clientID},
		"client_secret": {kc.clientSecret},
		"scope":         {"openid"},
		"username":      {username},
		"password":      {password},
	}
	resp, err := httputil2.PostFormUrlencoded(kc.client, kc.oidc.Endpoint().TokenURL, nil, params)
	if err != nil {
		return
	}
	tok := &Credentials{}
	if err = httputil2.DecodeJSONResponse(resp, tok); err != nil {
		return
	}

	token, err = kc.VerifyToken(tok.IDToken)
	return
}

func (kc *KeycloakClient) VerifyToken(token string) (*oidc.IDToken, error) {
	return kc.verifier.Verify(context.Background(), token)
}

package httputil

import (
	"bytes"
	"net/http"
	"net/url"
	"time"
)

type ClientCreds struct {
	AccessToken      string `json:"access_token,omitempty"`
	ExpiresIn        int    `json:"expires_in,omitempty"`
	RefreshExpiresIn int    `json:"refresh_expires_in,omitempty"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	TokenType        string `json:"token_type,omitempty"`

	expiresTime time.Time
}

func (c *ClientCreds) setExpiresTime() {
	if c.expiresTime.IsZero() {
		c.expiresTime = time.Now().Add(time.Second * time.Duration(c.ExpiresIn))
	}
}

func (c *ClientCreds) expired() bool {
	return c.expiresTime.Before(time.Now())
}

func PostFormUrlencoded(client *http.Client, url string, modifyRequest func(r *http.Request), values url.Values) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader([]byte(values.Encode())))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if modifyRequest != nil {
		modifyRequest(req)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if err := Ensure2XX(resp); err != nil {
		return nil, err
	}
	return resp, nil
}

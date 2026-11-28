package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var admin = &middleware.PlatformClaims{Tenants: []string{"TE100100"}, AccessGroups: middleware.AccessGroups{"/EEG_ADMIN"}}

func gqlRequest(acceptEncoding string) *http.Request {
	req := authed(httptest.NewRequest("POST", "/query", strings.NewReader(`{"query":"query { __typename }"}`)))
	req.Header.Set("Content-Type", "application/json")
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	return req
}

// A JSON answer is gzipped for a client that accepts it and plain for one that does not; the
// decoded body is the plain one.
func TestCompressJSON_Answer(t *testing.T) {
	h := served(t, admin)
	plain := httptest.NewRecorder()
	h.ServeHTTP(plain, gqlRequest(""))
	require.Equal(t, 200, plain.Code, plain.Body.String())
	assert.Empty(t, plain.Header().Get("Content-Encoding"))
	assert.Contains(t, plain.Header().Values("Vary"), "Accept-Encoding")
	assert.JSONEq(t, `{"data":{"__typename":"Query"}}`, plain.Body.String())

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, gqlRequest("gzip, deflate, br"))
	require.Equal(t, 200, rec.Code)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	assert.Contains(t, rec.Header().Values("Vary"), "Accept-Encoding")
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	zr, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	body, err := io.ReadAll(zr)
	require.NoError(t, err)
	assert.JSONEq(t, plain.Body.String(), string(body))
}

// The file downloads stay uncompressed even when the client accepts gzip. A request the router
// does not match would be compressed, so this also shows the templates are the real routes.
func TestCompressJSON_SkipsDownloads(t *testing.T) {
	h := served(t, admin)
	for _, path := range []string{"/eeg/export/masterdata", "/eeg/mailtemplates/invoice/files/4711"} {
		req := authed(httptest.NewRequest("GET", path, nil))
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		assert.Empty(t, rec.Header().Get("Content-Encoding"), path)
	}
	req := authed(httptest.NewRequest("GET", "/eeg/export/masterdata/other", nil))
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"), "control: unmatched is compressed")
}

// A panicking handler still gets the access log's 500 when the client accepts gzip: the compression
// sits outside logging.AccessLog in httpHandler.
func TestCompressJSON_PanicIs500(t *testing.T) {
	r := mux.NewRouter()
	r.HandleFunc("/boom", func(http.ResponseWriter, *http.Request) { panic("boom") })
	req := httptest.NewRequest("GET", "/boom", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	httpHandler(r).ServeHTTP(rec, req)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

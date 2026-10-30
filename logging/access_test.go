package logging

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// accessFields is everything an access line may hold. A new field is a
// decision (no user tracking, 2026-10-29), not a side effect.
var accessFields = []string{
	"@timestamp", "log.level", "message", "service.name", "service.version",
	"event.action", "event.outcome", "event.duration",
	"http.request.method", "http.route", "http.response.status_code",
	"request.id", "trace.id", "span.id", "tenant.id", "client.id", "graphql.operation.name",
}

func router(h http.HandlerFunc) *mux.Router {
	r := mux.NewRouter()
	s := r.PathPrefix("/").Subrouter()
	s.HandleFunc("/participant/{id}", h).Methods("GET")
	s.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {}).Methods("GET")
	s.Use(AccessLog)
	return s
}

func serve(r http.Handler, req *http.Request) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestAccessLog_Fields(t *testing.T) {
	buf := setup(t, Config{Level: slog.LevelInfo, JSON: true, Version: "1.2.3"})
	r := router(func(w http.ResponseWriter, r *http.Request) {
		SetClient(r.Context(), "at.ourproject.vfeeg.web")
		SetTenant(r.Context(), "TE100100")
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest("GET", "/participant/5c0a2d6e-1111-2222-3333-444455556666", nil)
	req.Header.Set("X-Request-Id", "abc-123")
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	w := serve(r, req)

	assert.Equal(t, "abc-123", w.Header().Get("X-Request-Id"))
	ls := lines(t, buf)
	require.Len(t, ls, 1)
	l := ls[0]
	assert.Equal(t, "http.access", l["event.action"])
	assert.Equal(t, "success", l["event.outcome"])
	assert.Equal(t, "info", l["log.level"])
	assert.Equal(t, "GET", l["http.request.method"])
	assert.Equal(t, "/participant/{id}", l["http.route"])
	assert.EqualValues(t, 201, l["http.response.status_code"])
	assert.Contains(t, l, "event.duration")
	assert.Equal(t, "abc-123", l["request.id"])
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", l["trace.id"])
	assert.Equal(t, "00f067aa0ba902b7", l["span.id"])
	assert.Equal(t, "TE100100", l["tenant.id"])
	assert.Equal(t, "at.ourproject.vfeeg.web", l["client.id"])
	assert.Equal(t, "GET /participant/{id} 201", l["message"])
}

func TestAccessLog_KeepsValidRequestID(t *testing.T) {
	buf := setup(t, Config{Level: slog.LevelInfo, JSON: true})
	id := "urn:x." + strings.Repeat("a", 122)
	req := httptest.NewRequest("GET", "/participant/1", nil)
	req.Header.Set("X-Request-Id", id)
	serve(router(func(w http.ResponseWriter, r *http.Request) {}), req)
	assert.Equal(t, id, lines(t, buf)[0]["request.id"])
}

func TestParseTraceparent(t *testing.T) {
	valid := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	tr, sp := parseTraceparent(valid)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", tr)
	assert.Equal(t, "00f067aa0ba902b7", sp)
	for _, in := range []string{
		"", "ff-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		"00-00000000000000000000000000000000-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-0000000000000000-01",
		"00-4BF92F3577B34DA6A3CE929D0E0E4736-00f067aa0ba902b7-01",
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",
	} {
		tr, sp := parseTraceparent(in)
		assert.Empty(t, tr+sp, in)
	}
}

func TestAccessLog_Outcome(t *testing.T) {
	for status, want := range map[int]string{200: "success", 302: "success", 400: "failure", 401: "refused", 403: "refused", 404: "failure", 500: "failure"} {
		buf := setup(t, Config{Level: slog.LevelInfo, JSON: true})
		serve(router(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status) }), httptest.NewRequest("GET", "/participant/1", nil))
		assert.Equal(t, want, lines(t, buf)[0]["event.outcome"], status)
	}
}

func TestAccessLog_RequestIDGeneratedWhenMissingOrInvalid(t *testing.T) {
	r := router(func(w http.ResponseWriter, r *http.Request) {})
	for _, in := range []string{"", "has space", strings.Repeat("a", 129), "a\"b"} {
		buf := setup(t, Config{Level: slog.LevelInfo, JSON: true})
		req := httptest.NewRequest("GET", "/participant/1", nil)
		req.Header.Set("X-Request-Id", in)
		req.Header.Set("traceparent", "00-00000000000000000000000000000000-00f067aa0ba902b7-01")
		w := serve(r, req)

		l := lines(t, buf)[0]
		assert.Regexp(t, `^[0-9a-f]{12}4[0-9a-f]{3}[89ab][0-9a-f]{15}$`, l["request.id"], in, "uuid4 as 32 hex")
		assert.Equal(t, l["request.id"], w.Header().Get("X-Request-Id"))
		assert.NotContains(t, l, "trace.id", "an all-zero trace id is invalid")
	}
}

func TestAccessLog_RefusalWithoutTenant(t *testing.T) {
	buf := setup(t, Config{Level: slog.LevelInfo, JSON: true})
	r := router(func(w http.ResponseWriter, r *http.Request) {
		SetClient(r.Context(), "at.ourproject.vfeeg.web")
		w.WriteHeader(http.StatusForbidden)
	})
	serve(r, httptest.NewRequest("GET", "/participant/1", nil))

	l := lines(t, buf)[0]
	assert.Equal(t, "refused", l["event.outcome"])
	assert.Equal(t, "warn", l["log.level"])
	assert.NotContains(t, l, "tenant.id")
	assert.Equal(t, "at.ourproject.vfeeg.web", l["client.id"])
}

func TestAccessLog_ProbesAreSkipped(t *testing.T) {
	buf := setup(t, Config{Level: slog.LevelInfo, JSON: true})
	serve(router(nil), httptest.NewRequest("GET", "/healthz", nil))
	assert.Empty(t, buf.String())
}

// Never log (concept point 3/6): no token, user agent, IP, raw path ids or
// query values in the access line, and no field outside accessFields.
func TestAccessLog_NeverLogsRequestData(t *testing.T) {
	buf := setup(t, Config{Level: slog.LevelInfo, JSON: true})
	r := router(func(w http.ResponseWriter, r *http.Request) {
		SetTenant(r.Context(), "TE100100")
	})
	secrets := []string{
		"eyJhbGciOiJSUzI1NiJ9.secret-token", "Mozilla/5.0 secret-agent", "203.0.113.7", "198.51.100.9",
		"AT0030000000000000000000000012345", "AT611904300234573201", "max.mustermann@example.org",
	}
	req := httptest.NewRequest("GET", "/participant/AT0030000000000000000000000012345?iban=AT611904300234573201&mail=max.mustermann@example.org", nil)
	req.RemoteAddr = "203.0.113.7:4711"
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJSUzI1NiJ9.secret-token")
	req.Header.Set("User-Agent", "Mozilla/5.0 secret-agent")
	req.Header.Set("X-Forwarded-For", "198.51.100.9")
	serve(r, req)

	out := buf.String()
	for _, s := range secrets {
		assert.NotContains(t, out, s)
	}
	l := lines(t, buf)[0]
	var extra []string
	for k := range l {
		if !slices.Contains(accessFields, k) {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	assert.Empty(t, extra, "fields outside the access-line allowlist")
}

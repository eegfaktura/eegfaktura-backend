package logging

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// The access line (concept point 6): one line per HTTP request, written when
// the request is done, so it comes after the token check in the handler
// wrappers (api/middleware). It holds the route template, never the raw path
// or query, and the tenant the backend used — no user id, e-mail, name, user
// agent or IP (no user tracking, 2026-10-29).

const (
	headerRequestID   = "X-Request-Id"
	headerTraceparent = "traceparent"
)

// skipRoutes are the Kubernetes probes; one line per probe is noise.
var skipRoutes = map[string]bool{"/healthz": true, "/readyz": true}

var (
	requestIDPattern   = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
	traceparentPattern = regexp.MustCompile(`^([0-9a-f]{2})-([0-9a-f]{32})-([0-9a-f]{16})-[0-9a-f]{2}$`)
)

type accessCtxKey struct{}

// accessInfo travels in the request context. The token check fills tenant
// and client; GraphQL fills the operation name.
type accessInfo struct {
	requestID, traceID, spanID  string
	tenant, clientID, operation string
}

func fromContext(ctx context.Context) *accessInfo {
	if ctx == nil {
		return nil
	}
	ai, _ := ctx.Value(accessCtxKey{}).(*accessInfo)
	return ai
}

func (ai *accessInfo) correlation() []slog.Attr {
	as := []slog.Attr{slog.String("request.id", ai.requestID)}
	if ai.traceID != "" {
		as = append(as, slog.String("trace.id", ai.traceID), slog.String("span.id", ai.spanID))
	}
	return as
}

// SetClient records the token's azp (the Keycloak client, not a person) once
// the token is verified.
func SetClient(ctx context.Context, clientID string) {
	if ai := fromContext(ctx); ai != nil {
		ai.clientID = clientID
	}
}

// SetTenant records the tenant the request was allowed for.
func SetTenant(ctx context.Context, tenant string) {
	if ai := fromContext(ctx); ai != nil {
		ai.tenant = tenant
	}
}

// SetOperation records the GraphQL operation name; /query is one route.
func SetOperation(ctx context.Context, name string) {
	if ai := fromContext(ctx); ai != nil {
		ai.operation = name
	}
}

// AccessLog is a gorilla/mux middleware (Router.Use).
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := routeTemplate(r)
		if skipRoutes[route] {
			next.ServeHTTP(w, r)
			return
		}

		ai := &accessInfo{requestID: r.Header.Get(headerRequestID)}
		if !requestIDPattern.MatchString(ai.requestID) {
			ai.requestID = newRequestID()
		}
		ai.traceID, ai.spanID = parseTraceparent(r.Header.Get(headerTraceparent))
		w.Header().Set(headerRequestID, ai.requestID)

		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), accessCtxKey{}, ai)))
		duration := time.Since(start)

		status := sw.status
		if status == 0 {
			status = http.StatusOK
		}
		outcome, level := "success", slog.LevelInfo
		switch {
		case status == http.StatusUnauthorized || status == http.StatusForbidden:
			outcome, level = "refused", slog.LevelWarn
		case status >= 400:
			outcome = "failure"
		}

		attrs := []slog.Attr{
			slog.String("event.action", "http.access"),
			slog.String("event.outcome", outcome),
			slog.String("http.request.method", r.Method),
			slog.Int("http.response.status_code", status),
			slog.Int64("event.duration", duration.Nanoseconds()),
		}
		msgRoute := "-"
		if route != "" {
			attrs = append(attrs, slog.String("http.route", route))
			msgRoute = route
		}
		if ai.tenant != "" {
			attrs = append(attrs, slog.String("tenant.id", ai.tenant))
		}
		if ai.clientID != "" {
			attrs = append(attrs, slog.String("client.id", ai.clientID))
		}
		if ai.operation != "" {
			attrs = append(attrs, slog.String("graphql.operation.name", ai.operation))
		}

		// the request context carries request.id, trace.id and span.id (contextHandler)
		ctx := context.WithValue(context.Background(), accessCtxKey{}, ai)
		slog.Default().LogAttrs(ctx, level, fmt.Sprintf("%s %s %d", r.Method, msgRoute, status), attrs...)
	})
}

func routeTemplate(r *http.Request) string {
	if cr := mux.CurrentRoute(r); cr != nil {
		if t, err := cr.GetPathTemplate(); err == nil {
			return t
		}
	}
	return ""
}

// newRequestID is a random (version 4) UUID as 32 hex characters.
func newRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b) // crypto/rand.Read never fails (Go 1.24+)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return hex.EncodeToString(b)
}

// parseTraceparent returns trace id and parent id of a valid W3C traceparent:
// not version ff, no all-zero ids. Otherwise both are empty.
func parseTraceparent(h string) (traceID, spanID string) {
	m := traceparentPattern.FindStringSubmatch(h)
	if m == nil || m[1] == "ff" || m[2] == strings.Repeat("0", 32) || m[3] == strings.Repeat("0", 16) {
		return "", ""
	}
	return m[2], m[3]
}

// statusWriter records the status code. It keeps Flush and Hijack (the
// GraphQL websocket transport needs Hijack) and Unwrap for
// http.ResponseController.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijack not supported")
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

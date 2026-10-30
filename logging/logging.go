// Package logging is the one place where the backend's log output is set up
// (logging concept W-261008-ca2, point 0): format, levels, the common ECS
// fields and the access line. Every log line goes through one slog handler;
// the existing logrus calls are routed into it by a hook (logrus.go).
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/sirupsen/logrus"
)

const ServiceName = "eegfaktura-backend"

// LevelTrace is below slog's debug; slog has no trace level of its own.
const LevelTrace = slog.Level(-8)

// Config is read from LOG_LEVEL, LOG_FORMAT and SERVICE_VERSION.
type Config struct {
	Level   slog.Level
	JSON    bool
	Version string
}

// ConfigFromEnv reads LOG_LEVEL (default info), LOG_FORMAT (default json) and
// SERVICE_VERSION (set by the image build, default dev). An unknown level or
// format is an error: the backend refuses to start.
func ConfigFromEnv(lookup func(string) (string, bool)) (Config, error) {
	cfg := Config{Level: slog.LevelInfo, JSON: true, Version: "dev"}
	if v, ok := lookup("SERVICE_VERSION"); ok && v != "" {
		cfg.Version = v
	}
	if v, ok := lookup("LOG_LEVEL"); ok {
		l, err := ParseLevel(v)
		if err != nil {
			return cfg, err
		}
		cfg.Level = l
	}
	if v, ok := lookup("LOG_FORMAT"); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "json":
			cfg.JSON = true
		case "text":
			cfg.JSON = false
		default:
			return cfg, fmt.Errorf("unknown LOG_FORMAT %q (json, text)", v)
		}
	}
	return cfg, nil
}

// ParseLevel accepts trace, debug, info, warn (or warning) and error.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return LevelTrace, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return slog.LevelInfo, fmt.Errorf("unknown LOG_LEVEL %q (trace, debug, info, warn, error)", s)
}

func levelName(l slog.Level) string {
	switch {
	case l < slog.LevelDebug:
		return "trace"
	case l < slog.LevelInfo:
		return "debug"
	case l < slog.LevelWarn:
		return "info"
	case l < slog.LevelError:
		return "warn"
	}
	return "error"
}

// NewHandler builds the one handler: one JSON object (or one key=value line)
// per record, ECS field names, service.name and service.version on every line.
func NewHandler(w io.Writer, cfg Config) slog.Handler {
	opts := &slog.HandlerOptions{Level: cfg.Level, ReplaceAttr: ecsKeys}
	var h slog.Handler
	if cfg.JSON {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	h = h.WithAttrs([]slog.Attr{
		slog.String("service.name", ServiceName),
		slog.String("service.version", cfg.Version),
	})
	return contextHandler{h}
}

func ecsKeys(groups []string, a slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return a
	}
	switch a.Key {
	case slog.TimeKey:
		return slog.String("@timestamp", a.Value.Time().UTC().Format("2006-01-02T15:04:05.000Z"))
	case slog.LevelKey:
		return slog.String("log.level", levelName(a.Value.Any().(slog.Level)))
	case slog.MessageKey:
		return slog.Attr{Key: "message", Value: a.Value}
	}
	return a
}

// allowedFields are the only record fields written (never-log rule: a field
// is a decision, not a side effect); others are dropped. ECS fields first,
// then the field names the existing logrus calls use.
var allowedFields = map[string]bool{
	"event.action": true, "event.outcome": true, "event.duration": true,
	"http.request.method": true, "http.route": true, "http.response.status_code": true,
	"request.id": true, "trace.id": true, "span.id": true,
	"tenant.id": true, "client.id": true, "graphql.operation.name": true,
	"error.type": true, "error.message": true, "error.stack_trace": true,
	"log.logger": true, "log.origin.function": true, "log.origin.file.name": true, "log.origin.file.line": true,
	// logrus fields in use (2026-10-30)
	"SQL": true, "STMT": true, "MAIL": true, "MSG": true, "PROTOCOL": true, "protocol": true,
	"code": true, "command": true, "ecId": true, "participantId": true, "templates": true,
	"sepaActiv": true, "settlementInterval": true,
	// db pool stats (database.go)
	"open": true, "inUse": true, "idle": true, "maxOpen": true, "waitCount": true, "waitDuration": true,
	"maxIdleClosed": true, "maxLifetimeClosed": true,
	// gql-admin-check, temporary F1 log (api/middleware/gqlAdminCheck.go); no "sub"
	// (user id) — no user tracking, 2026-10-29
	"access_groups.class": true, "superuser": true, "operation_type": true,
}

// contextHandler drops fields outside allowedFields and adds request.id,
// trace.id and span.id when the record's context belongs to an HTTP request
// (see AccessLog).
type contextHandler struct{ slog.Handler }

func (h contextHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		if allowedFields[a.Key] {
			out.AddAttrs(a)
		}
		return true
	})
	if ai := fromContext(ctx); ai != nil {
		out.AddAttrs(ai.correlation()...)
	}
	return h.Handler.Handle(ctx, out)
}

func (h contextHandler) WithAttrs(as []slog.Attr) slog.Handler {
	var kept []slog.Attr
	for _, a := range as {
		if allowedFields[a.Key] {
			kept = append(kept, a)
		}
	}
	return contextHandler{h.Handler.WithAttrs(kept)}
}

func (h contextHandler) WithGroup(name string) slog.Handler {
	return contextHandler{h.Handler.WithGroup(name)}
}

// Setup makes the handler the default for slog and the standard log package,
// and routes the global logrus logger into it.
func Setup(w io.Writer, cfg Config) {
	h := NewHandler(w, cfg)
	slog.SetDefault(slog.New(h))

	logrus.SetLevel(logrusLevel(cfg.Level))
	logrus.SetOutput(io.Discard)
	logrus.SetFormatter(discardFormatter{})
	logrus.StandardLogger().ReplaceHooks(logrus.LevelHooks{})
	logrus.AddHook(&bridge{handler: h})
}

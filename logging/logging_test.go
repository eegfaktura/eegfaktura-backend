package logging

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

// setup routes slog and logrus into a buffer for one test and restores them after.
func setup(t *testing.T, cfg Config) *bytes.Buffer {
	t.Helper()
	oldSlog := slog.Default()
	std := logrus.StandardLogger()
	oldLevel, oldOut, oldFmt, oldCaller := std.Level, std.Out, std.Formatter, std.ReportCaller
	oldHooks := std.ReplaceHooks(logrus.LevelHooks{})
	t.Cleanup(func() {
		slog.SetDefault(oldSlog)
		std.SetLevel(oldLevel)
		std.SetOutput(oldOut)
		std.SetFormatter(oldFmt)
		std.SetReportCaller(oldCaller)
		std.ReplaceHooks(oldHooks)
	})
	var buf bytes.Buffer
	Setup(&buf, cfg)
	return &buf
}

func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(l), &m), l)
		out = append(out, m)
	}
	return out
}

func TestConfigFromEnv_Defaults(t *testing.T) {
	cfg, err := ConfigFromEnv(env(nil))
	require.NoError(t, err)
	assert.Equal(t, slog.LevelInfo, cfg.Level)
	assert.True(t, cfg.JSON)
	assert.Equal(t, "dev", cfg.Version)

	cfg, err = ConfigFromEnv(env(map[string]string{"SERVICE_VERSION": "1.2.3"}))
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", cfg.Version)
}

func TestConfigFromEnv_Levels(t *testing.T) {
	for in, want := range map[string]slog.Level{
		"trace": LevelTrace, "debug": slog.LevelDebug, "info": slog.LevelInfo, "INFO": slog.LevelInfo,
		"warn": slog.LevelWarn, "warning": slog.LevelWarn, "error": slog.LevelError,
	} {
		cfg, err := ConfigFromEnv(env(map[string]string{"LOG_LEVEL": in}))
		require.NoError(t, err, in)
		assert.Equal(t, want, cfg.Level, in)
	}
}

// An unknown value stops the start instead of falling back silently.
func TestConfigFromEnv_UnknownValuesAreRefused(t *testing.T) {
	for _, m := range []map[string]string{
		{"LOG_LEVEL": "verbose"},
		{"LOG_LEVEL": ""},
		{"LOG_FORMAT": "xml"},
	} {
		_, err := ConfigFromEnv(env(m))
		assert.Error(t, err, m)
	}
	cfg, err := ConfigFromEnv(env(map[string]string{"LOG_FORMAT": "text"}))
	require.NoError(t, err)
	assert.False(t, cfg.JSON)
}

func TestJSON_CommonFieldsFromLogrus(t *testing.T) {
	buf := setup(t, Config{Level: slog.LevelInfo, JSON: true, Version: "1.2.3"})
	logrus.SetReportCaller(true)

	logrus.WithField("tenant", "TE100100").WithError(errors.New("boom")).Error("update failed\n")

	ls := lines(t, buf)
	require.Len(t, ls, 1)
	l := ls[0]
	assert.Regexp(t, regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`), l["@timestamp"])
	assert.Equal(t, "error", l["log.level"])
	assert.Equal(t, "eegfaktura-backend", l["service.name"])
	assert.Equal(t, "1.2.3", l["service.version"])
	assert.Equal(t, "update failed", l["message"])
	assert.Equal(t, "TE100100", l["tenant.id"])
	assert.Equal(t, "*errors.errorString", l["error.type"])
	assert.Equal(t, "boom", l["error.message"])
	assert.Equal(t, "logging_test.go", l["log.origin.file.name"])
	assert.NotContains(t, l, "tenant")
	assert.NotContains(t, l, "msg")
	assert.NotContains(t, l, "time")
}

func TestLevels_FilterAndNames(t *testing.T) {
	buf := setup(t, Config{Level: slog.LevelWarn, JSON: true})
	logrus.Info("hidden")
	logrus.Debug("hidden")
	slog.Info("hidden")
	logrus.Warn("shown")
	logrus.Error("shown")
	ls := lines(t, buf)
	require.Len(t, ls, 2)
	assert.Equal(t, "warn", ls[0]["log.level"])
	assert.Equal(t, "error", ls[1]["log.level"])

	buf = setup(t, Config{Level: LevelTrace, JSON: true})
	logrus.Trace("t")
	slog.Log(t.Context(), LevelTrace, "t")
	slog.Debug("d")
	ls = lines(t, buf)
	require.Len(t, ls, 3)
	assert.Equal(t, "trace", ls[0]["log.level"])
	assert.Equal(t, "trace", ls[1]["log.level"])
	assert.Equal(t, "debug", ls[2]["log.level"])
}

func TestText_SameFields(t *testing.T) {
	buf := setup(t, Config{Level: slog.LevelInfo, JSON: false, Version: "1.2.3"})
	logrus.WithField("tenant", "TE100100").Info("hello")
	out := buf.String()
	for _, want := range []string{"@timestamp=", "log.level=info", "message=hello", "service.name=eegfaktura-backend", "tenant.id=TE100100"} {
		assert.Contains(t, out, want)
	}
	assert.Equal(t, 1, strings.Count(out, "\n"))
}

// Fields outside the allowlist are dropped, from logrus and from slog.
func TestUnknownFieldsAreDropped(t *testing.T) {
	buf := setup(t, Config{Level: slog.LevelInfo, JSON: true})
	logrus.WithField("email", "max.mustermann@example.org").WithField("sub", "f81d4fae-7dec-11d0-a765-00a0c91e6bf6").
		WithField("ecId", "AT00300000000RC100181000000000001").Info("a")
	slog.Info("b", "iban", "AT611904300234573201", "tenant.id", "TE100100")
	ls := lines(t, buf)
	require.Len(t, ls, 2)
	assert.NotContains(t, buf.String(), "example.org")
	assert.NotContains(t, buf.String(), "f81d4fae", "sub (user id) never appears")
	assert.NotContains(t, allowedFields, "sub")
	assert.NotContains(t, buf.String(), "AT611904300234573201")
	assert.Equal(t, "AT00300000000RC100181000000000001", ls[0]["ecId"])
	assert.Equal(t, "TE100100", ls[1]["tenant.id"])
}

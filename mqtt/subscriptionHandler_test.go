package mqttclient

import (
	"io"
	"log/slog"
	"testing"

	"at.ourproject/vfeeg-backend/logging"
	"at.ourproject/vfeeg-backend/logging/logtest"
	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An ERROR message of eda-xp is one ERROR line with the reason, event.action, tenant and conversation
// (eegfaktura-dev#56 #4); before, the reason was dropped.
func TestErrorHandler_logsTheReason(t *testing.T) {
	buf := logtest.Capture(t, func(w io.Writer) { logging.Setup(w, logging.Config{Level: slog.LevelInfo, JSON: true}) })

	errorHandler(model.SubscribeMessage{MessageCode: "ERROR_MESSAGE", Protocol: model.ERROR, Tenant: "rc100100",
		Payload: model.EbmsMessage{ConversationId: "RC100100202611060000000000000001", ErrorMessage: "Tenant not registered"}})

	lines := logtest.Lines(t, buf)
	require.Len(t, lines, 1)
	l := lines[0]
	assert.Equal(t, "error", l["log.level"])
	assert.Equal(t, "eda.error.received", l["event.action"])
	assert.Equal(t, "Tenant not registered", l["error.message"])
	assert.Equal(t, "ERROR_MESSAGE", l["code"])
	assert.Equal(t, "RC100100", l["tenant.id"])
	assert.Contains(t, l["message"], "RC100100202611060000000000000001")
}

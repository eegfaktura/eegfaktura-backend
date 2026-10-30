package mqttclient

import (
	"encoding/json"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/model"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
)

// The revoke carries the reason in the existing MQTT field "reason" (eda-xp writes it to CMRevoke); without a reason
// the field is absent.
func TestRevokeMeteringPointReason(t *testing.T) {
	// a broker of its own (mock client) instead of the shared one of the other tests; Broker() is called first so
	// that its once.Do cannot replace this instance later
	shared := Broker()
	b, err := NewMessageBroker()
	require.NoError(t, err)
	_, err = b.Init(newMockClient)
	require.NoError(t, err)
	instance = b
	t.Cleanup(func() { instance = shared })

	payloads := make(chan []byte, 2)
	b.cl.Subscribe("eda/request", 1, func(c mqtt.Client, m mqtt.Message) { payloads <- m.Payload() })
	next := func() map[string]interface{} {
		t.Helper()
		select {
		case p := <-payloads:
			msg := map[string]interface{}{}
			require.NoError(t, json.Unmarshal(p, &msg))
			return msg
		case <-time.After(5 * time.Second):
			t.Fatal("no message on eda/request")
			return nil
		}
	}

	eeg := &model.Eeg{Id: "TE000001", Area: "LOCAL", CommunityId: "TE00000001212000012121",
		GridOperator: "TT009999", RcNumber: "RC100130"}
	meter := &model.MeteringPoint{MeteringPoint: "AT00000000000000000000000000122121", Direction: "CONSUMPTION",
		ConsentId: null.StringFrom("C-1")}

	reason := "Auszug des Mitglieds"
	require.NoError(t, RevokeMeteringPoint(eeg, meter, 1700000000000, &reason))
	msg := next()
	assert.Equal(t, "Auszug des Mitglieds", msg["reason"])
	assert.Equal(t, string(model.EBMS_AUFHEBUNG_CCMS), msg["messageCode"])

	require.NoError(t, RevokeMeteringPoint(eeg, meter, 1700000000000, nil))
	assert.NotContains(t, next(), "reason")
}

package mqttclient

import (
	"encoding/json"
	"sync"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
)

// The revoke carries the reason in the existing MQTT field "reason" (eda-xp writes it to CMRevoke); without a reason
// the field is absent.
func TestRevokeMeteringPointReason(t *testing.T) {
	broker, _ := Broker().Init(newMockClient)

	var payload map[string]interface{}
	var wg sync.WaitGroup
	broker.(*MessageBroker).cl.Subscribe("eda/request", 1, func(c mqtt.Client, m mqtt.Message) {
		payload = map[string]interface{}{}
		require.NoError(t, json.Unmarshal(m.Payload(), &payload))
		wg.Done()
	})

	eeg := &model.Eeg{Id: "TE000001", Area: "LOCAL", CommunityId: "TE00000001212000012121",
		GridOperator: "TT009999", RcNumber: "RC100130"}
	meter := &model.MeteringPoint{MeteringPoint: "AT00000000000000000000000000122121", Direction: "CONSUMPTION",
		ConsentId: null.StringFrom("C-1")}

	reason := "Auszug des Mitglieds"
	wg.Add(1)
	require.NoError(t, RevokeMeteringPoint(eeg, meter, 1700000000000, &reason))
	wg.Wait()
	assert.Equal(t, "Auszug des Mitglieds", payload["reason"])
	assert.Equal(t, string(model.EBMS_AUFHEBUNG_CCMS), payload["messageCode"])

	wg.Add(1)
	require.NoError(t, RevokeMeteringPoint(eeg, meter, 1700000000000, nil))
	wg.Wait()
	assert.NotContains(t, payload, "reason")
}

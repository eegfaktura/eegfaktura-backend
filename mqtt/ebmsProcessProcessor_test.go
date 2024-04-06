package mqttclient

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
	"sync"
	"testing"
	"time"
)

func TestRegistrationForParticipation(t *testing.T) {
	_, err := NewMessageBrokerMock()
	require.NoError(t, err)

	messageBroker.Start()

	token := messageBroker.client.Connect()
	token.Wait()
	require.NoError(t, token.Error())

	var wg sync.WaitGroup
	wg.Add(1)
	messageBroker.client.Subscribe("eda/request", 1, func(c mqtt.Client, m mqtt.Message) {
		var msg model.EbmsMessage
		require.NoError(t, json.Unmarshal(m.Payload(), &msg))
		fmt.Printf("M: %+v\n", msg)

		assert.Equal(t, "RC100130", msg.Sender)
		assert.Equal(t, "TT009999", msg.Receiver)
		assert.Equal(t, "TE00000001212000012121", msg.EcId)

		assert.Equal(t, "AT00000000000000000000000000122121", msg.Meter.MeteringPoint)
		assert.Equal(t, model.CONSUMPTION, msg.Meter.Direction)
		assert.Equal(t, 0, msg.Meter.PartFact)
		wg.Done()
	})

	eeg := &model.Eeg{
		Id:                 "TE000001",
		Name:               "Test-EEG",
		Description:        "",
		BusinessNr:         null.String{},
		Area:               "LOCAL",
		Legal:              "verein",
		OperatorName:       "Test-Netz",
		CommunityId:        "TE00000001212000012121",
		GridOperator:       "TT009999",
		RcNumber:           "RC100130",
		AllocationMode:     "DYNAMIC",
		SettlementInterval: "MONTHLY",
		ProviderBusinessNr: null.Int{},
		TaxNumber:          null.String{},
		VatNumber:          null.String{},
		ContactPerson:      null.String{},
		EegAddress:         model.EegAddress{},
		AccountInfo:        model.AccountInfo{},
		Contact:            model.Contact{},
		Optionals:          model.Optionals{},
		Online:             false,
	}

	meter := &model.MeteringPoint{
		MeteringPoint:    "AT00000000000000000000000000122121",
		Transformer:      null.String{},
		Direction:        "CONSUMPTION",
		Status:           "NEW",
		StatusCode:       null.Int{},
		TariffId:         null.String{},
		EquipmentNumber:  null.String{},
		EquipmentName:    null.String{},
		InverterId:       null.String{},
		Street:           null.String{},
		StreetNumber:     null.String{},
		City:             null.String{},
		Zip:              null.String{},
		RegisteredSince:  time.Time{},
		ModifiedAt:       time.Time{},
		ModifiedBy:       null.String{},
		GridOperatorId:   null.String{},
		GridOperatorName: null.String{},
		State:            nil,
		PartFact:         0,
	}

	require.NoError(t, RegistrationForParticipation(eeg, meter))
	wg.Wait()
}

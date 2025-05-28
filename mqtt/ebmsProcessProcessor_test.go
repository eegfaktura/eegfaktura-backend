package mqttclient

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
	"reflect"
	"sync"
	"testing"
)

func TestRegistrationForParticipation(t *testing.T) {
	//_, err := NewMessageBrokerMock()
	//require.NoError(t, err)

	//messageBroker.Start()
	//
	//token := messageBroker.client.Connect()
	//if token != nil {
	//	token.Wait()
	//	require.NoError(t, token.Error())
	//}
	//
	//var currentMsg mqtt.Message
	//var wg sync.WaitGroup
	//messageBroker.client.Subscribe("eda/request", 1, func(c mqtt.Client, m mqtt.Message) {
	//	currentMsg = m
	//	wg.Done()
	//})

	broker, _ := Broker().Init(newMockClient)

	var currentMsg mqtt.Message
	var wg sync.WaitGroup
	broker.(*MessageBroker).cl.Subscribe("eda/request", 1, func(c mqtt.Client, m mqtt.Message) {
		currentMsg = m
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
		RegisteredSince:  civil.Today(),
		ModifiedAt:       civil.Now(),
		ModifiedBy:       null.String{},
		GridOperatorId:   null.String{},
		GridOperatorName: null.String{},
		State:            nil,
		PartFact:         10,
	}

	copyMeter := func(attr string, value interface{}) *model.MeteringPoint {
		d := *meter
		reflect.ValueOf(&d).Elem().FieldByName(attr).Set(reflect.ValueOf(value))
		return &d
	}

	type args struct {
		eeg   *model.Eeg
		meter *model.MeteringPoint
		from  *int64
	}

	from := civil.Today().Unix() * 1000
	tests := []struct {
		name     string
		args     args
		validate func(t *testing.T, msg mqtt.Message)
	}{
		{
			name: "Registration with from attribute",
			args: args{
				eeg:   eeg,
				meter: meter,
				from:  &from,
			},
			validate: func(t *testing.T, m mqtt.Message) {
				var msg model.EbmsMessage
				fmt.Printf("Received message: %s\n", string(m.Payload()))
				require.NoError(t, json.Unmarshal(m.Payload(), &msg))
				fmt.Printf("M: %+v\n", msg)
				fmt.Printf("MM: %+v\n", msg.Meter)

				assert.Equal(t, "RC100130", msg.Sender)
				assert.Equal(t, "TT009999", msg.Receiver)
				assert.Equal(t, "TE00000001212000012121", msg.EcId)

				assert.Equal(t, "AT00000000000000000000000000122121", msg.Meter.MeteringPoint)
				assert.Equal(t, model.CONSUMPTION, msg.Meter.Direction)
				assert.Equal(t, 10, msg.Meter.PartFact)
				assert.Equal(t, civil.Today().Unix()*1000, msg.Meter.From)
			},
		},
		{
			name: "Registration without 'from' attribute",
			args: args{
				eeg:   eeg,
				meter: copyMeter("PartFact", 100),
				from:  nil,
			},
			validate: func(t *testing.T, m mqtt.Message) {
				var msg model.EbmsMessage
				fmt.Printf("Received message: %s\n", string(m.Payload()))
				require.NoError(t, json.Unmarshal(m.Payload(), &msg))
				fmt.Printf("M: %+v\n", msg)
				fmt.Printf("MM: %+v\n", msg.Meter)

				assert.Equal(t, "RC100130", msg.Sender)
				assert.Equal(t, "TT009999", msg.Receiver)
				assert.Equal(t, "TE00000001212000012121", msg.EcId)

				assert.Equal(t, "AT00000000000000000000000000122121", msg.Meter.MeteringPoint)
				assert.Equal(t, model.CONSUMPTION, msg.Meter.Direction)
				assert.Equal(t, 100, msg.Meter.PartFact)
				assert.Equal(t, int64(0), msg.Meter.From)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wg.Add(1)
			require.NoError(t, RegistrationForParticipation(tt.args.eeg, tt.args.meter, tt.args.from))
			wg.Wait()
			tt.validate(t, currentMsg)
		})
	}
}

func TestChangePartitionFactor(t *testing.T) {
	//_, err := NewMessageBrokerMock()
	//require.NoError(t, err)

	//messageBroker.Start()
	//
	//token := messageBroker.client.Connect()
	//if token != nil {
	//	token.Wait()
	//	require.NoError(t, token.Error())
	//}
	broker, _ := Broker().Init(newMockClient)

	var wg sync.WaitGroup
	wg.Add(1)
	broker.(*MessageBroker).cl.Subscribe("eda/request", 1, func(c mqtt.Client, m mqtt.Message) {
		var msg model.EbmsMessage
		require.NoError(t, json.Unmarshal(m.Payload(), &msg))
		fmt.Printf("M: %+v\n", msg)

		assert.Equal(t, "RC100130", msg.Sender)
		assert.Equal(t, "TT009999", msg.Receiver)
		assert.Equal(t, "TE00000001212000012121", msg.EcId)

		require.Equal(t, 1, len(msg.MeterList))
		assert.Equal(t, "AT00000000000000000000000000122121", msg.MeterList[0].MeteringPoint)
		assert.Equal(t, model.CONSUMPTION, msg.MeterList[0].Direction)
		assert.Equal(t, 10, msg.MeterList[0].PartFact)
		wg.Done()
	})

	eeg := &model.Eeg{
		Id:                 "TE000001",
		Name:               "Test-EEG",
		Description:        "",
		BusinessNr:         null.String{},
		Area:               "BEG",
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

	meters := []*model.ChangePartitionFactorRequest{
		&model.ChangePartitionFactorRequest{
			MeteringPoint:  "AT00000000000000000000000000122121",
			Direction:      "CONSUMPTION",
			GridOperatorId: null.StringFrom("TT009999"),
			Activation:     civil.Today(),
			PartFact:       10,
		},
	}

	require.NoError(t, ChangePartitionFactor(eeg, meters))
	wg.Wait()
}

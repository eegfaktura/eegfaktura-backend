package mqttclient

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"strings"
)

type TopicType string

var messageBroker *MessageBroker

func (t TopicType) Tenant() string {
	elems := strings.Split(string(t), "/")
	if len(elems) > 3 {
		return elems[3]
	}
	return string(t)
}

type InboundMessage struct {
	tenant string
	msg    []byte
}

type MessageBroker struct {
	callbackStore map[model.EbMsMessageType]model.SubscribeHandler
	Inbound       chan InboundMessage
	Outbound      chan model.EbmsMessage
	*MQTTStreamer
}

func StartMessageBroker() error {
	in := make(chan InboundMessage)
	out := make(chan model.EbmsMessage)

	streamer, err := NewMqttStreamer()
	if err != nil {
		return err
	}
	messageBroker = &MessageBroker{make(map[model.EbMsMessageType]model.SubscribeHandler), in, out, streamer}

	go messageBroker.Listen()

	return nil
}

func NewMessageBroker() (*MessageBroker, error) {
	in := make(chan InboundMessage)
	out := make(chan model.EbmsMessage)

	streamer, err := NewMqttStreamer()
	if err != nil {
		return nil, err
	}
	return &MessageBroker{make(map[model.EbMsMessageType]model.SubscribeHandler), in, out, streamer}, nil
}

func (mb *MessageBroker) SendMessage(m model.EbmsMessage, callback func(m string) error) {
	log.WithField("MSG", m.MessageCode).Info("Send Message to MQTT")
	payload, err := json.Marshal(m)
	if err != nil {
		log.WithField("error", err).Error("Marshaling EbmsMessage")
	}
	token := mb.client.Publish("eda/request", 1, false, payload)
	go func() {
		<-token.Done()
		if token.Error() != nil {
			log.Errorf("MQTT ERROR PUBLISHING: %s\n", token.Error())
		}
	}()
	token.Wait()
	callback("message sent")
}

func (mb *MessageBroker) Listen() {
	qos := 0
	token := mb.client.Subscribe("eda/response/#", byte(qos), func(client mqtt.Client, msg mqtt.Message) {
		log.Infof("Message from MQTT: %s [%+v]\n", TopicType(msg.Topic()).Tenant(), msg.Topic())
		mb.Inbound <- InboundMessage{strings.ToUpper(TopicType(msg.Topic()).Tenant()), msg.Payload()}
		msg.Ack()
	})
	token.Wait()
	if token.Error() != nil {
		panic(token.Error())
	}

	for {
		select {
		case msg := <-mb.Inbound:
			log.Infof("Message on topic: %s", msg.tenant)
			mb.received(msg)
		case send := <-mb.Outbound:
			mb.SendMessage(send, func(m string) error {
				fmt.Printf("Callback called: %+v\n", m)
				return nil
			})
		}
	}
}

func (mb *MessageBroker) Subscribe(subscriptions ...model.Subscriptions) {
	for _, s := range subscriptions {
		mb.callbackStore[s.MessageCode] = s.Handler
	}
}

func (mb *MessageBroker) Unsubscribe(subscriptions ...model.Subscriptions) {
	for _, s := range subscriptions {
		delete(mb.callbackStore, s.MessageCode)
	}
}

func (mb *MessageBroker) received(inbound InboundMessage) {
	msg := model.EbmsMessage{}
	err := json.Unmarshal(inbound.msg, &msg)
	if err != nil {
		log.Errorf("Error from MQTT: (%s) %v", inbound.tenant, inbound.msg)
		return
	}

	c, ok := mb.callbackStore[msg.MessageCode]
	if ok {
		c(model.SubscribeMessage{MessageCode: msg.MessageCode, Tenant: inbound.tenant, Payload: msg})
	}
}

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

func (t TopicType) Tenant() string {
	elems := strings.Split(string(t), "/")
	if len(elems) > 2 {
		return elems[2]
	}
	return string(t)
}

// SubscribeMessage aggregates the result from subscribing.
type SubscribeMessage struct {
	// Reports the index of corresponding SubscribeTopic.
	MessageCode model.EbMsMessageType

	// Reports the subscribed topic.
	Topic string

	// Reports the payload content.
	Payload []byte
}

type SubscribeHandler func(msg SubscribeMessage)

type Subscriptions struct {
	messageCode model.EbMsMessageType
	handler     SubscribeHandler
}

type InboundMessage struct {
	tenant string
	msg    []byte
}

type MessageBroker struct {
	callbackStore map[model.EbMsMessageType]SubscribeHandler
	Inbound       chan InboundMessage
	Outbound      chan model.EbmsMessage
	*MQTTStreamer
}

func NewMessageBroker() (*MessageBroker, error) {
	in := make(chan InboundMessage)
	out := make(chan model.EbmsMessage)

	streamer, err := NewMqttStreamer()
	if err != nil {
		return nil, err
	}
	return &MessageBroker{make(map[model.EbMsMessageType]SubscribeHandler), in, out, streamer}, nil
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
	qos := 1
	token := mb.client.Subscribe("eda/response/+", byte(qos), func(client mqtt.Client, msg mqtt.Message) {
		fmt.Printf("Message from MQTT: %s [%+v] %+v\n", TopicType(msg.Topic()).Tenant(), msg.Topic(), string(msg.Payload()))
		mb.Inbound <- InboundMessage{TopicType(msg.Topic()).Tenant(), msg.Payload()}
	})
	token.Wait()
	if token.Error() != nil {
		panic(token.Error())
	}

	for {
		select {
		case msg := <-mb.Inbound:
			fmt.Printf("Message Reveiced %+v\n", msg)
			mb.received(msg)
		case send := <-mb.Outbound:
			mb.SendMessage(send, func(m string) error {
				fmt.Printf("Callback called: %+v\n", m)
				return nil
			})
		}
	}
}

func (mb *MessageBroker) Subscribe(subscriptions ...Subscriptions) {
	for _, s := range subscriptions {
		mb.callbackStore[s.messageCode] = s.handler
	}
}

func (mb *MessageBroker) received(inbound InboundMessage) {
	message := model.EbmsMessage{}
	json.Unmarshal(inbound.msg, &message)

	c, ok := mb.callbackStore[message.MessageCode]
	if ok {
		payload, _ := json.Marshal(message)
		c(SubscribeMessage{message.MessageCode, inbound.tenant, payload})
	}
}

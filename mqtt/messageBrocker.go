package mqttclient

import (
	"at.ourproject/vfeeg-backend/database"
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
	if len(elems) > 4 {
		return elems[2]
	}
	return string(t)
}

func (t TopicType) TypeInfo() (string, string) {
	elems := strings.Split(string(t), "/")
	if len(elems) > 4 {
		return elems[2], elems[4]
	}
	return string(t), ""
}

type InboundMessage struct {
	tenant   string
	protocol model.EdaProtocol
	msg      []byte
}

type CommandMessage struct {
	tenant string
	cmd    string
	msg    []byte
}

type ErrorMessage struct {
	msg []byte
}

type MessageBroker struct {
	callbackStore map[model.EdaProtocol]model.SubscribeHandler
	Inbound       chan InboundMessage
	Command       chan CommandMessage
	ErrorC        chan ErrorMessage
	Outbound      chan model.EbmsMessage
	*MQTTStreamer
}

func NewMessageBroker() (*MessageBroker, error) {
	in := make(chan InboundMessage)
	out := make(chan model.EbmsMessage)
	cmd := make(chan CommandMessage)
	errC := make(chan ErrorMessage)

	streamer, err := NewMqttStreamer()
	if err != nil {
		return nil, err
	}
	messageBroker = &MessageBroker{
		make(map[model.EdaProtocol]model.SubscribeHandler),
		in,
		cmd,
		errC,
		out,
		streamer}

	return messageBroker, nil
}

func (mb *MessageBroker) Start() {
	go mb.Listen()
}

//func NewMessageBroker() (*MessageBroker, error) {
//	in := make(chan InboundMessage)
//	out := make(chan model.EbmsMessage)
//	cmd := make(chan CommandMessage)
//
//	streamer, err := NewMqttStreamer()
//	if err != nil {
//		return nil, err
//	}
//	return &MessageBroker{make(map[model.EdaProtocol]model.SubscribeHandler), in, cmd, out, streamer}, nil
//}

func (mb *MessageBroker) SendMessage(m model.EbmsMessage, callback func(m string) error) {
	log.WithField("tenant", m.Sender).WithField("MSG", m.MessageCode).Info("Send Message to MQTT")
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
	log.Info("Broker start listening ...")
	qos := 0
	tokenCommand := mb.client.Subscribe("eda/response/+/command/#", byte(qos), func(client mqtt.Client, msg mqtt.Message) {
		tenant, cmd := TopicType(msg.Topic()).TypeInfo()
		log.Infof("Command from MQTT: %s [%+s]", tenant, cmd)
		mb.Command <- CommandMessage{tenant: tenant, cmd: cmd, msg: msg.Payload()}
	})

	tokenCommand.Wait()
	if tokenCommand.Error() != nil {
		panic(tokenCommand.Error())
	}
	log.Info("Broker listening on command messages")

	token := mb.client.Subscribe("eda/response/+/protocol/#", byte(qos), func(client mqtt.Client, msg mqtt.Message) {
		log.Infof("Message from MQTT: %s [%+v]", TopicType(msg.Topic()).Tenant(), msg.Topic())
		tenant, protocol := TopicType(msg.Topic()).TypeInfo()
		mb.Inbound <- InboundMessage{
			strings.ToUpper(tenant),
			model.EdaProtocol(strings.ToUpper(protocol)),
			msg.Payload()}
	})
	token.Wait()
	if token.Error() != nil {
		panic(token.Error())
	}
	log.Info("Broker listening on protocol messages")

	errorToken := mb.client.Subscribe("eda/response/error", byte(qos), func(client mqtt.Client, msg mqtt.Message) {
		log.Infof("Error Message from MQTT: %s [%+v]", TopicType(msg.Topic()).Tenant(), msg.Topic())
		mb.ErrorC <- ErrorMessage{
			msg.Payload()}
	})
	errorToken.Wait()
	if token.Error() != nil {
		panic(token.Error())
	}
	log.Info("Broker listening on protocol messages")

	for {
		select {
		case msg := <-mb.Inbound:
			log.WithField("tenant", msg.tenant).Infof("Message on topic: %s", msg.protocol)
			mb.received(msg)
		case cmd := <-mb.Command:
			mb.command(cmd)
		case err := <-mb.ErrorC:
			log.Errorf("Error from Broker. %v", string(err.msg))
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
		mb.callbackStore[s.Protocol] = s.Handler
	}
}

func (mb *MessageBroker) Unsubscribe(subscriptions ...model.Subscriptions) {
	for _, s := range subscriptions {
		delete(mb.callbackStore, s.Protocol)
	}
}

func (mb *MessageBroker) received(inbound InboundMessage) {
	msg := model.EbmsMessage{}
	err := json.Unmarshal(inbound.msg, &msg)
	if err != nil {
		log.Errorf("Error from MQTT: (%s) %v - %v", inbound.tenant, inbound.protocol, err)
		return
	}
	c, ok := mb.callbackStore[inbound.protocol]
	if ok {
		c(model.SubscribeMessage{
			Protocol:    inbound.protocol,
			MessageCode: msg.MessageCode,
			Tenant:      inbound.tenant,
			Payload:     msg,
		})
	}
}

func (mb *MessageBroker) command(cmd CommandMessage) {
	msg := map[string]interface{}{}
	err := json.Unmarshal(cmd.msg, &msg)
	if err != nil {
		log.Errorf("Error from MQTT: (%s) cmd: %v - %v", cmd.tenant, cmd, err)
		return
	}

	switch cmd.cmd {
	case "pontononlinestate":
		online, ok := msg["online"]
		if ok {
			log.Infof("Update EEG Online State to %v", online)
			db, err := database.ConnectToDatabase()
			if err != nil {
				log.WithField("tenant", cmd.tenant).Error(err.Error())
				return
			}
			defer func() { _ = db.Close() }()

			if err := database.UpdateEegPartial(db, strings.ToUpper(cmd.tenant), map[string]interface{}{"online": online}); err != nil {
				log.Errorf("Error Command: %+v", err)
			}
		}
	}

}

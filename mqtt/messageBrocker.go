package mqttclient

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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
	client        mqtt.Client
}

func NewMessageBroker() (*MessageBroker, error) {
	in := make(chan InboundMessage)
	out := make(chan model.EbmsMessage)
	cmd := make(chan CommandMessage)
	errC := make(chan ErrorMessage, 10)

	client, err := NewMqttClient()
	if err != nil {
		return nil, err
	}

	messageBroker = &MessageBroker{
		make(map[model.EdaProtocol]model.SubscribeHandler),
		in,
		cmd,
		errC,
		out,
		client}

	messageBroker.ConfigRoutes()

	return messageBroker, nil
}

func (mb *MessageBroker) ConfigRoutes() {
	log.Infof("Configure MQTT Routes")
	client := mb.client
	client.AddRoute("eda/response/+/command/#", func(client mqtt.Client, msg mqtt.Message) {
		tenant, cmd := TopicType(msg.Topic()).TypeInfo()
		log.Infof("Command from MQTT: %s [%+s]", tenant, cmd)
		mb.Command <- CommandMessage{tenant: tenant, cmd: cmd, msg: msg.Payload()}
	})

	client.AddRoute("eda/response/+/protocol/#", func(client mqtt.Client, msg mqtt.Message) {
		log.Infof("Message from MQTT: %s [%+v] %v", TopicType(msg.Topic()).Tenant(), msg.Topic(), msg.Qos())
		tenant, protocol := TopicType(msg.Topic()).TypeInfo()
		mb.Inbound <- InboundMessage{
			strings.ToUpper(tenant),
			model.EdaProtocol(strings.ToUpper(protocol)),
			msg.Payload()}
	})

	client.AddRoute("eda/response/error", func(client mqtt.Client, msg mqtt.Message) {
		log.Infof("Error Message from MQTT: [%+v] %d", msg.Topic(), msg.Qos())
		mb.ErrorC <- ErrorMessage{
			msg.Payload()}
	})
}

func (mb *MessageBroker) Connect() error {

	client := mb.client

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return token.Error()
	}

	qos := viper.GetInt("mqtt.qos")
	if token := client.Subscribe("eda/response/+/command/#", byte(qos), nil); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	log.Info("Broker subscribe to command messages")

	if token := client.Subscribe("eda/response/+/protocol/#", byte(qos), nil); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	log.Info("Broker subscribe to protocol messages")

	if token := client.Subscribe("eda/response/error", byte(qos), nil); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	log.Info("Broker subscribe to tenant error messages")

	return nil
}

func (mb *MessageBroker) Start() error {
	go mb.Listen()
	return mb.Connect()
}

func (mb *MessageBroker) Stop() {
	if token := mb.client.Unsubscribe("eda/response/+/protocol/#", "eda/response/error", "eda/response/+/command/#"); token.Wait() && token.Error() == nil {
		mb.client.Disconnect(2000)
	}
}

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

	for {
		select {
		case msg := <-mb.Inbound:
			log.WithField("tenant", msg.tenant).Infof("Message on topic: %s", msg.protocol)
			mb.received(msg)
			//log.WithField("tenant", msg.tenant).Infof("Message on topic: %s handeled", msg.protocol)
		case cmd := <-mb.Command:
			mb.command(cmd)
		case err := <-mb.ErrorC:
			log.Errorf("Error from Broker. %v", string(err.msg))
		case send := <-mb.Outbound:
			mb.SendMessage(send, func(m string) error {
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
		log.WithField("tenant", inbound.tenant).WithError(err).Errorf("Error from MQTT: %v", inbound.protocol)
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

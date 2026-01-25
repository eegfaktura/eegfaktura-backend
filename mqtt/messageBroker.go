package mqttclient

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"context"
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"strings"
	"sync"
)

var (
	once     sync.Once
	instance IMessageBroker
)

type CloseC struct {
	C      chan bool
	closed bool
	mutex  sync.Mutex
}

func NewCloseC() *CloseC {
	return &CloseC{C: make(chan bool)}
}

func (mc *CloseC) SafeClose() {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()
	if !mc.closed {
		close(mc.C)
		mc.closed = true
	}
}

func (mc *CloseC) IsClosed() bool {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()
	return mc.closed
}

type IMessageBroker interface {
	Init(newClient func(p IMessageBroker) (mqtt.Client, error)) (IMessageBroker, error)
	OnConnect(ctx context.Context, cl mqtt.Client)
	OnDisConnect(cl mqtt.Client)
	SendMessage(msg model.EbmsMessage)
	Subscribe(subscriptions ...model.Subscriptions)
	Unsubscribe(subscriptions ...model.Subscriptions)
	Stop()
}

type MessageBroker struct {
	Inbound       chan InboundMessage
	Outbound      chan model.EbmsMessage
	Command       chan CommandMessage
	ErrorC        chan ErrorMessage
	callbackStore map[model.EdaProtocol]model.SubscribeHandler
	close         *CloseC
	cl            mqtt.Client
}

func Broker() IMessageBroker {
	once.Do(func() {
		var err error
		instance, err = NewMessageBroker()
		if err != nil {
			panic(err)
		}
	})
	return instance
}

func NewMessageBroker() (*MessageBroker, error) {
	in := make(chan InboundMessage)
	out := make(chan model.EbmsMessage)
	cmd := make(chan CommandMessage)
	errC := make(chan ErrorMessage, 10)

	messageBrokerV2 := &MessageBroker{
		in,
		out,
		cmd,
		errC,
		make(map[model.EdaProtocol]model.SubscribeHandler),
		NewCloseC(),
		nil,
	}
	return messageBrokerV2, nil
}

func (m *MessageBroker) Init(newClient func(p IMessageBroker) (mqtt.Client, error)) (IMessageBroker, error) {
	client, err := newClient(m)
	if err != nil {
		return nil, err
	}

	m.ConfigRoutes(client)

	token := client.Connect()
	if token.Wait(); token.Error() != nil {
		log.Fatalf("Couldn't connect to MQTT broker: %v\n", token.Error())
		return nil, token.Error()
	}
	m.cl = client
	go m.Listen()
	return m, nil
}

func (m *MessageBroker) OnConnect(ctx context.Context, cl mqtt.Client) {

	qos := viper.GetInt("mqtt.qos")
	if token := cl.Subscribe("eda/response/+/command/#", byte(qos), nil); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	log.Info("Broker subscribe to command messages")

	if token := cl.Subscribe("eda/response/+/protocol/#", byte(qos), nil); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	log.Info("Broker subscribe to protocol messages")

	if token := cl.Subscribe("eda/response/error", byte(qos), nil); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	log.Info("Broker subscribe to tenant error messages")

}

func (m *MessageBroker) OnDisConnect(cl mqtt.Client) {
	m.close.SafeClose()
}

func (m *MessageBroker) ConfigRoutes(cl mqtt.Client) {
	log.Infof("Configure MQTT Routes")
	cl.AddRoute("eda/response/+/command/#", func(client mqtt.Client, msg mqtt.Message) {
		tenant, cmd := TopicType(msg.Topic()).TypeInfo()
		log.WithField("tenant", tenant).Infof("Command from MQTT: Command: %s", cmd)
		m.Command <- CommandMessage{tenant: tenant, cmd: cmd, msg: msg.Payload()}
	})

	cl.AddRoute("eda/response/+/protocol/#", func(client mqtt.Client, msg mqtt.Message) {
		tenant, protocol := TopicType(msg.Topic()).TypeInfo()
		log.WithField("tenant", tenant).Infof("Message from MQTT: Topic: %+v Protocol: %s QoS: %v", msg.Topic(), protocol, msg.Qos())
		m.Inbound <- InboundMessage{
			strings.ToUpper(tenant),
			model.EdaProtocol(strings.ToUpper(protocol)),
			msg.Payload()}
	})

	cl.AddRoute("eda/response/error", func(client mqtt.Client, msg mqtt.Message) {
		log.Infof("Error Message from MQTT: Topic: %v Msg: %+v QoS: %d", msg.Topic(), string(msg.Payload()), msg.Qos())
		m.ErrorC <- ErrorMessage{
			msg.Payload()}
	})
}

func (m *MessageBroker) Listen() {
	log.Info("Broker start listening ...")
	m.close = NewCloseC()

	for {
		select {
		case <-m.close.C:
			log.Info("Broker stopped listening ...")
			return
		case msg := <-m.Inbound:
			m.received(msg)
		case cmd := <-m.Command:
			m.command(cmd)
		case err := <-m.ErrorC:
			_ = err
		case msg := <-m.Outbound:
			m.sendMessage(msg)
		}
	}
}

func (m *MessageBroker) Subscribe(subscriptions ...model.Subscriptions) {
	for _, s := range subscriptions {
		m.callbackStore[s.Protocol] = s.Handler
	}
}

func (m *MessageBroker) Unsubscribe(subscriptions ...model.Subscriptions) {
	for _, s := range subscriptions {
		delete(m.callbackStore, s.Protocol)
	}
}

func (m *MessageBroker) Stop() {
	m.close.SafeClose()
	if m.cl != nil {
		m.cl.Disconnect(500)
	}
}

func (m *MessageBroker) received(inbound InboundMessage) {
	msg := model.EbmsMessage{}
	err := json.Unmarshal(inbound.msg, &msg)
	if err != nil {
		log.WithField("tenant", inbound.tenant).WithError(err).Errorf("Error from MQTT: %v", inbound.protocol)
		return
	}
	c, ok := m.callbackStore[inbound.protocol]
	if ok {
		c(model.SubscribeMessage{
			Protocol:    inbound.protocol,
			MessageCode: msg.MessageCode,
			Tenant:      inbound.tenant,
			Payload:     msg,
		})
	}
}

func (m *MessageBroker) command(cmd CommandMessage) {
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
			db, err := database.GetDB(context.Background())
			if err != nil {
				log.WithField("tenant", cmd.tenant).Error(err.Error())
				return
			}

			if err := db.UpdateOnlineState(strings.ToUpper(cmd.tenant), online.(bool)); err != nil {
				log.Errorf("Error Command: %+v", err)
			}
		}
	}
}

func (m *MessageBroker) sendMessage(msg model.EbmsMessage) {
	log.WithField("tenant", msg.Sender).WithField("MSG", msg.MessageCode).Info("Send Message to MQTT")

	if m.cl == nil {
		log.Warn("Broker not connected!")
		return
	}

	payload, err := json.Marshal(msg)
	if err != nil {
		log.WithField("error", err).Error("Marshaling EbmsMessage")
	}

	token := m.cl.Publish("eda/request", 1, false, payload)
	go func() {
		<-token.Done()
		if token.Error() != nil {
			log.Errorf("MQTT ERROR PUBLISHING: %s\n", token.Error())
		}
	}()
	token.Wait()
}

func (m *MessageBroker) SendMessage(msg model.EbmsMessage) {

	version := viper.GetString(fmt.Sprintf("eda-process-versions.%s", msg.MessageCode))
	if len(version) > 0 {
		msg.MessageCodeVersion = version
	}

	m.Outbound <- msg
	log.WithField("tenant", msg.Sender).Infof("Message sent successfully Protokol: %s", msg.MessageCode)
}

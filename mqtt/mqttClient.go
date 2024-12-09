package mqttclient

import (
	"at.ourproject/vfeeg-backend/model"
	"errors"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"time"
)

type MQTTStreamer struct {
	client mqtt.Client
}
type Error string

var (
	MqttBrokerNotStarted = errors.New("Broker not running")
)

func NewMqttClient() (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()

	brokerHost := viper.GetString("mqtt.host")
	brokerId := viper.GetString("mqtt.id")

	log.Infof("Use MQTT broker with address %s and Id %s", brokerHost, brokerId)

	opts.AddBroker(brokerHost)
	opts.SetClientID(brokerId)

	opts.SetOrderMatters(true)            // Allow out of order messages (use this option unless in order delivery is essential)
	opts.ConnectTimeout = 2 * time.Second // Minimal delays on connect
	opts.WriteTimeout = 2 * time.Second   // Minimal delays on writes
	opts.KeepAlive = 10                   // Keepalive every 10 seconds so we quickly detect network outages
	opts.PingTimeout = 10 * time.Second   // local broker so response should be quick

	// Automate connection management (will keep trying to connect and will reconnect if network drops)
	opts.ConnectRetry = true
	opts.AutoReconnect = true
	opts.CleanSession = false
	//opts.ProtocolVersion = 4
	//opts.SetStore(mqtt.NewFileStore(":memory:"))

	//opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
	//	log.Infof("Default message handler: [%+v] %d", msg.Topic(), msg.Qos())
	//})

	// Log events
	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		log.Info("connection lost")
	}
	opts.OnConnect = func(mqtt.Client) {
		log.Info("MQTT connection established")
	}
	//opts.OnConnect = onConnect
	opts.OnReconnecting = func(cl mqtt.Client, co *mqtt.ClientOptions) {
		log.Info("attempting to reconnect")
	}

	//mqtt.ERROR = log.New()
	//mqtt.CRITICAL = log.New()
	//mqtt.WARN = log.New()
	//mqtt.DEBUG = log.New()

	client := mqtt.NewClient(opts)

	return client, nil
}

func Subscribe(subscriptions ...model.Subscriptions) error {
	if messageBroker != nil {
		messageBroker.Subscribe(subscriptions...)
		return nil
	}
	return MqttBrokerNotStarted
}

func Unsubscribe(subscriptions ...model.Subscriptions) error {
	if messageBroker != nil {
		messageBroker.Unsubscribe(subscriptions...)
		return nil
	}
	return MqttBrokerNotStarted
}

func SendEbmsMessage(msg model.EbmsMessage) error {

	version := viper.GetString(fmt.Sprintf("eda-process-versions.%s", msg.MessageCode))
	if len(version) > 0 {
		msg.MessageCodeVersion = version
	}

	if messageBroker != nil {
		messageBroker.Outbound <- msg
		log.WithField("tenant", msg.Sender).Infof("Message sent successfully")
		return nil
	}
	return MqttBrokerNotStarted
}

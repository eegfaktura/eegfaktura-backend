package mqttclient

import (
	"context"
	"strings"
	"time"

	"at.ourproject/vfeeg-backend/model"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type TopicType string

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

func NewMqttClient(broker IMessageBroker) (mqtt.Client, error) {
	opts := mqtt.NewClientOptions()

	brokerHost := viper.GetString("mqtt.host")
	brokerId := viper.GetString("mqtt.id")

	log.Infof("Use MQTT broker with address %s and Id %s", brokerHost, brokerId)

	opts.AddBroker(brokerHost)
	opts.SetClientID(brokerId)
	opts.SetProtocolVersion(4)
	opts.SetAutoAckDisabled(false)
	opts.SetCleanSession(false)

	opts.SetOrderMatters(false)            // Allow out of order messages (use this option unless in order delivery is essential)
	opts.ConnectTimeout = 30 * time.Second // Minimal delays on connect
	opts.WriteTimeout = 30 * time.Second   // Minimal delays on writes
	opts.KeepAlive = 60                    // Keepalive every 60 seconds so we quickly detect network outages
	opts.PingTimeout = 30 * time.Second    // local broker so response should be quick

	// Automate connection management (will keep trying to connect and will reconnect if network drops)
	opts.ConnectRetry = true
	opts.AutoReconnect = true

	ctx := context.Background()

	// Log events
	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		log.Info("connection lost")
	}

	opts.OnConnect = func(cl mqtt.Client) {
		log.Info("MQTT connection established ...")
		broker.OnConnect(ctx, cl)
	}
	//opts.OnConnect = onConnect
	opts.OnReconnecting = func(cl mqtt.Client, co *mqtt.ClientOptions) {
		log.Info("attempting to reconnect ...")
	}

	//mqtt.ERROR = log.New()
	//mqtt.CRITICAL = log.New()
	//mqtt.WARN = log.New()
	//mqtt.DEBUG = log.New()

	client := mqtt.NewClient(opts)

	return client, nil
}

package main

import (
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"os"
	"time"
)

func main() {

	opts := mqtt.NewClientOptions()

	brokerHost := "tcp://localhost:1883"
	brokerId := "edash-test-publisher-1"

	log.Infof("Use MQTT broker with address %s and Id %s", brokerHost, brokerId)

	opts.AddBroker(brokerHost)
	opts.SetClientID(brokerId)

	opts.SetOrderMatters(true)        // Allow out of order messages (use this option unless in order delivery is essential)
	opts.ConnectTimeout = time.Second // Minimal delays on connect
	opts.WriteTimeout = time.Second   // Minimal delays on writes
	opts.KeepAlive = 10               // Keepalive every 10 seconds so we quickly detect network outages
	opts.PingTimeout = time.Second    // local broker so response should be quick

	// Automate connection management (will keep trying to connect and will reconnect if network drops)
	opts.ConnectRetry = true
	opts.AutoReconnect = true

	// Log events
	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		log.Info("connection lost")
	}
	opts.OnConnect = func(mqtt.Client) {
		log.Info("MQTT connection established")
	}
	opts.OnReconnecting = func(mqtt.Client, *mqtt.ClientOptions) {
		log.Info("attempting to reconnect")
	}

	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	payload, err := os.ReadFile("./energy-message-test.json")
	if err != nil {
		panic(err)
	}

	for _ = range [100]int{} {
		token := client.Publish("eda/response/energy/te100101", byte(1), false, payload)
		token.Wait()
		err = token.Error()
		if err != nil {
			fmt.Printf("Send error %*v\n", err)
		}
	}

}

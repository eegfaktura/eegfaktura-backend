package mqttclient

import "at.ourproject/vfeeg-backend/model"

func NewMessageBrokerMock() (*MessageBroker, error) {
	in := make(chan InboundMessage)
	out := make(chan model.EbmsMessage)
	cmd := make(chan CommandMessage)
	errC := make(chan ErrorMessage)

	streamer := &MQTTStreamer{client: newMockClient()}
	messageBroker = &MessageBroker{
		make(map[model.EdaProtocol]model.SubscribeHandler),
		in,
		cmd,
		errC,
		out,
		streamer}

	return messageBroker, nil
}

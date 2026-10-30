package mqttclient

import (
	"at.ourproject/vfeeg-backend/model"
	"github.com/sirupsen/logrus"
)

func GetSubsriptions() []model.Subscriptions {
	return []model.Subscriptions{
		{
			Protocol: model.ERROR,
			Handler:  errorHandler,
		},
	}
}

func errorHandler(msg model.SubscribeMessage) {
	logrus.WithField("tenant", msg.Tenant).Errorf("Receive Error from EDA COMMUNICATION. Code: %s conversation: %s", msg.MessageCode, msg.Payload.ConversationId)
}

func InitErrorSubscriptions() {
	Broker().Subscribe(GetSubsriptions()...)
}

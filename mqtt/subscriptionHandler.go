package mqttclient

import (
	"at.ourproject/vfeeg-backend/model"
	"github.com/sirupsen/logrus"
)

func GetSubsriptions() []model.Subscriptions {
	return []model.Subscriptions{
		{
			model.ERROR,
			errorHandler,
		},
	}
}

func errorHandler(msg model.SubscribeMessage) {
	logrus.Errorf("Receive Error from EDA COMMUNICATION. Reason: %+v", msg)
}

func InitErrorSubscriptions() {
	Broker().Subscribe(GetSubsriptions()...)
}

package mqttclient

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"github.com/sirupsen/logrus"
)

func GetSubsriptions() []Subscriptions {
	return []Subscriptions{{
		model.EBMS_ERROR_MESSAGE,
		errorHandler,
	}}
}

func errorHandler(msg SubscribeMessage) {
	if err := database.SaveNotification(msg.Topic, string(msg.Payload), "ERROR", "USER"); err != nil {
		logrus.Error(err)
	}
}

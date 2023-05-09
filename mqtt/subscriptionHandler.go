package mqttclient

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"github.com/sirupsen/logrus"
)

func GetSubsriptions() []model.Subscriptions {
	return []model.Subscriptions{{
		model.EBMS_ERROR_MESSAGE,
		errorHandler,
	}}
}

func errorHandler(msg model.SubscribeMessage) {
	var err error
	var msgBytes []byte
	if msgBytes, err = json.Marshal(msg.Payload); err == nil {
		if err = database.SaveNotification(msg.Tenant, string(msgBytes), "ERROR", "USER"); err != nil {
			logrus.Error(err)
		}
		return
	}
	logrus.Errorf("Parse object to json: %v", err)
}

func InitErrorSubscriptions() {
	messageBroker.Subscribe(GetSubsriptions()...)
}

package mqttclient

import (
	"github.com/eegfaktura/eegfaktura-backend/model"
	"github.com/sirupsen/logrus"
)

func GetSubsriptions() []model.Subscriptions {
	return []model.Subscriptions{{
		model.ERROR,
		errorHandler,
	}}
}

func errorHandler(msg model.SubscribeMessage) {
	//var err error
	//var msgBytes []byte
	//if msgBytes, err = json.Marshal(msg.Payload); err == nil {
	//	if err = recorder.saveNotification(string(msgBytes), msg.Tenant, "ERROR", "USER"); err != nil {
	//		logrus.Error(err)
	//	}
	//	return
	//}
	logrus.Errorf("Receive Error from EDA COMMUNICATION. Reason: %v", msg)

}

func InitErrorSubscriptions() {
	messageBroker.Subscribe(GetSubsriptions()...)
}

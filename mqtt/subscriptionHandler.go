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

// errorHandler logs an ERROR message of eegfaktura-eda-xp with its reason (errorMessage), e.g. a refused request
// (eegfaktura-dev#56 #4). The user does not see it yet.
func errorHandler(msg model.SubscribeMessage) {
	logrus.WithFields(logrus.Fields{"tenant": msg.Tenant, "event.action": "eda.error.received", "code": string(msg.MessageCode)}).
		WithField("error.message", msg.Payload.ErrorMessage).
		Errorf("error message from eda-xp, conversation %s", msg.Payload.ConversationId)
}

func InitErrorSubscriptions() {
	Broker().Subscribe(GetSubsriptions()...)
}

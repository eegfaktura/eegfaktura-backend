package eda

import (
	"encoding/json"
	"time"

	"github.com/eegfaktura/eegfaktura-backend/database"
	"github.com/eegfaktura/eegfaktura-backend/model"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
)

type EdaRecording interface {
	saveNotification(notificationValue map[string]interface{}, tenant, notificationType, role string) error
	saveHistory(tenant string, messageCode model.EbMsMessageType, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error
	databaseConnect() (*sqlx.DB, error)
}

type EdaRecorder struct {
	dbOpen database.OpenDbXConnection
}

func NewEdaRecorder() *EdaRecorder {
	return &EdaRecorder{dbOpen: database.GetDBXConnection}
}

func (r *EdaRecorder) databaseConnect() (*sqlx.DB, error) {
	return r.dbOpen()
}

func (r *EdaRecorder) saveNotification(notificationValue map[string]interface{}, tenant, notificationType, role string) error {
	var msgBytes []byte
	var err error
	if msgBytes, err = json.Marshal(notificationValue); err == nil {
		if err = database.SaveNotification(r.dbOpen, tenant, string(msgBytes), notificationType, role); err != nil {
			logrus.Error(err)
			return err
		}
	}
	return nil
}

func (r *EdaRecorder) saveHistory(tenant string, messageCode model.EbMsMessageType, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error {
	db, err := database.GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	var msgBytes []byte
	if msgBytes, err = json.Marshal(msg); err == nil {
		if err = database.SaveEdaHistory(r.dbOpen, &model.EdaProcessHistory{
			Tenant:         tenant,
			ConversationId: conversationId,
			ProcessType:    messageCode,
			Date:           time.Time{},
			Protocol:       null.StringFrom(string(protocol)),
			Issuer:         role,
			MessageByte:    msgBytes,
			MessageMap:     nil,
			Direction:      dir,
		}); err != nil {
			logrus.Error(err)
			return err
		}
	}
	return nil
}

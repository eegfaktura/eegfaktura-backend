package eda

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/parser"
	"at.ourproject/vfeeg-backend/services"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
	"time"
)

type EdaRecording interface {
	saveNotification(notificationValue map[string]interface{}, tenant, notificationType, role string) error
	saveHistory(tenant string, messageCode model.EbMsMessageType, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error
	meteringPointPerformAnswerMsg(tenant string, meterId []string) error
	databaseConnectFunc() database.OpenDbXConnection
	databaseConnection() (*sqlx.DB, error)
}

type EdaRecorder struct {
	dbOpen database.OpenDbXConnection
}

func NewEdaRecorder() *EdaRecorder {
	return &EdaRecorder{dbOpen: database.GetDBXConnection}
}

func (r *EdaRecorder) databaseConnectFunc() database.OpenDbXConnection {
	return r.dbOpen
}

func (r *EdaRecorder) databaseConnection() (*sqlx.DB, error) {
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

	var err error
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

func (r *EdaRecorder) meteringPointPerformAnswerMsg(tenant string, meterId []string) error {

	eeg, err := database.GetEeg(database.GetDBXConnection, tenant)
	if err != nil {
		return err
	}

	db, err := r.dbOpen()
	if err != nil {
		return err
	}
	defer func() {
		err = db.Close()
		if err != nil {
			logrus.Errorf("Error Close Database: %v", err)
		}
	}()

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, mid := range meterId {
		participant, err := database.FindParticipantByMeteringPoint(db, tenant, mid)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			} else {
				logrus.WithField("tenant", tenant).Warn(err)
			}
		}
		if participant != nil && participant.Contact.Email.Valid {
			if err = parser.SendActivationMailFromTemplate(services.SendMail,
				tenant, "Aktivierung im Serviceportal", eeg, participant); err != nil {
				logrus.Errorf("Error Sending Mail: %+v", err.Error())
			}
		}
	}
	return tx.Commit()
}

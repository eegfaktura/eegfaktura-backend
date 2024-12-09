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
	saveNotification(db *sqlx.DB, tenant string, code model.EbMsMessageType, meters []string, errCodes []int16, protocol model.EdaProtocol)
	saveHistory(db *sqlx.DB, tenant string, messageCode model.EbMsMessageType, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error
	meteringPointPerformAnswerMsg(ecId string, meterId []string) error
	databaseConnectFunc() database.OpenDbXConnection
	databaseConnection() (*sqlx.DB, error)
}

type EdaRecorder struct {
	dbOpen database.OpenDbXConnection
}

func NewEdaRecorder() *EdaRecorder {
	return &EdaRecorder{dbOpen: database.ConnectToDatabase}
}

func (r *EdaRecorder) databaseConnectFunc() database.OpenDbXConnection {
	return r.dbOpen
}

func (r *EdaRecorder) databaseConnection() (*sqlx.DB, error) {
	return r.dbOpen()
}

func (r *EdaRecorder) saveNotification(db *sqlx.DB, tenant string, code model.EbMsMessageType, meters []string, errCodes []int16, protocol model.EdaProtocol) {
	var err error
	notificationValue := map[string]interface{}{
		"type":           code,
		"meteringPoints": meters,
		"responseCodes":  convertCodes2Strings(errCodes),
	}

	if err = database.SaveNotificationFromMap(db, notificationValue, tenant, model.N_TYPE_MESSAGE, model.N_PROCESS_EDA_PROCESS, "ADMIN"); err != nil {
		logrus.WithField("PROTOCOL", protocol).Error(err)
	}
}

func (r *EdaRecorder) saveHistory(db *sqlx.DB, tenant string, messageCode model.EbMsMessageType, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error {

	var err error
	var msgBytes []byte
	if msgBytes, err = json.Marshal(msg); err == nil {
		if err = database.SaveEdaHistory(db, &model.EdaProcessHistory{
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

func (r *EdaRecorder) meteringPointPerformAnswerMsg(ecId string, meterId []string) error {

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

	eeg, err := database.GetEegByEcId(db, ecId)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for _, mid := range meterId {
		participant, err := database.FindParticipantByMeteringPoint(db, eeg.Id, mid)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			} else {
				logrus.WithField("tenant", eeg.Id).Warn(err)
			}
		}
		if participant != nil && participant.Contact.Email.Valid {
			if err = parser.SendActivationMailFromTemplate(services.SendMail,
				eeg.Id, "Aktivierung im Serviceportal", eeg, participant); err != nil {
				logrus.WithField("tenant", eeg.Id).WithError(err).Error("Error Sending Mail")
			}
		}
	}
	return tx.Commit()
}

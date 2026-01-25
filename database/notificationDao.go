package database

import (
	"at.ourproject/vfeeg-backend/model"
	dbsql "database/sql"
	"encoding/json"
	"github.com/doug-martin/goqu/v9"
	"github.com/jjeffery/civil"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
	"math"
	"strings"
	"time"
)

type NotificationRepository interface {
	SaveNotification(tenant string, code model.EbMsMessageType, meters []string, errCodes []string, protocol model.EdaProtocol) error
	SaveNotificationFromMap(notificationValue map[string]interface{}, tenant string, notificationType model.NotificationType, process model.NotificationProcess, role string) error
	SaveHistory(tenant string, code model.EbMsMessageType, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error
	FetchEdaHistory(tenant, protocol string, start, end int64, pageSize uint) (interface{}, error)
	GetNotification(tenant string, start int64, isAdmin bool) ([]model.EegNotification, error)
}

func (db *sqlDatabase) SaveNotification(tenant string, code model.EbMsMessageType, meters []string, errCodes []string, protocol model.EdaProtocol) error {
	var err error
	notificationValue := map[string]interface{}{
		"type":           code,
		"meteringPoints": meters,
		"responseCodes":  errCodes,
	}

	if err = db.SaveNotificationFromMap(notificationValue, tenant, model.N_TYPE_MESSAGE, model.N_PROCESS_EDA_PROCESS, "ADMIN"); err != nil {
		logrus.WithField("PROTOCOL", protocol).Error(err)
	}
	return err
}

func (db *sqlDatabase) SaveNotificationFromMap(notificationValue map[string]interface{}, tenant string, notificationType model.NotificationType, process model.NotificationProcess, role string) error {
	var msgBytes []byte
	var err error
	if msgBytes, err = json.Marshal(notificationValue); err == nil {
		if err = createNotification(db.db, tenant, string(msgBytes), notificationType, process, role); err != nil {
			logrus.Error(err)
			return err
		}
	}
	return nil
}

func (db *sqlDatabase) GetNotification(tenant string, start int64, isAdmin bool) ([]model.EegNotification, error) {
	return getNotification(db.db, tenant, start, isAdmin)
}

func (db *sqlDatabase) SaveHistory(tenant string, code model.EbMsMessageType, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error {

	var err error
	var msgBytes []byte
	if msgBytes, err = json.Marshal(msg); err == nil {
		if err = saveEdaHistory(db.db, &model.EdaProcessHistory{
			Tenant:         tenant,
			ConversationId: conversationId,
			ProcessType:    code,
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

func (db *sqlDatabase) FetchEdaHistory(tenant, protocol string, start, end int64, pageSize uint) (interface{}, error) {
	return fetchEdaHistory(db.db, tenant, protocol, start, end, pageSize)
}

func saveEdaHistory(db *sqlx.DB, history *model.EdaProcessHistory) error {
	sql, _, err := pgDialect.Insert("base.processhistory").Rows(history).ToSQL()
	_, err = db.Exec(sql)
	return err
}

func fetchEdaHistory(db *sqlx.DB, tenant, protocol string, start, end int64, pageSize uint) (interface{} /*map[string]map[string][]model.EdaProcessHistory*/, error) {
	startDate := civil.DateOf(time.UnixMilli(start))
	endDate := civil.DateOf(time.UnixMilli(end))

	protocolArray := strings.Split(protocol, ";")
	h := []model.EdaProcessHistory{}
	block := pgDialect.From("base.processhistory").Select(&h).
		Order(goqu.I("date").Asc()).
		Where(
			goqu.C("tenant").Eq(tenant), /*goqu.C("protocol").IsNotNull(),*/
			goqu.C("protocol").Eq(protocolArray),
			goqu.C("date").Between(goqu.Range(startDate, endDate)))

	if pageSize > 0 {
		block.Limit(pageSize + 1)
	}

	stmt, _, err := block.ToSQL()

	//fmt.Printf("STMT: %v\n", stmt)
	err = db.Select(&h, stmt)
	if err != nil && err != dbsql.ErrNoRows {
		logrus.WithField("SQL", "SELECT").Errorf("Query History: %+v", stmt)
		return nil, err
	}

	resultSize := len(h)
	if pageSize > 0 {
		resultSize = int(math.Min(float64(len(h)), float64(pageSize)))
	}
	out := map[string]map[string][]model.EdaProcessHistory{}
	for i := 0; i < resultSize; i++ {
		e := h[i]
		err = json.Unmarshal(e.MessageByte, &e.MessageMap)
		if ci, ok := out[e.Protocol.String]; ok {
			ci[e.ConversationId] = append(ci[e.ConversationId], e)
		} else {
			ci := map[string][]model.EdaProcessHistory{}
			ci[e.ConversationId] = []model.EdaProcessHistory{e}
			out[e.Protocol.String] = ci
		}
	}

	type NextType struct {
		Start    int64  `json:"start,omitempty"`
		End      int64  `json:"end,omitempty"`
		Protocol string `json:"protocol,omitempty"`
		PageSize uint   `json:"page_size,omitempty"`
	}

	next := NextType{}
	if len(h) > int(pageSize) {
		last := h[pageSize]
		next.Start = last.Date.UnixMilli()
		next.End = end
		next.Protocol = last.Protocol.String
		next.PageSize = pageSize
	}

	result := struct {
		History struct {
			Next NextType `json:"next,omitempty"`
		}
		Data map[string]map[string][]model.EdaProcessHistory `json:"data"`
	}{
		History: struct {
			Next NextType `json:"next,omitempty"`
		}{
			Next: next,
		},
		Data: out,
	}

	return result, err
}

func createNotification(db *sqlx.DB, tenant, notification string,
	msgType model.NotificationType, process model.NotificationProcess, role string) error {
	stmt, _, err := pgDialect.Insert("base.notification").
		Rows(
			goqu.Record{"tenant": tenant, "notification": notification, "type": msgType, "role": role, "process": process},
		).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = db.Exec(stmt)
	return err
}

func getNotification(db *sqlx.DB, tenant string, start int64, isAdmin bool) ([]model.EegNotification, error) {
	n := []model.EegNotification{}

	statement := pgDialect.From("base.notification").Select(&n).
		Where(goqu.C("tenant").Eq(tenant), goqu.C("id").Gt(start))
	if !isAdmin {
		statement = statement.Where(goqu.C("role").Eq("USER"))
	}

	sql, _, err := statement.Order(goqu.I("id").Desc()).Limit(30).ToSQL()
	if err != nil {
		return nil, err
	}
	err = db.Select(&n, sql)
	if err != nil && err != dbsql.ErrNoRows {
		return nil, err
	}

	return n, err
}

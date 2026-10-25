package database

import (
	"context"
	dbsql "database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"at.ourproject/vfeeg-backend/energyrequest"
	"at.ourproject/vfeeg-backend/model"
	"github.com/doug-martin/goqu/v9"
	"github.com/jjeffery/civil"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
)

type NotificationRepository interface {
	SaveNotification(tenant string, code model.EbMsMessageType, meters []string, errCodes []string, protocol model.EdaProtocol) error
	SaveNotificationFromMap(notificationValue map[string]interface{}, tenant string, notificationType model.NotificationType, process model.NotificationProcess, role string) error
	SaveHistory(tenant string, code model.EbMsMessageType, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error
	FetchEdaHistory(tenant, protocol string, start, end int64, pageSize uint) (interface{}, error)
	GetNotification(ctx context.Context, tenant string, start int64, isAdmin bool) ([]model.EegNotification, error)
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

// notificationPageSize is how many notifications the list returns.
const notificationPageSize = 30

// GetNotification lists the newest notifications of tenant after id start. For admins, an open energy request run
// with a summary is listed as a live entry (the current summary with live: true, at the run's row id and creation
// time) and its answers are left out; they are listed again once the stored summary has replaced the live entry.
// When the energy request rows cannot be read, the list comes without live entries (logged: a cancelled or timed out
// request as warning, any other error as error).
func (db *sqlDatabase) GetNotification(ctx context.Context, tenant string, start int64, isAdmin bool) ([]model.EegNotification, error) {
	if !isAdmin {
		return getNotification(db.db, tenant, start, isAdmin, nil)
	}
	rows, tenants, err := db.energyRequestCommunityCached(ctx, tenant)
	if err != nil {
		entry := logrus.WithField("tenant", tenant).WithError(err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			entry.Warn("notifications: no live energy request entries")
		} else {
			entry.Error("notifications: no live energy request entries")
		}
		return getNotification(db.db, tenant, start, isAdmin, nil)
	}
	live, hidden := energyrequest.Live(rows, tenant, tenants, time.Now())
	n, err := getNotification(db.db, tenant, start, isAdmin, hidden)
	if err != nil {
		return nil, err
	}
	for _, e := range live {
		if e.RowId <= start {
			continue
		}
		msg, err := json.Marshal(e.Summary)
		if err != nil {
			return nil, err
		}
		n = append(n, model.EegNotification{Id: e.RowId, MsgType: string(model.N_TYPE_MESSAGE),
			Process: model.N_PROCESS_EDA_PROCESS, Message: msg, Date: time.UnixMilli(e.CreatedAt).UTC()})
	}
	sort.Slice(n, func(i, j int) bool { return n[i].Id > n[j].Id })
	if len(n) > notificationPageSize {
		n = n[:notificationPageSize]
	}
	return n, nil
}

func (db *sqlDatabase) SaveHistory(tenant string, code model.EbMsMessageType, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error {

	var err error
	var msgBytes []byte
	if msgBytes, err = json.Marshal(msg); err == nil {
		if err = saveEdaHistory(db.db, &model.EdaProcessHistory{
			Tenant:         tenant,
			ConversationId: conversationId,
			ProcessType:    code,
			Date:           civil.DateTime{},
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
	if pageSize < uint(len(h)) {
		last := h[pageSize]
		next.Start = last.Date.Unix() * 1000
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

func createNotification(db sqlx.Execer, tenant, notification string,
	msgType model.NotificationType, process model.NotificationProcess, role string) error {
	stmt, _, err := pgDialect.Insert("base.notification").
		Rows(
			// date explizit als UTC setzen statt dem Spalten-Default `now()`: die Spalte ist
			// `timestamp without time zone`; `now()` schreibt lokale Wanduhrzeit (CEST), die als
			// `...Z` serialisiert wird → Frontend rechnet +Offset doppelt. `now() at time zone 'utc'`
			// liefert die UTC-Wanduhrzeit → korrekter Instant.
			goqu.Record{"tenant": tenant, "notification": notification, "type": msgType, "role": role, "process": process,
				"date": goqu.L("(now() AT TIME ZONE 'utc')")},
		).
		ToSQL()
	if err != nil {
		return err
	}

	_, err = db.Exec(stmt)
	return err
}

func getNotification(db *sqlx.DB, tenant string, start int64, isAdmin bool, hidden map[int64]bool) ([]model.EegNotification, error) {
	n := []model.EegNotification{}

	statement := pgDialect.From("base.notification").Select(&n).
		Where(goqu.C("tenant").Eq(tenant), goqu.C("id").Gt(start),
			goqu.C("process").Neq(model.N_PROCESS_ENERGY_REQUEST))
	if !isAdmin {
		statement = statement.Where(goqu.C("role").Eq("USER"))
	}
	if len(hidden) > 0 {
		ids := make([]int64, 0, len(hidden))
		for id := range hidden {
			ids = append(ids, id)
		}
		statement = statement.Where(goqu.C("id").NotIn(ids))
	}

	sql, _, err := statement.Order(goqu.I("id").Desc()).Limit(notificationPageSize).ToSQL()
	if err != nil {
		return nil, err
	}
	err = db.Select(&n, sql)
	if err != nil && err != dbsql.ErrNoRows {
		return nil, err
	}

	return n, err
}

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

// collapseBatch is how many notifications GetNotification reads at once for admins; collapseMaxBatches bounds the
// reads per call. A request for a whole community leaves out a request and an answer per metering point, so the
// visible page can lie a few thousand rows back.
const (
	collapseBatch      = 200
	collapseMaxBatches = 25
)

// GetNotification lists the newest notifications of tenant after id start. For admins, the requests and answers of
// an energy request with a summary are left out (energyrequest.Collapse): while the run is open, a live entry (the
// current summary with live: true, at the run's row id and creation time) stands for them, placed with the newest of
// its messages; once closed, the stored summary. When the energy request rows cannot be read, the list comes without
// live entries and only the stored summaries stand for their messages (logged: a cancelled or timed out request as
// warning, any other error as error).
func (db *sqlDatabase) GetNotification(ctx context.Context, tenant string, start int64, isAdmin bool) ([]model.EegNotification, error) {
	if !isAdmin {
		return getNotification(db.db, tenant, start)
	}
	rows, tenants, err := db.energyRequestCommunityCached(ctx, tenant)
	if err != nil {
		entry := logrus.WithField("tenant", tenant).WithError(err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			entry.Warn("notifications: no live energy request entries")
		} else {
			entry.Error("notifications: no live energy request entries")
		}
		n, _, err := collapsedNotifications(context.WithoutCancel(ctx), db.db, tenant, start, nil, nil)
		return n, err
	}
	live, hidden := energyrequest.Live(rows, tenant, tenants, time.Now())
	n, latest, err := collapsedNotifications(ctx, db.db, tenant, start, rows, hidden)
	if err != nil {
		return nil, err
	}
	// a live entry is placed with its newest message; its id stays the run's row id
	place := map[int64]int64{}
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
		place[e.RowId] = max(e.RowId, latest[e.RowId])
	}
	key := func(x model.EegNotification) int64 {
		if p, ok := place[x.Id]; ok {
			return p
		}
		return x.Id
	}
	sort.SliceStable(n, func(i, j int) bool { return key(n[i]) > key(n[j]) })
	if len(n) > notificationPageSize {
		n = n[:notificationPageSize]
	}
	return n, nil
}

// collapsedNotifications reads the newest admin notifications of tenant after id start in batches and leaves out
// hidden and the messages energyrequest.Collapse assigns to a run or stored summary, until a page is full. runs are
// the community rows of the live entries (nil: none could be read). latest is as in energyrequest.Collapse.
func collapsedNotifications(ctx context.Context, db *sqlx.DB, tenant string, start int64, runs []energyrequest.Row,
	hidden map[int64]bool) ([]model.EegNotification, map[int64]int64, error) {
	visible := []model.EegNotification{}
	latest := map[int64]int64{}
	summaries := []energyrequest.Message{}
	before := int64(math.MaxInt64)
	for i := 0; i < collapseMaxBatches && len(visible) < notificationPageSize; i++ {
		batch := []model.EegNotification{}
		statement := pgDialect.From("base.notification").Select(&batch).
			Where(goqu.C("tenant").Eq(tenant), goqu.C("id").Gt(start), goqu.C("id").Lt(before),
				goqu.C("process").Neq(model.N_PROCESS_ENERGY_REQUEST)).
			Order(goqu.I("id").Desc()).Limit(collapseBatch)
		sql, _, err := statement.ToSQL()
		if err != nil {
			return nil, nil, err
		}
		if err = db.SelectContext(ctx, &batch, sql); err != nil && !errors.Is(err, dbsql.ErrNoRows) {
			return nil, nil, err
		}
		if len(batch) == 0 {
			break
		}
		before = batch[len(batch)-1].Id
		messages := make([]energyrequest.Message, 0, len(batch))
		for _, b := range batch {
			m := energyrequest.Message{Id: b.Id, Date: b.Date, Content: json.RawMessage(b.Message)}
			if energyrequest.IsSummary(m.Content) {
				summaries = append(summaries, m)
			}
			messages = append(messages, m)
		}
		collapsed, l := energyrequest.Collapse(runs, summaries, messages)
		for id, last := range l {
			latest[id] = max(latest[id], last)
		}
		for _, b := range batch {
			if hidden[b.Id] || collapsed[b.Id] {
				continue
			}
			visible = append(visible, b)
			if len(visible) == notificationPageSize {
				break
			}
		}
		if len(batch) < collapseBatch {
			break
		}
	}
	return visible, latest, nil
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

// getNotification lists the newest notifications for users (role USER) of tenant after id start.
func getNotification(db *sqlx.DB, tenant string, start int64) ([]model.EegNotification, error) {
	n := []model.EegNotification{}

	statement := pgDialect.From("base.notification").Select(&n).
		Where(goqu.C("tenant").Eq(tenant), goqu.C("id").Gt(start),
			goqu.C("process").Neq(model.N_PROCESS_ENERGY_REQUEST), goqu.C("role").Eq("USER"))

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

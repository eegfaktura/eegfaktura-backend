package database

import (
	"at.ourproject/vfeeg-backend/model"
	dbsql "database/sql"
	"encoding/json"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"time"
)

func SaveEdaHistory(dbOpen OpenDbXConnection, history *model.EdaProcessHistory) error {
	db, err := dbOpen()
	if err != nil {
		return err
	}
	defer db.Close()

	sql, _, err := goqu.Insert("base.processhistory").Rows(history).ToSQL()
	_, err = db.Exec(sql)
	return err
}

func FetchEdaHistory(db *sqlx.DB, tenant string, start, end int64) (map[string]map[string][]model.EdaProcessHistory, error) {
	startDate := time.UnixMilli(start)
	endDate := time.UnixMilli(end)

	h := []model.EdaProcessHistory{}
	stmt, _, err := pgDialect.From("base.processhistory").Select(&h).
		Where(goqu.C("tenant").Eq(tenant), goqu.C("protocol").IsNotNull(), goqu.C("date").Between(goqu.Range(startDate, endDate))).ToSQL()

	err = db.Select(&h, stmt)
	if err != nil && err != dbsql.ErrNoRows {
		logrus.WithField("SQL", "SELECT").Errorf("Query History: %+v", stmt)
		return nil, err
	}

	out := map[string]map[string][]model.EdaProcessHistory{}
	for _, e := range h {
		err = json.Unmarshal(e.MessageByte, &e.MessageMap)
		if ci, ok := out[e.Protocol.String]; ok {
			ci[e.ConversationId] = append(ci[e.ConversationId], e)
		} else {
			ci := map[string][]model.EdaProcessHistory{}
			ci[e.ConversationId] = []model.EdaProcessHistory{e}
			out[e.Protocol.String] = ci
		}
	}

	return out, err
}

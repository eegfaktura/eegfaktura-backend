package database

import (
	"at.ourproject/vfeeg-backend/model"
	dbsql "database/sql"
	"encoding/json"
	"github.com/doug-martin/goqu/v9"
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

func FetchEdaHistory(dbOpen OpenDbXConnection, tenant string) (map[string]map[string][]model.EdaProcessHistory, error) {
	db, err := dbOpen()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	h := []model.EdaProcessHistory{}
	sql, _, err := pgDialect.From("base.processhistory").Select(&h).
		Where(goqu.C("tenant").Eq(tenant), goqu.C("protocol").IsNotNull()).ToSQL()

	err = db.Select(&h, sql)
	if err != nil && err != dbsql.ErrNoRows {
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
		//fmt.Printf("R: %+v\n", e)
	}

	return out, err
}

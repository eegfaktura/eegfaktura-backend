package database

import (
	"at.ourproject/vfeeg-backend/model"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
)

const TABLE_EEG = "base.eeg"
const TABLE_EEG_ADDRESS = "base.address"

type EegRepository interface {
	GetEegById(tenant string) (*model.Eeg, error)
	GetEegByEcId(edId string) (*model.Eeg, error)
	UpdateEegPartial(tenant string, fields map[string]interface{}) error
	GetGridOperators() (map[string]string, error)
	FetchTenantsName(tenants []string, isSuperUser bool) ([]tenantsNameStruct, error)
	InsertEeg(tenant string, eeg *model.Eeg) error
	UpdateOnlineState(tenant string, onlineState bool) error
}

func (db *sqlDatabase) GetEegById(tenant string) (*model.Eeg, error) {
	return getEegById(db.db, tenant)
}

func (db *sqlDatabase) GetEegByEcId(edId string) (*model.Eeg, error) {
	return getEegByEcId(db.db, edId)
}

func (db *sqlDatabase) UpdateEegPartial(tenant string, fields map[string]interface{}) error {
	return updateEegPartial(db.db, tenant, fields)
}

func (db *sqlDatabase) GetGridOperators() (map[string]string, error) {
	return getGridOperators(db.db)
}

func (db *sqlDatabase) FetchTenantsName(tenants []string, isSuperUser bool) ([]tenantsNameStruct, error) {
	return fetchTenantsName(db.db, tenants, isSuperUser)
}

func (db *sqlDatabase) InsertEeg(tenant string, eeg *model.Eeg) error {
	return insertEeg(db.db, tenant, eeg)
}

func (db *sqlDatabase) UpdateOnlineState(tenant string, onlineState bool) error {

	stmt, _, err := goqu.Update(TABLE_EEG).
		Set(goqu.Record{"online": onlineState}).
		Where(goqu.Ex{"tenant": goqu.V(tenant)}).
		ToSQL()

	if err != nil {
		return err
	}

	_, err = db.db.Exec(stmt)
	if err != nil {
		return err
	}
	return nil
}

func getEegById(tx *sqlx.DB, tenant string) (*model.Eeg, error) {

	var eeg model.Eeg
	stmt, _, err := pgDialect.From(TABLE_EEG).Select(&eeg).Where(goqu.C("tenant").Eq(tenant)).ToSQL()
	if err != nil {
		return nil, model.ErrGetEeg(err)
	}

	err = tx.Get(&eeg, stmt)
	if err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return nil, model.ErrGetEeg(err)
	}
	return &eeg, nil
}

func getEegByEcId(tx *sqlx.DB, edId string) (*model.Eeg, error) {

	var eeg model.Eeg
	stmt, _, err := pgDialect.From(TABLE_EEG).Select(&eeg).Where(goqu.C("communityId").Eq(edId)).ToSQL()
	if err != nil {
		return nil, model.ErrGetEeg(err)
	}

	err = tx.Get(&eeg, stmt)
	if err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return nil, model.ErrGetEeg(err)
	}
	return &eeg, nil
}

func insertEeg(db *sqlx.DB, tenant string, eeg *model.Eeg) error {

	sql, _, err := pgDialect.Insert(TABLE_EEG).Rows(eeg).OnConflict(goqu.DoNothing()).ToSQL()
	_, err = db.Exec(sql)
	if err != nil {
		log.WithField("SQL", "INSERT").Errorf("Stmt: %s", sql)
		return err
	}

	return err
}

func updateEegPartial(db *sqlx.DB, tenant string, fields map[string]interface{}) error {
	var eeg model.Eeg
	updateRecord, err := buildRecordMap(&eeg, fields)
	if err != nil {
		return err
	}
	statement, _, err := pgDialect.Update(TABLE_EEG).Set(updateRecord).Where(goqu.Ex{"tenant": goqu.V(tenant)}).ToSQL()
	if err != nil {
		log.WithError(err).Errorf("Update EEG VALUES: %s", statement)
		return err
	}

	_, err = db.Exec(statement)
	return err
}

func getGridOperators(db *sqlx.DB) (map[string]string, error) {

	sql, _, err := pgDialect.From("base.gridoperators").ToSQL()

	rows, err := db.Query(sql)
	if err != nil {
		return nil, err
	}

	var id string
	var name string
	result := map[string]string{}
	for rows.Next() {
		err = rows.Scan(&id, &name)
		if err != nil {
			return nil, err
		}
		result[id] = name
	}

	return result, nil
}

type tenantsNameStruct struct {
	Tenant string `json:"tenant" db:"tenant"`
	Name   string `json:"name" db:"name"`
}

func fetchTenantsName(db *sqlx.DB, tenants []string, isSuperUser bool) ([]tenantsNameStruct, error) {
	tenantsName := []tenantsNameStruct{}
	selectStmt := pgDialect.From(TABLE_EEG).Select(&tenantsName)
	if !isSuperUser {
		selectStmt = selectStmt.Where(goqu.C("tenant").In(tenants))
	}
	stmt, _, err := selectStmt.ToSQL()
	if err != nil {
		return nil, err
	}
	if err := db.Select(&tenantsName, stmt); err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return nil, err
	}
	return tenantsName, nil
}

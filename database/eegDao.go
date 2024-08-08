package database

import (
	"at.ourproject/vfeeg-backend/model"
	dbsql "database/sql"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
)

const TABLE_EEG = "base.eeg"
const TABLE_EEG_ADDRESS = "base.address"

func GetEeg(tx *sqlx.DB, tenant string) (*model.Eeg, error) {
	var eeg model.Eeg
	err := tx.QueryRow(""+
		"SELECT name, description, \"businessNr\", legal, gridoperator_name, \"communityId\", gridoperator_code, \"rcNumber\", area, \"allocationMode\", "+
		"\"settlementInterval\", \"providerBusinessNr\", street, \"streetNumber\", zip, city, phone, email, website, iban, owner, sepa, \"bankName\", "+
		"\"taxNumber\", \"vatNumber\", online, \"contactPerson\" FROM base.eeg WHERE tenant = $1", tenant).
		Scan(&eeg.Name, &eeg.Description, &eeg.BusinessNr, &eeg.Legal, &eeg.OperatorName,
			&eeg.CommunityId, &eeg.GridOperator, &eeg.RcNumber, &eeg.Area,
			&eeg.AllocationMode, &eeg.SettlementInterval, &eeg.ProviderBusinessNr,
			&eeg.Street, &eeg.StreetNumber, &eeg.Zip, &eeg.City, &eeg.Contact.Phone, &eeg.Contact.Email,
			&eeg.Optionals.Website, &eeg.AccountInfo.Iban, &eeg.AccountInfo.Owner, &eeg.AccountInfo.Sepa, &eeg.AccountInfo.BankName,
			&eeg.TaxNumber, &eeg.VatNumber, &eeg.Online, &eeg.ContactPerson,
		)
	if err == dbsql.ErrNoRows {
		return nil, nil
	}
	eeg.Id = tenant
	return &eeg, err
}

func GetEegById(tx *sqlx.DB, tenant string) (*model.Eeg, error) {

	var eeg model.Eeg
	stmt, _, err := pgDialect.From("base.eeg").Select(&eeg).Where(goqu.C("tenant").Eq(tenant)).ToSQL()
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

func GetEegByEcId(tx *sqlx.DB, edId string) (*model.Eeg, error) {

	var eeg model.Eeg
	stmt, _, err := pgDialect.From("base.eeg").Select(&eeg).Where(goqu.C("communityId").Eq(edId)).ToSQL()
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

func InsertEeg(db *sqlx.DB, tenant string, eeg *model.Eeg) error {

	sql, _, err := pgDialect.Insert("base.eeg").Rows(eeg).ToSQL()
	log.Printf("Stmt: %s", sql)
	_, err = db.Exec(sql)
	if err != nil {
		log.WithField("SQL", "INSERT").Errorf("Stmt: %s", sql)
		return err
	}

	return err
}

func UpdateEegPartial(db *sqlx.DB, tenant string, fields map[string]interface{}) error {
	statement, _, _ := pgDialect.Update(TABLE_EEG).Set(fields).Where(goqu.Ex{"tenant": goqu.V(tenant)}).ToSQL()

	log.Debugf("Update EEG VALUES: %s\n", statement)

	_, err := db.Exec(statement)
	return err
}

//func UpdateEegAddressPartial(tenant string, fields map[string]interface{}) error {
//	db, err := GetDBXConnection()
//	if err != nil {
//		return err
//	}
//	defer db.Close()
//
//	statement, _, _ := pgDialect.Update(TABLE_EEG_ADDRESS).Set(fields).Where(goqu.Ex{"tenant": goqu.V(tenant)}).ToSQL()
//
//	log.Debugf("Update EEG VALUES: %s\n", statement)
//
//	_, err = db.Exec(statement)
//	return err
//}

func SaveNotification(dbOpen OpenDbXConnection, tenant string, notification string, msgType, role string) error {
	db, err := dbOpen()
	if err != nil {
		return err
	}
	defer func() {
		_ = db.Close()
	}()

	_, err = db.Exec("INSERT INTO base.notification (tenant, notification, date, type, role) VALUES ($1, $2, NOW(), $3, $4)", tenant, notification, msgType, role)
	return err
}

func GetNotification(db *sqlx.DB, tenant string, start int64, isAdmin bool) ([]model.EegNotification, error) {
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

func GetGridOperators(db *sqlx.DB) (map[string]string, error) {

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

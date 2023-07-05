package database

import (
	"at.ourproject/vfeeg-backend/model"
	"database/sql"
	"github.com/doug-martin/goqu/v9"
	log "github.com/sirupsen/logrus"
)

const TABLE_EEG = "base.eeg"
const TABLE_EEG_ADDRESS = "base.address"

func GetEeg(tenant string) (*model.Eeg, error) {

	db, err := GetDBXConnection()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var eeg model.Eeg
	err = db.QueryRow(""+
		"SELECT name, \"businessNr\", legal, gridoperator_name, \"communityId\", gridoperator_code, \"rcNumber\", \"allocationMode\", "+
		"\"settlementInterval\", \"providerBusinessNr\", street, \"streetNumber\", zip, city, phone, email, website, iban, owner, sepa, "+
		"\"taxNumber\", \"vatNumber\", online FROM base.eeg WHERE tenant = $1", tenant).
		Scan(&eeg.Name, &eeg.BusinessNr, &eeg.Legal, &eeg.OperatorName,
			&eeg.CommunityId, &eeg.GridOperator, &eeg.RcNumber,
			&eeg.AllocationMode, &eeg.SettlementInterval, &eeg.ProviderBusinessNr,
			&eeg.Street, &eeg.StreetNumber, &eeg.Zip, &eeg.City, &eeg.Contact.Phone, &eeg.Contact.Email,
			&eeg.Optionals.Website, &eeg.AccountInfo.Iban, &eeg.AccountInfo.Owner, &eeg.AccountInfo.Sepa,
			&eeg.TaxNumber, &eeg.VatNumber, &eeg.Online,
		)
	if err == sql.ErrNoRows {
		return &eeg, nil
	}
	eeg.Id = tenant
	return &eeg, err
}

func UpdateEeg(tenant string, eeg *model.Eeg) error {

	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(""+
		`INSERT INTO base.eeg (tenant, name, "businessNr", legal, gridoperator_name, "communityId", gridoperator_code, `+
		`"rcNumber", "allocationMode", "settlementInterval", "providerBusinessNr", "taxNumber", "vatNumber", `+
		`street, "streetNumber", city, zip, phone, email, website, iban, owner, sepa, online) `+
		`VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, `+
		`$12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, %24) `+
		`ON CONFLICT (tenant, name, "rcNumber") `+
		`DO UPDATE SET "businessNr"=$3, legal=$4, gridoperator_name=$5, "communityId"=$6, gridoperator_code=$7, `+
		`"allocationMode"=$9, "settlementInterval"=$10, "providerBusinessNr"=$11, "taxNumber"= $12, "vatNumber"=$13, `+
		`street=$14, street_number=$15, city=$16, zip=$17, phone=$18, email=$19, website=$20, iban=$21, `+
		`owner=$22, sepa=$23, online=$24`,
		tenant, eeg.Name, eeg.BusinessNr, eeg.Legal, eeg.OperatorName,
		eeg.CommunityId, eeg.GridOperator, eeg.RcNumber, eeg.AllocationMode,
		eeg.SettlementInterval, eeg.ProviderBusinessNr, eeg.TaxNumber, eeg.VatNumber, eeg.Street, eeg.StreetNumber, eeg.City, eeg.Zip,
		eeg.Contact.Phone, eeg.Contact.Email, eeg.Optionals.Website, eeg.AccountInfo.Iban, eeg.AccountInfo.Owner, eeg.AccountInfo.Sepa, eeg.Online)
	if err != nil {
		return err
	}

	return err
}

func UpdateEegPartial(tenant string, fields map[string]interface{}) error {
	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	statement, _, _ := pgDialect.Update(TABLE_EEG).Set(fields).Where(goqu.Ex{"tenant": goqu.V(tenant)}).ToSQL()

	log.Debugf("Update EEG VALUES: %s\n", statement)

	_, err = db.Exec(statement)
	return err
}

func UpdateEegAddressPartial(tenant string, fields map[string]interface{}) error {
	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	statement, _, _ := pgDialect.Update(TABLE_EEG_ADDRESS).Set(fields).Where(goqu.Ex{"tenant": goqu.V(tenant)}).ToSQL()

	log.Debugf("Update EEG VALUES: %s\n", statement)

	_, err = db.Exec(statement)
	return err
}

func GetCommunityId(tenant string) (string, error) {

	db, err := GetDBConnection()
	if err != nil {
		return "", err
	}
	defer db.Close()

	communityId := ""
	err = db.QueryRow(`SELECT "communityId" FROM base.eeg WHERE tenant = $1`, tenant).Scan(&communityId)

	return communityId, err
}

//func fetchEegAddressInfo(db sqlx.DB, tenant string)

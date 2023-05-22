package database

import (
	"at.ourproject/vfeeg-backend/model"
	"database/sql"
)

func GetEeg(tenant string) (*model.Eeg, error) {

	db, err := GetDBXConnection()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var eeg model.Eeg
	err = db.QueryRow(""+
		"SELECT name, businessNr, legal, gridoperator_name, communityId, gridoperator_code, rcNumber, allocationMode, "+
		"settlementInterval, providerBusinessNr, street, street_number, zip, city, phone, email, website, iban, owner, sepa, "+
		"taxid, vatid, online FROM base.eeg WHERE tenant = $1", tenant).
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
		"INSERT INTO base.eeg "+
		" (tenant, name, businessNr, legal, gridoperator_name, communityId, gridoperator_code, rcNumber, allocationMode, settlementInterval, providerBusinessNr, "+
		"street, street_number, city, zip, phone, email, website, iban, owner, sepa) "+
		"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, "+
		"$12, $13, $14, $15, $16, $17, $18, $19, $20, $21) "+
		"ON CONFLICT (tenant, name, rcnumber) "+
		"DO UPDATE SET businessNr=$3, legal=$4, gridoperator_name=$5, communityId=$6, gridoperator_code=$7, "+
		"allocationMode=$9, settlementInterval=$10, providerBusinessNr=$11, "+
		"street=$12, street_number=$13, city=$14, zip=$15, phone=$16, email=$17, website=$18, iban=$19, "+
		"owner=$20, sepa=$21",
		tenant, eeg.Name, eeg.BusinessNr, eeg.Legal, eeg.OperatorName,
		eeg.CommunityId, eeg.GridOperator, eeg.RcNumber, eeg.AllocationMode,
		eeg.SettlementInterval, eeg.ProviderBusinessNr, eeg.Street, eeg.StreetNumber, eeg.City, eeg.Zip,
		eeg.Contact.Phone, eeg.Contact.Email, eeg.Optionals.Website, eeg.AccountInfo.Iban, eeg.AccountInfo.Owner, eeg.AccountInfo.Sepa)
	if err != nil {
		return err
	}

	return err
}

func GetCommunityId(tenant string) (string, error) {

	db, err := GetDBConnection()
	if err != nil {
		return "", err
	}
	defer db.Close()

	communityId := ""
	err = db.QueryRow("SELECT communityid FROM base.eeg WHERE tenant = $1", tenant).Scan(&communityId)

	return communityId, err
}

//func fetchEegAddressInfo(db sqlx.DB, tenant string)

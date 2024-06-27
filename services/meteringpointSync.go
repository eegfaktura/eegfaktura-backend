package services

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	log "github.com/sirupsen/logrus"
)

func SyncMeteringPoints(tenant string, ebms *model.EbmsMessage) error {

	db, err := database.ConnectToDatabase()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	tx, err := db.Beginx()
	if err != nil {
		log.Errorf("Not able to open a transaction. %s", err.Error())
		return err
	}

	defer func() {
		switch err {
		case nil:
			err = tx.Commit()
		default:
			err = tx.Rollback()
		}
	}()

	return database.UpdateMeteringPoints(tx, tenant, model.ConvertFromMeterList(ebms.MeterList))
}

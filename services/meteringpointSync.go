package services

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
)

func SyncMeteringPoints(tenant string, ebms *model.EbmsMessage) error {

	db, err := database.ConnectToDatabase()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	return database.UpdateActiveMeteringPoints(db, tenant, ebms.MeterList)
}

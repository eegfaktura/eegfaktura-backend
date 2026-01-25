package services

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"context"
)

func SyncMeteringPoints(tenant string, ebms *model.EbmsMessage) error {

	db, err := database.GetDB(context.Background())
	if err != nil {
		return err
	}

	return db.UpdateActiveMeteringPoints(tenant, ebms.MeterList)
}

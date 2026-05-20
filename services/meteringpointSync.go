package services

import (
	"context"
	"time"

	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
)

func SyncMeteringPoints(tenant string, ebms *model.EbmsMessage) error {

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	db, err := database.GetDB(ctx)
	if err != nil {
		return err
	}

	return db.UpdateActiveMeteringPoints(ctx, tenant, ebms.MeterList)
}

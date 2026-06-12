package repository

import (
	"context"
	"time"

	"at.ourproject/vfeeg-backend/database"
)

type EegRepository struct {
	db database.Database
}

func (er *EegRepository) UpdatePartial(tenant string, values map[string]string) error {
	var err error

	fields := map[string]interface{}{}
	for k, v := range values {
		fields[k] = v
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err = er.db.UpdateEegPartial(ctx, tenant, fields); err != nil {
		return err
	}
	return nil
}

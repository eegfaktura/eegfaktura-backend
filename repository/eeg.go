package repository

import (
	"at.ourproject/vfeeg-backend/database"
	"github.com/jmoiron/sqlx"
)

type EegRepository struct {
	db *sqlx.DB
}

func (er *EegRepository) UpdatePartial(tenant string, values map[string]string) error {
	var err error

	fields := map[string]interface{}{}
	for k, v := range values {
		fields[k] = v
	}

	if err = database.UpdateEegPartial(er.db, tenant, fields); err != nil {
		return err
	}
	return nil
}

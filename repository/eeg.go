package repository

import (
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

	if err = er.db.UpdateEegPartial(tenant, fields); err != nil {
		return err
	}
	return nil
}

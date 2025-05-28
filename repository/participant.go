package repository

import (
	"at.ourproject/vfeeg-backend/database"
	"github.com/jmoiron/sqlx"
)

type ParticipantRepository struct {
	db *sqlx.DB
}

func (pr *ParticipantRepository) UpdateParticipant(tenant, participantId string, values map[string]string) error {
	var err error
	for k, v := range values {
		if err = database.UpdateParticipantPartial(pr.db, participantId, k, v); err != nil {
			return err
		}
	}
	return nil
}

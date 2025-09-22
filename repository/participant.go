package repository

import (
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
)

type ParticipantRepository struct {
	db      *sqlx.DB
	dialect goqu.DialectWrapper
}

//var (
//	TABLE_PARTICIPANT = "base.participant"
//)

//func (pr *ParticipantRepository) UpdateParticipant(tenant, participantId string, values map[string]string) error {
//	var err error
//	for k, v := range values {
//		if err = database.UpdateParticipantPartial(pr.db, participantId, k, v); err != nil {
//			return err
//		}
//	}
//	return nil
//}

//func (pr *ParticipantRepository) DeleteParticipant(participantId string) error {
//	return database.DeleteParticipant(pr.db, participantId)
//}

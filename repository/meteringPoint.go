package repository

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"errors"
	"github.com/jjeffery/civil"
	"github.com/jmoiron/sqlx"
)

type MeteringPointRepository struct {
	db *sqlx.DB
}

func (mrepo *MeteringPointRepository) UpdateProcessStatus(
	tenant string, meters []string,
	processState model.ProcessStatusType, statusCode *int16, activeSince, inactiveSince *civil.Date, consentId *string) error {

	var defaultStatusCode int16 = 0

	switch processState {
	case model.NEW:
		fallthrough
	case model.PENDING:
		fallthrough
	case model.INIT:
		return database.MeteringPointsSetStatus(mrepo.db, tenant, processState, &defaultStatusCode, meters, nil, nil)
	case model.APPROVED:
		return database.MeteringPointsSetStatus(mrepo.db, tenant, processState, &defaultStatusCode, meters, nil, consentId)
	case model.ACTIVE:
		return database.MeteringPointsSetStatus(mrepo.db, tenant, processState, &defaultStatusCode, meters, activeSince, consentId)
	case model.REVOKED:
		return database.MeteringPointsSetStatus(mrepo.db, tenant, processState, statusCode, meters, nil, nil)
	case model.INACTIVE:
		if inactiveSince == nil {
			today := civil.Today()
			inactiveSince = &today
		}
		return database.MeteringPointRevoke(mrepo.db, tenant, meters[0], *inactiveSince)
	}
	return errors.New("invalid process state")
}

func (mrepo *MeteringPointRepository) UpdateActiveSinceDate(tenant, participantId, meter, username string, activeSince *civil.Date) error {
	return database.UpdateMeteringPointPartial(mrepo.db, tenant, username, participantId, meter, map[string]interface{}{"activesince": activeSince})
}

func (mrepo *MeteringPointRepository) UpdateInActiveSinceDate(tenant, participantId, meter, username string, inactiveSince *civil.Date) error {
	return database.UpdateMeteringPointPartial(mrepo.db, tenant, username, participantId, meter, map[string]interface{}{"inactivesince": inactiveSince})
}

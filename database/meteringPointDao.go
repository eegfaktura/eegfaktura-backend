package database

import (
	"at.ourproject/vfeeg-backend/model"
	"fmt"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
	"time"
)

const TABLE_METERINGPOINT = "base.meteringpoint"

type meteringEntryType struct {
	*model.MeteringPoint
	Participant_id string
	Tenant         string
}

func createMeteringEntries(tenant, participantId string, points []*model.MeteringPoint, state *model.StatusType) []*meteringEntryType {
	meteringEntries := []*meteringEntryType{}
	for _, p := range points {
		if state != nil {
			p.Status = *state
		}
		p.ModifiedBy = null.StringFrom("SYSTEM")
		p.ModifiedAt = time.Now()
		meteringEntries = append(meteringEntries, &meteringEntryType{p, participantId, tenant})
	}
	return meteringEntries
}

func RegisterMeteringPoints(tx *sqlx.Tx, tenant, participantId string, point []*model.MeteringPoint) error {
	state := model.NEW
	return saveMeteringPoint(tx, createMeteringEntries(tenant, participantId, point, &state))
}

func ImportMeteringPoints(tx *sqlx.Tx, tenant, participantId string, point []*model.MeteringPoint) error {
	return saveMeteringPoint(tx, createMeteringEntries(tenant, participantId, point, nil))
}

func saveMeteringPoint(tx *sqlx.Tx, meteringEntry []*meteringEntryType) error {
	statement, _, _ := pgDialect.Insert(TABLE_METERINGPOINT).Rows(meteringEntry).ToSQL()
	log.Debugf("Register Meterings: %+v", statement)
	_, err := tx.Exec(statement)

	if err != nil {
		return err
	}

	type participantMeterState struct {
		Participant_id    string `db:"participant_id"`
		Tenant            string
		Metering_point_id string `db:"metering_point"`
		Changed_by        string
		ActiveSince       time.Time `db:"activesince"`
	}

	stateEntries := []participantMeterState{}
	for _, e := range meteringEntry {
		e.State = &model.MeterState{
			ActiveSince:   e.RegisteredSince,
			InactiveSince: time.Date(2999, 12, 31, 0, 0, 0, 0, time.Local),
		}

		stateEntries = append(stateEntries, participantMeterState{
			Participant_id:    e.Participant_id,
			Tenant:            e.Tenant,
			Metering_point_id: e.MeteringPoint.MeteringPoint,
			Changed_by:        e.ModifiedBy.String,
			ActiveSince:       e.RegisteredSince,
		})
	}

	statement, _, _ = pgDialect.Insert("base.participant_meter_state").Rows(stateEntries).ToSQL()
	log.Debugf("Register Meterings: %+v", statement)
	_, err = tx.Exec(statement)

	return err
}

func RegisterMeteringPoint(openDb OpenDbXConnection, tenant, participantId string, point *model.MeteringPoint) error {
	db, err := openDb()
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.Beginx()
	if err != nil {
		log.Errorf("Not able to open a transaction. %s", err.Error())
		return err
	}

	defer func() {
		if err = tx.Commit(); err != nil {
			log.Errorf("Not able to commit the transaction. %s", err.Error())
		}
	}()

	return saveMeteringPoint(tx, createMeteringEntries(tenant, participantId, []*model.MeteringPoint{point}, &point.Status))
	//return RegisterMeteringPoints(tx, tenant, participantId, []*model.MeteringPoint{point})

	//type meteringEntryType struct {
	//	*model.MeteringPoint
	//	ParticipantId string `db:"participant_id"`
	//	Tenant        string
	//}
	//meteringEntry := meteringEntryType{point, participantId, tenant}
	//
	//statement, _, _ := pgDialect.Insert(TABLE_METERINGPOINT).Rows(meteringEntry).ToSQL()
	//_, err = db.Exec(statement)
	//return err
}

func UpdateMeteringPoint(tenant, participantId, meterId string, meteringPoint *model.MeteringPoint) error {
	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	updateObject := *meteringPoint
	updateObject.State = nil
	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
		Set(updateObject).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participantId},
		}).
		ToSQL()

	fmt.Printf("Update Metering Point: %+v\n", statement)
	_, err = db.Exec(statement)

	return err
}

func RemoveMeteringPoint(dbOpen OpenDbXConnection, tenant, participantId, meterId string) error {
	db, err := dbOpen()
	if err != nil {
		return err
	}
	defer db.Close()

	statement, _, _ := goqu.Delete(TABLE_METERINGPOINT).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participantId},
			"status":            goqu.Op{"eq": "INVALID"},
		}).
		ToSQL()
	_, err = db.Exec(statement)

	return err
}

func ActivateMeteringPoints(tenant string, meterId []string) error {
	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
		Set(goqu.Record{"status": "ACTIVE"}).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
		}).
		ToSQL()
	_, err = db.Exec(statement)

	return err
}

func MeteringPointsSetStatus(dbOpen OpenDbXConnection, tenant string, status model.StatusType, meterId []string) error {
	db, err := dbOpen()
	if err != nil {
		return err
	}
	defer db.Close()

	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
		Set(goqu.Record{"status": status, "registeredSince": time.Now(), "modifiedAt": time.Now(), "modifiedBy": "EVU"}).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
		}).
		ToSQL()
	_, err = db.Exec(statement)

	return err
}

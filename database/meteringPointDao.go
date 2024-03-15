package database

import (
	"at.ourproject/vfeeg-backend/model"
	"errors"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
	"time"
)

const TABLE_METERINGPOINT = "base.meteringpoint"

//const TABLE_METERINGPOINT_STATE = "base.participant_meter_state"

type meteringEntryType struct {
	*model.MeteringPoint
	Participant_id string    `goqu:"skipupdate"`
	Tenant         string    `goqu:"skipupdate"`
	ActiveSince    time.Time `db:"activesince"`
	Active         int       `db:"active"`
	Flag           null.Int  `goqu:"skipupdate,omitempty"`
}

func createMeteringEntries(tenant, username, participantId string, points []*model.MeteringPoint, state *model.StatusType) []*meteringEntryType {
	meteringEntries := []*meteringEntryType{}
	now := time.Now().Local()
	for _, p := range points {
		if state != nil {
			p.Status = *state
		}
		p.ModifiedBy = null.StringFrom(username)
		p.ModifiedAt = time.Now()
		if p.RegisteredSince.IsZero() {
			p.RegisteredSince = now
		}
		if len(p.Status) == 0 {
			p.Status = model.NEW
		}
		meteringEntries = append(meteringEntries,
			&meteringEntryType{p, participantId, tenant,
				p.RegisteredSince, int(calcActive(p.Status)), null.IntFrom(int64(calcFlag(p.Status)))})
	}
	return meteringEntries
}

//func RegisterMeteringPoints(tx *sqlx.Tx, tenant, participantId string, point []*model.MeteringPoint) error {
//	state := model.NEW
//	return saveMeteringPoint(tx, createMeteringEntries(tenant, participantId, point, &state))
//}

func ImportMeteringPoints(tx *sqlx.Tx, tenant, username, participantId string, point []*model.MeteringPoint) error {
	return saveMeteringPoint(tx, createMeteringEntries(tenant, username, participantId, point, nil))
}

// saveMeteringPoint creates new metering point in the database.
// Accourding to the status of new metering point (ACTIVE when excel import; NEW otherwise) the flag of the meterstate will be adapted
func saveMeteringPoint(tx *sqlx.Tx, meteringEntry []*meteringEntryType) error {

	//m, err := FindMeteringById(OpenDbXConnection, meter)

	statement, _, _ := pgDialect.Insert(TABLE_METERINGPOINT).Rows(meteringEntry). /*.OnConflict(goqu.DoNothing())*/ ToSQL()
	log.Debugf("Register Meterings: %+v", statement)
	res, err := tx.Exec(statement)

	if err != nil {
		log.Errorf("Result: %v", res)
		return err
	}

	//type participantMeterState struct {
	//	Participant_id    string `db:"participant_id"`
	//	Tenant            string
	//	Metering_point_id string `db:"metering_point"`
	//	Changed_by        string
	//	ActiveSince       time.Time `db:"activesince"`
	//	Flag              null.Int
	//}
	//
	//stateEntries := []participantMeterState{}
	//for _, e := range meteringEntry {
	//	if e.State == nil {
	//		e.State = &model.MeterState{
	//			ActiveSince:   e.RegisteredSince,
	//			InactiveSince: time.Date(2999, 12, 31, 0, 0, 0, 0, time.Local),
	//		}
	//	}
	//
	//	stateEntries = append(stateEntries, participantMeterState{
	//		Participant_id:    e.Participant_id,
	//		Tenant:            e.Tenant,
	//		Metering_point_id: e.MeteringPoint.MeteringPoint,
	//		Changed_by:        e.ModifiedBy.String,
	//		ActiveSince:       e.RegisteredSince,
	//
	//		Flag: null.IntFrom(int64(calcFlag(e.Status))),
	//	})
	//}
	//
	//statement, _, _ = pgDialect.Insert(TABLE_METERINGPOINT_STATE).Rows(stateEntries).ToSQL()
	//_, err = tx.Exec(statement)
	//if err != nil {
	//	log.WithField("SQL", "INSERT").Debugf("Stmt: %s", statement)
	//}

	return err
}

func calcFlag(status model.StatusType) model.ProcessFlag {
	switch status {
	case model.ACTIVE:
		return model.F_IDLE
	default:
		return model.F_WAITING
	}
}

func calcActive(status model.StatusType) model.ProcessStatus {
	switch status {
	case model.INACTIVE:
		return model.P_INACTIVE
	default:
		return model.P_ACTIVE
	}
}
func RegisterMeteringPoint(db *sqlx.DB, tenant, username, participantId string, point *model.MeteringPoint) error {
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

	return saveMeteringPoint(tx, createMeteringEntries(tenant, username, participantId, []*model.MeteringPoint{point}, &point.Status))
}

func UpdateMeteringPoint(db *sqlx.DB, tenant, username, participantId, meterId string, meteringPoint *model.MeteringPoint) error {
	updateObject := *meteringPoint
	updateObject.State = nil
	updateObject.ModifiedBy = null.StringFrom(username)
	updateObject.ModifiedAt = time.Now()

	updateEntry := meteringEntryType{
		MeteringPoint: &updateObject, ActiveSince: meteringPoint.State.ActiveSince, Active: int(calcActive(updateObject.Status)),
	}
	statement, _, err := goqu.Update(TABLE_METERINGPOINT).
		Set(updateEntry).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participantId},
		}).
		ToSQL()
	if err != nil {
		return model.ErrUpdateMeter(err)
	}

	_, err = db.Exec(statement)
	if err != nil {
		log.WithField("SQL", "UPDATE").Errorf("Stmt: %v", statement)
		return model.ErrUpdateMeter(err)
	}
	return nil
}

func RemoveMeteringPoint(db *sqlx.DB, tenant, participantId, meterId string) error {
	statement, _, err := goqu.Delete(TABLE_METERINGPOINT).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participantId},
			"status":            goqu.Op{"eq": "INVALID"},
		}).
		ToSQL()
	if err != nil {
		return model.ErrRemoveMeteringPoint(err)
	}

	_, err = db.Exec(statement)
	if err != nil {
		return model.ErrRemoveMeteringPoint(err)
	}
	return err
}

//func ActivateMeteringPoints(tenant string, meterId []string) error {
//	db, err := GetDBXConnection()
//	if err != nil {
//		return err
//	}
//	defer db.Close()
//
//	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
//		Set(goqu.Record{"status": "ACTIVE"}).
//		Where(goqu.Ex{
//			"tenant":            goqu.Op{"eq": tenant},
//			"metering_point_id": goqu.Op{"eq": meterId},
//		}).
//		ToSQL()
//	_, err = db.Exec(statement)
//
//	return err
//}

func MeteringPointsSetStatus(db *sqlx.DB, tenant string, status model.StatusType, meterId []string) error {
	updateSet := struct {
		Status          model.StatusType `db:"status"`
		ModifiedAt      time.Time        `db:"modifiedAt"`
		ModifiedBy      string           `db:"modifiedBy"`
		RegisteredSince time.Time        `db:"registeredSince" goqu:"omitempty"`
		Inactivesince   time.Time        `db:"inactivesince" goqu:"omitempty"`
		Flag            model.ProcessFlag
		Active          model.ProcessStatus
	}{
		Status:     status,
		ModifiedAt: time.Now(),
		ModifiedBy: "EVU",
		Flag:       calcFlag(status),
		Active:     calcActive(status),
	}

	/**
	Consider in case reactivating the metering point for the same participant, the inactivesince time must be adjusted to the very end time period.
	The activesince time must be left alone as it controls the visibility of the time period in the user client.
	Therefore, the activesince time is only set at creation time.

	IMPROVE: Check the context of the meteringpoint while activating.
	*/
	flag := model.F_WAITING
	if status == model.ACTIVE {
		t := time.Date(2999, 12, 31, 23, 59, 59, 0, time.UTC)
		updateSet.Inactivesince = t
	} else if status == model.NEW || status == model.PENDING {
		t := time.Now()
		updateSet.RegisteredSince = t
	}

	statement, _, err := goqu.Update(TABLE_METERINGPOINT).
		Set(updateSet).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"flag":              goqu.Op{"eq": flag},
		}).ToSQL()
	if err != nil {
		return model.ErrStatusMeter(err)
	}
	_, err = db.Exec(statement)
	if err != nil {
		log.WithField("SQL", "UPDATE").Errorf("Stmt: %v", statement)
		return model.ErrStatusMeter(err)
	}
	return nil
}

func MeteringPointRevoke(db *sqlx.DB, tenant, meterId string, status model.StatusType, consentEnd time.Time) error {

	log.Debugf("Revoke Meter: %s at %v\n", meterId, consentEnd)

	participant, err := FindParticipantByMeteringPoint(db, tenant, meterId)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		err := tx.Rollback()
		if err != nil {
			//log.Error(err)
		}
	}()

	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
		Set(goqu.Record{
			"status":          status,
			"registeredSince": time.Now(),
			"modifiedAt":      time.Now(),
			"modifiedBy":      "EVU",
			"active":          calcActive(status),
			"inactivesince":   consentEnd.Local()}).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participant.Id.String()},
		}).
		ToSQL()
	_, err = tx.Exec(statement)

	if err != nil {
		return err
	}

	//statement, _, _ = goqu.Update(TABLE_METERINGPOINT_STATE).
	//	Set(goqu.Record{"inactivesince": consentEnd, "changed_at": time.Now(), "changed_by": "EVU", "active": 0}).
	//	Where(goqu.Ex{
	//		"tenant":         goqu.Op{"eq": tenant},
	//		"participant_id": goqu.Op{"eq": participant.Id.String()},
	//		"metering_point": goqu.Op{"eq": meterId},
	//	}).
	//	ToSQL()
	//_, err = tx.Exec(statement)
	//
	//if err != nil {
	//	log.WithField("SQL", "SELECT").Errorf("Stmt: %v", statement)
	//	return err
	//}
	//fmt.Printf("Finish Revoke Meter: %s at %v on participant %v\n", meterId, consentEnd, participant)
	return tx.Commit()
}

func MeteringPointSetInactive(dbOpen OpenDbXConnection, tenant, meterId string, status model.StatusType, consentEnd time.Time) error {

	db, err := dbOpen()
	if err != nil {
		return err
	}
	defer db.Close()

	participant, err := FindParticipantByMeteringPoint(db, tenant, meterId)
	if err != nil {
		return err
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		err := tx.Rollback()
		if err != nil {
			//log.Error(err)
		}
	}()

	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
		Set(goqu.Record{"status": status, "registeredSince": time.Now(), "modifiedAt": time.Now(), "modifiedBy": "EVU", "inactivesince": consentEnd}).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participant.Id.String()},
		}).
		ToSQL()
	_, err = tx.Exec(statement)

	if err != nil {
		return err
	}

	//statement, _, _ = goqu.Update(TABLE_METERINGPOINT_STATE).
	//	Set(goqu.Record{"inactivesince": consentEnd, "changed_at": time.Now(), "changed_by": "EVU"}).
	//	Where(goqu.Ex{
	//		"tenant":         goqu.Op{"eq": tenant},
	//		"participant_id": goqu.Op{"eq": participant.Id.String()},
	//		"metering_point": goqu.Op{"eq": meterId},
	//	}).
	//	ToSQL()
	//_, err = tx.Exec(statement)
	//
	//if err != nil {
	//	log.WithField("SQL", "SELECT").Errorf("Stmt: %v", statement)
	//	return err
	//}
	//fmt.Printf("Finish Revoke Meter: %s at %v on participant %v\n", meterId, consentEnd, participant)
	return tx.Commit()
}

func FindGridOperatorId(dbOpen OpenDbXConnection, meterId string) (string, error) {
	db, err := dbOpen()
	if err != nil {
		return "", err
	}
	defer db.Close()

	gridOperatorId := ""
	stmt, _, err := pgDialect.From(TABLE_METERINGPOINT).Select("grid_operator_id").Where(goqu.C("metering_point_id").Eq(meterId)).ToSQL()
	if err != nil {
		return "", err
	}
	err = db.QueryRow(stmt).Scan(&gridOperatorId)
	if err != nil {
		return "", err
	}

	return gridOperatorId, nil
}

func FindInactiveMeteringById(db *sqlx.DB, meterId string) ([]*model.MeteringPoint, error) {
	return findMeteringByIdAndState(db, []string{meterId}, model.P_INACTIVE)
}

func GetMeteringByIds(db *sqlx.DB, meterIds []string) ([]*model.MeteringPoint, error) {
	return findMeteringByIdAndState(db, meterIds, model.P_ACTIVE)
}

func FindMeteringById(tx *sqlx.DB, meterId string) (*model.MeteringPoint, error) {
	m, err := findMeteringByIdAndState(tx, []string{meterId}, model.P_ACTIVE)
	if err != nil {
		return nil, err
	}
	if len(m) == 1 {
		return m[0], nil
	}
	return nil, model.ErrFindMeter(errors.New("More as one active Meteringpoint was found"))
}

func findMeteringByIdAndState(db *sqlx.DB, meterIds []string, active model.ProcessStatus) ([]*model.MeteringPoint, error) {
	var m []*model.MeteringPoint

	stateStmt := pgDialect.From(TABLE_METERINGPOINT).
		Select(
			goqu.C("activesince"),
			goqu.C("inactivesince"),
			goqu.C("active"),
			goqu.C("metering_point_id").As("mid"))
	stmt, _, err := pgDialect.From(TABLE_METERINGPOINT, stateStmt.As("state")).Select(&model.MeteringPoint{}).
		Where(
			goqu.C("metering_point_id").In(meterIds),
			goqu.I("state.active").Eq(active),
			goqu.C("mid").Eq(goqu.C("metering_point_id")),
		).ToSQL()

	if err != nil {
		return nil, model.ErrFindMeter(err)
	}

	fmt.Printf("SQL-Stmt: %s\n", stmt)
	err = db.Select(&m, stmt)
	if err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return nil, model.ErrFindMeter(err)
	}
	return m, nil
}

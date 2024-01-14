package database

import (
	"at.ourproject/vfeeg-backend/model"
	//"at.ourproject/vfeeg-backend/util"
	"fmt"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
	"time"
)

const TABLE_METERINGPOINT = "base.meteringpoint"
const TABLE_METERINGPOINT_STATE = "base.participant_meter_state"

type meteringEntryType struct {
	*model.MeteringPoint
	Participant_id string
	Tenant         string
}

func createMeteringEntries(tenant, username, participantId string, points []*model.MeteringPoint, state *model.StatusType) []*meteringEntryType {
	meteringEntries := []*meteringEntryType{}
	for _, p := range points {
		if state != nil {
			p.Status = *state
		}
		p.ModifiedBy = null.StringFrom(username)
		p.ModifiedAt = time.Now()
		meteringEntries = append(meteringEntries, &meteringEntryType{p, participantId, tenant})
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

func saveMeteringPoint(tx *sqlx.Tx, meteringEntry []*meteringEntryType) error {
	//meterToInsert := []*meteringEntryType{}
	//for _, e := range meteringEntry {
	//	c := meteringEntryType{}
	//	c = copy
	//	c.State = nil
	//	meterToInsert = append(meterToInsert, &c)
	//}
	statement, _, _ := pgDialect.Insert(TABLE_METERINGPOINT).Rows(meteringEntry).ToSQL()
	//statement, _, _ := goqu.Insert(TABLE_METERINGPOINT).Rows(meterToInsert).ToSQL()
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
		if e.State == nil {
			e.State = &model.MeterState{
				ActiveSince:   e.RegisteredSince,
				InactiveSince: time.Date(2999, 12, 31, 0, 0, 0, 0, time.Local),
			}
		}

		stateEntries = append(stateEntries, participantMeterState{
			Participant_id:    e.Participant_id,
			Tenant:            e.Tenant,
			Metering_point_id: e.MeteringPoint.MeteringPoint,
			Changed_by:        e.ModifiedBy.String,
			ActiveSince:       e.RegisteredSince,
		})
	}

	statement, _, _ = pgDialect.Insert(TABLE_METERINGPOINT_STATE).Rows(stateEntries).ToSQL()
	log.Debugf("Register Meterings: %+v", statement)
	_, err = tx.Exec(statement)

	return err
}

func RegisterMeteringPoint(openDb OpenDbXConnection, tenant, username, participantId string, point *model.MeteringPoint) error {
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

	return saveMeteringPoint(tx, createMeteringEntries(tenant, username, participantId, []*model.MeteringPoint{point}, &point.Status))
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

func UpdateMeteringPoint(tenant, username, participantId, meterId string, meteringPoint *model.MeteringPoint) error {
	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	updateObject := *meteringPoint
	updateObject.State = nil
	updateObject.ModifiedBy = null.StringFrom(username)
	updateObject.ModifiedAt = time.Now()

	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
		Set(updateObject).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participantId},
		}).
		ToSQL()

	fmt.Printf("Update Metering Point: %+v\n", meteringPoint.MeteringPoint)
	_, err = db.Exec(statement)

	if err != nil {
		return err
	}

	type participantMeterState struct {
		Changed_by  string
		Changed_at  time.Time
		ActiveSince time.Time `db:"activesince"`
	}

	particpantState := &participantMeterState{
		Changed_by: username,
		Changed_at: time.Now(),
		//ActiveSince: time.Date(
		//	meteringPoint.State.ActiveSince.Year(),
		//	meteringPoint.State.ActiveSince.Month(),
		//	meteringPoint.State.ActiveSince.Day(),
		//	0, 0, 0, 0, time.Local,
		//),
		ActiveSince: meteringPoint.State.ActiveSince,
	}

	statement, _, _ = goqu.Update(TABLE_METERINGPOINT_STATE).
		Set(particpantState).
		Where(goqu.Ex{
			"tenant":         goqu.Op{"eq": tenant},
			"metering_point": goqu.Op{"eq": meterId},
			"participant_id": goqu.Op{"eq": participantId},
		}).
		ToSQL()

	fmt.Printf("Update Metering Point State: %+v\n", statement)
	_, err = db.Exec(statement)

	if err != nil {
		return err
	}
	//meteringPoint.State.ActiveSince = particpantState.ActiveSince
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

func FindMeteringById(dbOpen OpenDbXConnection, meterId string) (*model.MeteringPoint, error) {
	db, err := dbOpen()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	m := model.MeteringPoint{}
	stmt, _, err := pgDialect.From(TABLE_METERINGPOINT).Select(&m).Where(goqu.C("metering_point_id").Eq(meterId)).ToSQL()
	if err != nil {
		return nil, err
	}
	err = db.Get(&m, stmt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

//func MeteringPointPerformAnswerMsg(dbOpen OpenDbXConnection, tenant string, meterId []string) error {
//	eeg, err := GetEeg(tenant)
//	if err != nil {
//		return err
//	}
//
//	db, err := dbOpen()
//	if err != nil {
//		return err
//	}
//	defer func() {
//		err = db.Close()
//		if err != nil {
//			log.Errorf("Error Close Database: %v", err)
//		}
//	}()
//
//	tx, err := db.Beginx()
//	if err != nil {
//		return err
//	}
//	defer func() {
//		err := tx.Rollback()
//		if err != nil {
//			log.Errorf("Rollback Error: %v", err)
//		}
//	}()
//
//	for _, mid := range meterId {
//		participant, err := FindParticipantByMeteringPoint(tx, tenant, mid)
//		if err != nil {
//			return err
//		}
//		if participant.Contact.Email.Valid {
//			if err = parser.SendActivationMailFromTemplate(services.SendMail,
//				tenant, "Aktivierung im Serviceportal", eeg, participant); err != nil {
//				log.Errorf("Error Sending Mail: %+v", err.Error())
//			}
//		}
//	}
//	return tx.Commit()
//}

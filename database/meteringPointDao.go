package database

import (
	"at.ourproject/vfeeg-backend/model"
	"errors"

	//"at.ourproject/vfeeg-backend/util"
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
	ActiveSince    time.Time `goqu:"skipupdate"`
	Active         int       `goqu:"skipupdate"`
	Flag           null.Int  `goqu:"skipupdate"`
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
			&meteringEntryType{p, participantId, tenant, p.RegisteredSince, calcActive(p.Status), null.IntFrom(int64(calcFlag(p.Status)))})
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

func calcFlag(status model.StatusType) int {
	switch status {
	case model.ACTIVE:
		return 0
	default:
		return 1
	}
}

func calcActive(status model.StatusType) int {
	switch status {
	case model.INACTIVE:
		return 0
	default:
		return 1
	}
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
}

func UpdateMeteringPoint(openDb OpenDbXConnection, tenant, username, participantId, meterId string, meteringPoint *model.MeteringPoint) error {
	db, err := openDb()
	if err != nil {
		return err
	}
	defer db.Close()

	updateObject := *meteringPoint
	updateObject.State = nil
	updateObject.ModifiedBy = null.StringFrom(username)
	updateObject.ModifiedAt = time.Now()

	updateEntry := meteringEntryType{
		MeteringPoint: &updateObject, ActiveSince: meteringPoint.State.ActiveSince, Active: calcActive(updateObject.Status),
	}
	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
		Set(updateEntry).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participantId},
		}).
		ToSQL()

	_, err = db.Exec(statement)
	if err != nil {
		log.WithField("SQL", "UPDATE").Errorf("Stmt: %v", statement)
		return err
	}
	return nil
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

func MeteringPointRevoke(db *sqlx.DB, tenant, meterId string, status model.StatusType, consentEnd time.Time) error {

	fmt.Printf("Revoke Meter: %s at %v\n", meterId, consentEnd)

	//db, err := dbOpen()
	//if err != nil {
	//	return err
	//}
	//defer db.Close()

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

	//var participantId string
	//stmt, _, _ := pgDialect.From(TABLE_METERINGPOINT_STATE).Select("participant_id").
	//	Where(
	//		goqu.C("metering_point").Eq(meterId),
	//		goqu.C("tenant").Eq(tenant),
	//		goqu.C("inactivesince").Gt(consentEnd),
	//		goqu.C("activesince").Lt(consentEnd)).ToSQL()
	//err = tx.Get(&participantId, stmt)

	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
		Set(goqu.Record{
			"status":          status,
			"registeredSince": time.Now(),
			"modifiedAt":      time.Now(),
			"modifiedBy":      "EVU",
			"active":          calcActive(status),
			"inactivesince":   consentEnd}).
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

func FindInactiveMeteringById(dbOpen OpenDbXConnection, meterId string) ([]*model.MeteringPoint, error) {
	return findMeteringByIdAndState(dbOpen, meterId, 0)
}

func FindMeteringById(dbOpen OpenDbXConnection, meterId string) (*model.MeteringPoint, error) {
	m, err := findMeteringByIdAndState(dbOpen, meterId, 1)
	if err != nil {
		return nil, err
	}
	if len(m) == 1 {
		return m[0], nil
	}
	return nil, errors.New("More as one active Meteringpoint was found")
}

func findMeteringByIdAndState(dbOpen OpenDbXConnection, meterId string, active int) ([]*model.MeteringPoint, error) {
	db, err := dbOpen()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var m []*model.MeteringPoint

	stateStmt := pgDialect.From(TABLE_METERINGPOINT).
		Select(
			goqu.C("activesince"),
			goqu.C("inactivesince"),
			goqu.C("active"),
			goqu.C("metering_point_id").As("mid"))
	stmt, _, err := pgDialect.From(TABLE_METERINGPOINT, stateStmt.As("state")).Select(&model.MeteringPoint{}).
		Where(
			goqu.C("metering_point_id").Eq(meterId),
			goqu.I("state.active").Eq(active),
			goqu.C("mid").Eq(goqu.C("metering_point_id")),
		).ToSQL()

	//stmt, _, err := pgDialect.From(TABLE_METERINGPOINT).Select(&m).
	//	InnerJoin(goqu.T("participant_meter_state").Schema("base"),
	//		goqu.On(
	//			goqu.Ex{"base.meteringpoint.participant_id": goqu.I("participant_meter_state.participant_id")},
	//			goqu.Ex{"base.meteringpoint.metering_point_id": goqu.I("participant_meter_state.metering_point")})).
	//	Where(goqu.C("metering_point_id").Eq(meterId)).ToSQL()
	if err != nil {
		return nil, err
	}
	err = db.Select(&m, stmt)
	if err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return nil, err
	}
	return m, nil
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

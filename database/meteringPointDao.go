package database

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jjeffery/civil"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"gopkg.in/guregu/null.v4"
	"time"
)

const TABLE_METERINGPOINT = "base.meteringpoint"
const TABLE_PARTITION_FACT = "base.metering_partition_factor"
const TABLE_PARTITION_FACT_VIEW = "base.activemeteringpartition"

//const TABLE_METERINGPOINT_STATE = "base.participant_meter_state"

type meteringEntryType struct {
	*model.MeteringPoint
	Participant_id string     `goqu:"skipupdate"`
	Tenant         string     `goqu:"skipupdate"`
	ActiveSince    civil.Date `db:"activesince"`
	Active         int        `db:"active"`
	Flag           null.Int   `goqu:"skipupdate,omitempty"`
}

type partitionFactorRecord struct {
	MeteringPoint  string `db:"metering_point_id"`
	Participant_id string `goqu:"skipupdate"`
	Tenant         string `goqu:"skipupdate"`
	PartFact       int    `db:"partFact"`
	CreatedBy      string `db:"createdBy"`
}

func createMeteringEntries(tenant, username, participantId string, points []*model.MeteringPoint, state *model.StatusType) ([]*meteringEntryType, []*partitionFactorRecord) {
	var partFactEntries []*partitionFactorRecord
	var meteringEntries []*meteringEntryType
	for _, p := range points {
		if state != nil {
			p.Status = *state
		}
		p.ModifiedBy = null.StringFrom(username)
		p.ModifiedAt = civil.Now()
		if p.RegisteredSince.IsZero() {
			p.RegisteredSince = civil.Today()
		}
		if len(p.Status) == 0 {
			p.Status = model.NEW
		}
		meteringEntries = append(meteringEntries,
			&meteringEntryType{p, participantId, tenant,
				p.RegisteredSince, int(calcActive(p.Status)), null.IntFrom(int64(calcFlag(p.Status)))})

		partFactEntries = append(partFactEntries, &partitionFactorRecord{
			MeteringPoint:  p.MeteringPoint,
			Participant_id: participantId,
			Tenant:         tenant,
			PartFact:       p.PartFact,
			CreatedBy:      username,
		})
	}
	return meteringEntries, partFactEntries
}

//func RegisterMeteringPoints(tx *sqlx.Tx, tenant, participantId string, point []*model.MeteringPoint) error {
//	state := model.NEW
//	return saveMeteringPoint(tx, createMeteringEntries(tenant, participantId, point, &state))
//}

func ImportMeteringPoints(tx *sqlx.Tx, tenant, username, participantId string, point []*model.MeteringPoint) error {
	meteringEntries, partFactEntries := createMeteringEntries(tenant, username, participantId, point, nil)
	return saveMeteringPoint(tx, meteringEntries, partFactEntries)
}

// saveMeteringPoint creates new metering point in the database.
// Accourding to the status of new metering point (ACTIVE when excel import; NEW otherwise) the flag of the meterstate will be adapted
func saveMeteringPoint(tx *sqlx.Tx, meteringEntry []*meteringEntryType, partFactEntries []*partitionFactorRecord) error {
	if len(meteringEntry) == 0 || len(partFactEntries) == 0 {
		log.Warn("Save Meteringpoints with empty list of partFact or partFact entries")
		return nil
	}
	statement, _, err := pgDialect.Insert(TABLE_METERINGPOINT).Rows(meteringEntry). /*.OnConflict(goqu.DoNothing())*/ ToSQL()
	if err != nil {
		return model.ErrSaveMeteringPoint(err)
	}
	res, err := tx.Exec(statement)

	if err != nil {
		log.Errorf("Stmt: %s - %v", statement, res)
		return model.ErrSaveMeteringPoint(err)
	}

	statement, _, err = pgDialect.Insert(TABLE_PARTITION_FACT).Rows(partFactEntries). /*.OnConflict(goqu.DoNothing())*/ ToSQL()
	if err != nil {
		return model.ErrSaveMeteringPoint(err)
	}
	res, err = tx.Exec(statement)

	if err != nil {
		log.Errorf("Stmt: %s - %v", statement, res)
		return model.ErrSaveMeteringPoint(err)
	}

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

func calcFlagPtr(status *model.StatusType) *model.ProcessFlag {
	if status == nil {
		return nil
	}
	flag := calcFlag(*status)
	return &flag
}

func calcActive(status model.StatusType) model.ProcessStatus {
	switch status {
	case model.INACTIVE:
		return model.P_INACTIVE
	default:
		return model.P_ACTIVE
	}
}

func calcActivePtr(status *model.StatusType) *model.ProcessStatus {
	if status == nil {
		return nil
	}
	state := calcActive(*status)
	return &state
}

func RegisterMeteringPoint(db *sqlx.DB, tenant, username, participantId string, point *model.MeteringPoint) error {
	tx, err := db.Beginx()
	if err != nil {
		log.WithError(err).Error("Not able to open a transaction.")
		return err
	}

	defer func() {
		switch err {
		case nil:
			err = tx.Commit()
		default:
			err = tx.Rollback()
		}
	}()

	meteringEntries, partFactEntries := createMeteringEntries(tenant, username, participantId, []*model.MeteringPoint{point}, &point.Status)
	err = saveMeteringPoint(tx, meteringEntries, partFactEntries)
	return err
}

func MoveMeteringPoint(db *sqlx.DB, tenant, username, sParticipantId, dParticipantId, meterId string) error {
	tx, err := db.Beginx()
	if err != nil {
		log.Errorf("Not able to open a transaction. %s", err.Error())
		return model.ErrOpenTx(err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		} else {
			_ = tx.Commit()
		}
	}()

	//var partFact partitionFactorRecord
	//
	//statement, _, err := goqu.Select(&partFact).From(TABLE_PARTITION_FACT).
	//	Where(goqu.Ex{
	//		"tenant":            goqu.Op{"eq": tenant},
	//		"metering_point_id": goqu.Op{"eq": meterId},
	//		"participant_id":    goqu.Op{"eq": sParticipantId},
	//	}).
	//	ToSQL()
	//if err != nil {
	//	return model.ErrUpdateMeter(err)
	//}
	//err = tx.Get(&partFact, statement)
	//if err != nil {
	//	log.WithField("SQL", "SELECT").Errorf("Stmt: %v", statement)
	//	return model.ErrUpdateMeter(err)
	//}

	//statement, _, err = goqu.Delete(TABLE_PARTITION_FACT).
	//	Where(goqu.Ex{
	//		"tenant":            goqu.Op{"eq": tenant},
	//		"metering_point_id": goqu.Op{"eq": meterId},
	//		"participant_id":    goqu.Op{"eq": sParticipantId},
	//	}).
	//	ToSQL()
	//if err != nil {
	//	return model.ErrUpdateMeter(err)
	//}
	//_, err = tx.Exec(statement)
	//if err != nil {
	//	log.WithField("SQL", "DELETE").Errorf("Stmt: %v", statement)
	//	return model.ErrUpdateMeter(err)
	//}

	statement, _, err := goqu.Update(TABLE_METERINGPOINT).
		Set(goqu.Record{"participant_id": dParticipantId}).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": sParticipantId},
		}).
		ToSQL()
	if err != nil {
		return model.ErrUpdateMeter(err)
	}
	_, err = tx.Exec(statement)
	if err != nil {
		log.WithField("SQL", "UPDATE").Errorf("Stmt: %v", statement)
		return model.ErrUpdateMeter(err)
	}

	//partFact.Participant_id = dParticipantId
	//statement, _, err = goqu.Insert(TABLE_PARTITION_FACT).Rows(&partFact).
	//	ToSQL()
	//_, err = tx.Exec(statement)
	//if err != nil {
	//	log.WithField("SQL", "INSERT").Errorf("Stmt: %v", statement)
	//	return model.ErrUpdateMeter(err)
	//}

	return nil
}

func UpdateMeteringPoint(db *sqlx.DB, tenant, username, participantId, meterId string, meteringPoint *model.MeteringPoint) error {
	updateObject := *meteringPoint
	updateObject.State = nil
	updateObject.ModifiedBy = null.StringFrom(username)
	updateObject.ModifiedAt = civil.Now()

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

func MeteringPointsSetStatus(db *sqlx.DB, tenant string, status model.StatusType, statusCode int16, meterId []string, activeSince *civil.Date, consentId *string) error {
	updateSet := struct {
		Status          model.StatusType   `db:"status"`
		StatusCode      int16              `db:"statusCode" goqu:"omitempty"`
		ModifiedAt      civil.NullDateTime `db:"modifiedAt" goqu:"defaultifempty""`
		ModifiedBy      string             `db:"modifiedBy"`
		RegisteredSince civil.NullDate     `db:"registeredSince" goqu:"omitempty"`
		Inactivesince   civil.NullDate     `db:"inactivesince" goqu:"omitempty"`
		Activesince     civil.NullDate     `db:"activesince" goqu:"omitempty"`
		ConsentId       *string            `db:"consent_id" goqu:"omitnil"`
		Flag            model.ProcessFlag
		Active          model.ProcessStatus
	}{
		Status:     status,
		StatusCode: statusCode,
		ModifiedBy: "EVU",
		ConsentId:  consentId,
		Flag:       calcFlag(status),
		Active:     calcActive(status),
	}

	/**
	Consider in case reactivating the metering point for the same participant, the inactivesince time must be adjusted to the very end time period.
	The activesince time must be left alone as it controls the visibility of the time period in the user client.
	Therefore, the activesince time is only set at creation time.

	IMPROVE: Check the context of the meteringpoint while activating.
	*/
	log.WithField("tenant", tenant).Infof("Change Status. Meters: %v activeSince: %v status: %v", meterId, activeSince, status)
	flag := model.F_WAITING
	if status == model.ACTIVE {
		t := civil.DateFor(2999, 12, 31)
		updateSet.Inactivesince = civil.NullDateFrom(&t)
		updateSet.Activesince = civil.NullDateFrom(activeSince)
	} else if status == model.NEW || status == model.PENDING {
		t := civil.Today()
		updateSet.RegisteredSince = civil.NullDateFrom(&t)
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

	result, err := db.Exec(statement)
	if err != nil {
		log.WithField("SQL", "UPDATE").Errorf("Stmt: %v", statement)
		return model.ErrStatusMeter(err)
	}
	if rows, err := result.RowsAffected(); err != nil || rows == 0 {
		log.WithField("SQL", "UPDATE").Infof("No Rows Affected. Stmt: %v", statement)
	}
	return nil
}

func MeteringPointRevoke(db *sqlx.DB, tenant, meterId string, status model.StatusType, consentEnd civil.Date) error {

	log.Debugf("Revoke Meter: %s at %v\n", meterId, consentEnd)

	participant, err := FindParticipantByMeteringPoint(db, tenant, meterId)
	if err != nil {
		return model.ErrFindParticipant(err)
	}

	tx, err := db.Beginx()
	if err != nil {
		return model.ErrOpenTx(err)
	}
	defer func() {
		err := tx.Rollback()
		if err != nil {
			//log.Error(err)
		}
	}()

	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
		Set(goqu.Record{
			"status":        status,
			"modifiedAt":    civil.Now(),
			"modifiedBy":    "EVU",
			"active":        calcActive(status),
			"flag":          calcFlag(status),
			"inactivesince": consentEnd}).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participant.Id.String()},
		}).
		ToSQL()
	_, err = tx.Exec(statement)

	if err != nil {
		return model.ErrUpdateMeter(err)
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

func MeteringPointRevokeByConsentId(db *sqlx.DB, consentId *string, meterId string, consentEnd civil.Date) (*string, error) {
	execDB := goqu.New("postgres", db)

	tx, err := execDB.Begin()
	if err != nil {
		return nil, model.ErrOpenTx(err)
	}
	defer func() {
		switch err {
		case nil:
			_ = tx.Commit()
		default:
			_ = tx.Rollback()
		}
	}()

	var whereClause exp.Expression
	if consentId != nil {
		whereClause = goqu.And(
			goqu.C("metering_point_id").Eq(meterId),
			goqu.Or(
				goqu.C("consent_id").Eq(consentId),
				goqu.And(
					goqu.C("consent_id").Is(nil),
					goqu.C("active").Eq(1))))
	} else {
		whereClause = goqu.And(
			goqu.C("metering_point_id").Eq(meterId),
			goqu.C("active").Eq(1))
	}

	update := tx.Update(TABLE_METERINGPOINT).
		Set(goqu.Record{
			"status":        model.INACTIVE,
			"modifiedAt":    civil.Now(),
			"modifiedBy":    "EVU",
			"active":        calcActive(model.INACTIVE),
			"flag":          calcFlag(model.INACTIVE),
			"inactivesince": consentEnd}).
		Where(whereClause, goqu.ExOr{}).
		Returning("tenant").
		Executor()

	var tenants []string
	if err := update.ScanVals(&tenants); err != nil {
		return nil, model.ErrUpdateMeter(err)
	}
	if len(tenants) != 1 {
		log.Warnf("Meteringpoint %s is not unique", meterId)
		return nil, model.ErrUpdateMeter(errors.New(fmt.Sprintf("Meteringpoint %s is not unique", meterId)))
	}
	return &tenants[0], nil
}

func MeteringPointChangePartFactor(db *sqlx.DB, tenant string, meters []model.Meter) error {

	log.Debug("Change Partition Factor")

	tx, err := db.Beginx()
	if err != nil {
		return model.ErrOpenTx(err)
	}
	defer func() {
		err := tx.Rollback()
		if err != nil {
			//log.Error(err)
		}
	}()

	//insert into base.metering_partition_factor (metering_point_id, participant_id, "partFact", tenant, "createdBy")
	//						select metering_point_id, participant_id, 10 as "partFact", tenant, 'system' as "createdBy" from base.meteringpoint where metering_point_id in ('AT0030000000000000000000000060061', 'AT0030000000000000000000000433950') and tenant = 'CC100392';

	//inMeters := []string{"AT111111111111111", "AT222222222222"}

	metersJson, err := json.Marshal(meters)
	if err != nil {
		return model.ErrUpdateMeter(err)
	}

	withClause := goqu.L(
		fmt.Sprintf(`(SELECT * FROM json_to_recordset('%s') AS cols("meteringPoint" TEXT, direction TEXT, activation BIGINT, "partFact" INT))`, metersJson))
	insertQuery := goqu.From(TABLE_METERINGPOINT, withClause.As("ma")).
		Select(
			goqu.C("metering_point_id"),
			goqu.C("participant_id"),
			goqu.C("tenant"),
			goqu.C("partFact"),
			goqu.V("system").As("createdBy"),
		).Where(goqu.C("metering_point_id").Eq(goqu.I("ma.meteringPoint")), goqu.C("tenant").Eq(tenant))
	stmt, _, err := goqu.Insert(TABLE_PARTITION_FACT).
		Cols("metering_point_id", "participant_id", "tenant", "partFact", "createdBy").
		FromQuery(insertQuery).ToSQL()

	fmt.Printf("Stmt Insert PactChange: %s - %v\n", stmt, err)
	_, err = tx.Exec(stmt)
	if err != nil {
		return model.ErrUpdateMeter(err)
	}
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

func FindInactiveMeteringById(db *sqlx.DB, tenant string, meterId string) ([]*model.MeteringPoint, error) {
	mode := model.P_INACTIVE
	return findMeteringByIdAndState(db, tenant, []string{meterId}, &mode)
}

func GetMeteringByIds(db *sqlx.DB, tenant string, meterIds []string) ([]*model.MeteringPoint, error) {
	mode := model.P_ACTIVE
	return findMeteringByIdAndState(db, tenant, meterIds, &mode)
}

func FindAllMeteringByTenant(tx *sqlx.DB, tenant, participantId string, meterIds []string) ([]*model.MeteringPoint, error) {
	var m []*model.MeteringPoint

	stateStmt := pgDialect.From(TABLE_METERINGPOINT).
		Select(
			goqu.C("activesince"),
			goqu.C("inactivesince"),
			goqu.C("active"),
			goqu.C("metering_point_id").As("mid"),
			goqu.C("tenant").As("mid_tenant"))

	partFactStmt := pgDialect.From(TABLE_PARTITION_FACT_VIEW).
		Select(
			goqu.C("partFact"),
			goqu.C("metering_point_id").As("mpfmid"),
			goqu.C("participant_id").As("mpfpid"),
			goqu.C("tenant").As("mpf_tenant"))

	stmt, _, err := pgDialect.From(TABLE_METERINGPOINT, stateStmt.As("state"), partFactStmt.As("mpfpF")).Select(&model.MeteringPoint{}).
		Where(
			goqu.C("metering_point_id").In(meterIds),
			goqu.C("tenant").Eq(tenant),
			goqu.C("mid_tenant").Eq(tenant),
			goqu.C("mpf_tenant").Eq(tenant),
			goqu.C("participant_id").Eq(participantId),
			goqu.C("mid").Eq(goqu.C("metering_point_id")),
			goqu.C("mpfmid").Eq(goqu.C("metering_point_id")),
			goqu.C("mpfpid").Eq(goqu.C("participant_id")),
		).ToSQL()

	if err != nil {
		return nil, model.ErrFindMeter(err)
	}
	log.WithField("SQL", "SELECT").Infof("Stmt: %s", stmt)
	err = tx.Select(&m, stmt)
	if err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return nil, model.ErrFindMeter(err)
	}
	return m, nil
}

func FindMeteringById(tx *sqlx.DB, tenant string, meterId string) (*model.MeteringPoint, error) {
	mode := model.P_ACTIVE
	m, err := findMeteringByIdAndState(tx, tenant, []string{meterId}, &mode)
	if err != nil {
		return nil, err
	}
	if len(m) == 1 {
		return m[0], nil
	}
	return nil, model.ErrFindMeter(errors.New("More as one active Meteringpoint was found"))
}

func findMeteringByIdAndState(db *sqlx.DB, tenant string, meterIds []string, active *model.ProcessStatus) ([]*model.MeteringPoint, error) {
	var m []*model.MeteringPoint

	stateStmt := pgDialect.From(TABLE_METERINGPOINT).
		Select(
			goqu.C("activesince"),
			goqu.C("inactivesince"),
			goqu.C("active"),
			goqu.C("metering_point_id").As("mid")).
		Where(goqu.C("tenant").Eq(tenant))

	partFactStmt := pgDialect.From(TABLE_PARTITION_FACT_VIEW).
		Select(
			goqu.C("partFact"),
			goqu.C("metering_point_id").As("mpfmid"),
			goqu.C("participant_id").As("mpfpid")).
		Where(goqu.C("tenant").Eq(tenant))

	//var activClause exp.BooleanExpression
	//activClause := goqu.V(true).Eq(true)
	//if active != nil {
	//	activClause = goqu.I("state.active").Eq(*active)
	//}

	whereClause := goqu.Ex{
		"metering_point_id": goqu.Op{"in": meterIds},
		"tenant":            tenant,
		"mid":               goqu.C("metering_point_id"),
		"mpfmid":            goqu.C("metering_point_id"),
		"mpfpid":            goqu.C("participant_id"),
	}
	if active != nil {
		whereClause["state.active"] = *active
	}

	stmt, _, err := pgDialect.From(TABLE_METERINGPOINT, stateStmt.As("state"), partFactStmt.As("mpfpF")).Select(&model.MeteringPoint{}).
		Where(whereClause). //goqu.C("metering_point_id").In(meterIds),
		////goqu.I("state.active").Eq(active),
		//goqu.C("tenant").Eq(tenant),
		//goqu.C("mid").Eq(goqu.C("metering_point_id")),
		//goqu.C("mpfmid").Eq(goqu.C("metering_point_id")),
		//goqu.C("mpfpid").Eq(goqu.C("participant_id")),
		//activClause,
		ToSQL()

	if err != nil {
		log.WithField("tenant", tenant).Errorf("Stmt: %s", stmt)
		return nil, model.ErrFindMeter(err)
	}

	err = db.Select(&m, stmt)
	if err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return nil, model.ErrFindMeter(err)
	}
	return m, nil
}

func UpdateActiveMeteringPoints(tx *sqlx.Tx, tenant string, ml []model.Meter) error {
	tml := model.StandardizeMeteringPointList(ml)

	status := model.ACTIVE
	meteringPoints := model.ConvertToDbMeterList(tml)
	//partFacts := model.ConvertToDbMeterPartFactList(tml)
	for _, m := range meteringPoints {
		m.Status = &status
		m.Active = calcActivePtr(m.Status)
		m.Flag = calcFlagPtr(m.Status)
		sql, _, err := goqu.Update(TABLE_METERINGPOINT).
			Set(m).
			Where(goqu.Ex{
				"tenant":            goqu.Op{"eq": tenant},
				"metering_point_id": goqu.Op{"eq": m.MeteringPoint},
				"active":            goqu.Op{"eq": 1},
			}).
			ToSQL()

		if err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("Error while creating statement")
		}

		r, err := tx.Exec(sql)
		if err != nil {
			log.WithField("SQL", "UPDATE").Errorf("Stmt: %s", sql)
			return model.ErrUpdateMeter(err)
		}
		rowsAffected, err := r.RowsAffected()
		if err != nil || rowsAffected == 0 {
			log.WithField("SQL", "UPDATE").Errorf("R: %d, E: %v, Stmt: %s", rowsAffected, err, sql)
		}

		// Todo:
		// Redesign table schema. It is not possible to update partition factor if the metering point exist multiple time in one eeg.
		// Exactly this behaviour exist when participant's mapping is changed or a new participant take over a metering point.

		//partFact := partFacts[i]
		//versionStmt := goqu.From(TABLE_PARTITION_FACT).Select(goqu.MAX("version")).Where(goqu.Ex{
		//	"tenant":            goqu.Op{"eq": tenant},
		//	"metering_point_id": goqu.Op{"eq": m.MeteringPoint},
		//})
		//sql, _, _ = goqu.Update(TABLE_PARTITION_FACT).
		//	Set(partFact).
		//	Where(goqu.Ex{
		//		"tenant":            goqu.Op{"eq": tenant},
		//		"metering_point_id": goqu.Op{"eq": m.MeteringPoint},
		//		"version":           goqu.Op{"eq": versionStmt},
		//	}).
		//	ToSQL()
		//
		//r, err = tx.Exec(sql)
		//if err != nil {
		//	log.WithField("SQL", "UPDATE").Errorf("Stmt: %s", sql)
		//	return model.ErrUpdateMeter(err)
		//}
		//rowsAffected, err = r.RowsAffected()
		//if err != nil || rowsAffected == 0 {
		//	log.WithField("SQL", "UPDATE").Errorf("R: %d, E: %v, Stmt: %s", rowsAffected, err, sql)
		//}
	}

	return nil
}

func FindMeteringPointsForTenant(db *sqlx.DB, tenant string) ([]*model.MeteringPoint, error) {
	stmt, _, _ := pgDialect.From(TABLE_METERINGPOINT).
		Select("metering_point_id").
		Where(goqu.C("tenant").Eq(tenant)).Order(goqu.I("direction").Asc()).ToSQL()

	mIds := []string{}
	err := db.Select(&mIds, stmt)
	if err != nil {
		log.WithField("tenant", tenant).WithError(err).Errorf("Stmt: %s", stmt)
		return nil, err
	}
	return findMeteringByIdAndState(db, tenant, mIds, nil)
}

func FindMeteringPointsActivePeriod(db *sqlx.DB, tenant string, activeSince, inactiveSince int64) ([]*model.MeteringPoint, error) {
	subStmt := pgDialect.From(TABLE_METERINGPOINT).Where(
		goqu.C("activesince").Lte(civil.DateOf(time.UnixMilli(activeSince))),
		goqu.C("tenant").Eq(tenant))

	stmt, _, _ := pgDialect.From(subStmt.As("a")).Select("metering_point_id").
		Where(goqu.I("inactivesince").Gte(civil.DateOf(time.UnixMilli(inactiveSince)))).
		ToSQL()

	mIds := []string{}
	err := db.Select(&mIds, stmt)
	if err != nil {
		log.WithField("tenant", tenant).WithError(err).Errorf("Stmt: %s", stmt)
		return nil, err
	}
	return findMeteringByIdAndState(db, tenant, mIds, nil)
}

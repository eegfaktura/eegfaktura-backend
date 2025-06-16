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
	Participant_id string            `goqu:"skipupdate"`
	Tenant         string            `goqu:"skipupdate"`
	ActiveSince    civil.NullDate    `db:"activesince" goqu:"omitempty"`
	InactiveSince  civil.NullDate    `db:"inactivesince" goqu:"omitempty"`
	Active         int               `db:"active"`
	Flag           model.ProcessFlag `goqu:"skipupdate"`
}

type partitionFactorRecord struct {
	MeteringPoint  string `db:"metering_point_id"`
	Participant_id string `goqu:"skipupdate"`
	Tenant         string `goqu:"skipupdate"`
	PartFact       int    `db:"partFact"`
	CreatedBy      string `db:"createdBy"`
}

func createMeteringEntries(tenant, username, participantId string, points []*model.MeteringPoint, processState *model.ProcessStatusType) ([]*meteringEntryType, []*partitionFactorRecord) {
	var partFactEntries []*partitionFactorRecord
	var meteringEntries []*meteringEntryType

	getDateWithDefault := func(d civil.NullDate, defaultDate civil.NullDate) civil.NullDate {
		if d.Valid {
			return d
		}
		//return civil.NullDate{Date: civil.DateFor(time.Now().Year(), 1, 1), Valid: true}
		return defaultDate
	}

	for _, p := range points {
		var activeSince civil.NullDate
		var inactiveSince civil.NullDate

		if processState != nil {
			p.ProcessState = *processState
		}
		p.ModifiedBy = null.StringFrom(username)
		p.ModifiedAt = civil.Now()
		if p.RegisteredSince.IsZero() {
			p.RegisteredSince = civil.Today()
		}
		if len(p.ProcessState) == 0 {
			p.ProcessState = model.NEW
		} else if p.ProcessState == model.ACTIVE {
			if p.State != nil {
				activeSince = getDateWithDefault(p.State.ActiveSince, civil.NullDate{Date: p.RegisteredSince, Valid: true})
				inactiveSince = getDateWithDefault(p.State.InactiveSince, civil.NullDate{Date: civil.DateFor(2999, 12, 31), Valid: true})
			} else {
				activeSince = civil.NullDate{Date: p.RegisteredSince, Valid: true}
				inactiveSince = civil.NullDate{Date: civil.DateFor(2999, 12, 31), Valid: true}
			}
		}
		p.Status = calcState(p.ProcessState)

		meteringEntries = append(meteringEntries,
			&meteringEntryType{p, participantId, tenant,
				activeSince, inactiveSince, int(calcActive(p.Status)), model.F_ASSIGNED})

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
	return saveMeteringPoint(tx, tenant, meteringEntries, partFactEntries)
}

// saveMeteringPoint creates new metering point in the database.
// Accourding to the status of new metering point (ACTIVE when excel import; NEW otherwise) the flag of the meterstate will be adapted
func saveMeteringPoint(tx *sqlx.Tx, tenant string, meteringEntry []*meteringEntryType, partFactEntries []*partitionFactorRecord) error {
	if len(meteringEntry) == 0 || len(partFactEntries) == 0 {
		log.Warn("Save Meteringpoints with empty list of metering points or partFact entries")
		return nil
	}

	// Check if a metering point already exists. Even when this metering point will be assigned to a new participant.
	// Because of dependencies in invoices it is not possible to delete existing relationships between participant and metering point
	// All existing metering points will be tagged to moved (flag = 0)
	meterIds := make([]string, 0)
	for _, entry := range meteringEntry {
		meterIds = append(meterIds, entry.MeteringPoint.MeteringPoint)
	}

	existingMeters := []string{}
	stmtExisting, _, err := pgDialect.From(TABLE_METERINGPOINT).Select("metering_point_id").
		Where(goqu.Ex{
			"metering_point_id": meterIds,
			"tenant":            tenant,
			"status":            model.S_INACTIVE},
		).ToSQL()

	if err := tx.Select(&existingMeters, stmtExisting); err == nil && len(existingMeters) > 0 {
		updateExistingStmt, _, err := pgDialect.Update(TABLE_METERINGPOINT).
			Set(goqu.Record{"flag": 0, "process_state": model.INACTIVE}).
			Where(goqu.Ex{
				"metering_point_id": existingMeters,
				"tenant":            tenant,
				"status":            model.S_INACTIVE,
			}).
			ToSQL()
		if err != nil {
			return model.ErrSaveMeteringPoint(err)
		}
		_, err = tx.Exec(updateExistingStmt)
		if err != nil {
			log.WithField("SQL", "UPDATE").Errorf("Stmt: %v", updateExistingStmt)
			return model.ErrSaveMeteringPoint(err)
		}
	}
	// -------------------------------------------

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

//func calcFlag(status model.ProcessStatusType) model.ProcessFlag {
//	switch status {
//	case model.ACTIVE:
//		return model.F_MOVED
//	default:
//		return model.F_ASSIGNED
//	}
//}
//
//func calcFlagPtr(status *model.ProcessStatusType) *model.ProcessFlag {
//	if status == nil {
//		return nil
//	}
//	flag := calcFlag(*status)
//	return &flag
//}

func calcActive(status model.StatusType) model.ProcessStatus {
	switch status {
	case model.S_INACTIVE:
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

	meteringEntries, partFactEntries := createMeteringEntries(tenant, username, participantId, []*model.MeteringPoint{point}, &point.ProcessState)
	err = saveMeteringPoint(tx, tenant, meteringEntries, partFactEntries)
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
	return nil
}

func UpdateMeteringPointPartial(db *sqlx.DB, tenant, username, participantId, meterId string, values map[string]interface{}) error {

	values["modifiedBy"] = username
	values["modifiedAt"] = civil.Now()

	statement, _, err := pgDialect.Update(TABLE_METERINGPOINT).Set(values).
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

// RemoveMeteringPoint Deletes metering point in the database. All relations to any participants will be gone.
// A metering point can only be removed even no activation was ever be established. The status column must be in state INIT.
// This indicates a metering point waiting for to be part of a community.
// In case of the metering point is revoked by the net operator the status flag is switching to INACTIVE.
// !!Never set the status to INIT once it has been activated!
func RemoveMeteringPoint(db *sqlx.DB, tenant, participantId, meterId string) error {
	statement, _, err := goqu.Delete(TABLE_METERINGPOINT).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participantId},
			"process_state":     goqu.Op{"eq": "INVALID"},
			"status":            goqu.Op{"eq": "INIT"},
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

func calcState(processState model.ProcessStatusType) model.StatusType {
	switch processState {
	case model.INACTIVE:
		return model.S_INACTIVE
	case model.ACTIVE:
		return model.S_ACTIVE
	default:
		return model.S_INIT
	}
}
func ArchiveMeteringPoint(db *sqlx.DB, tenant, participantId, meterId string) error {
	statement, _, err := goqu.Update(TABLE_METERINGPOINT).
		Set(goqu.Record{"status": model.S_INACTIVE, "process_state": model.ARCHIVED, "flag": 2}).
		Where(goqu.Ex{
			"participant_id":    goqu.Op{"eq": participantId},
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
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

// MeteringPointsSetStatus The MeteringPointSetStatus method handles only ECON or ECOF messages.
// It requires that only one metering point of a community is assigned (flag == 1).
func MeteringPointsSetStatus(db *sqlx.DB, tenant string, processState model.ProcessStatusType, statusCode *int16, meterId []string, activeSince *civil.Date, consentId *string) error {

	// setStatus In case of the ABSCHLUSS message is received before the APPROVED message the processtatus should remain in ACTIVE mode.
	setStatus := func(status model.ProcessStatusType) goqu.Expression {
		switch status {
		case model.APPROVED:
			return goqu.Case().When(goqu.C("process_state").Eq(string(model.ACTIVE)), string(model.ACTIVE)).Else(goqu.V(status))
		default:
			return goqu.V(status)
		}
	}

	updateRecord := map[string]interface{}{
		"process_state": setStatus(processState),
		"modifiedAt":    civil.Now(),
		"modifiedBy":    "EVU",
	}

	if processState == model.INIT {
		updateRecord["activesince"] = goqu.V(nil)
	}

	if consentId != nil {
		updateRecord["consent_id"] = *consentId
	}
	if statusCode != nil {
		updateRecord["statusCode"] = *statusCode
		//} else {
		//	updateRecord["statusCode"] = nil
	}
	if processState == model.ACTIVE {
		updateRecord["status"] = model.S_ACTIVE
	}
	if activeSince != nil {
		updateRecord["activesince"] = goqu.COALESCE(goqu.C("activesince"), *activeSince)
		updateRecord["inactivesince"] = civil.DateFor(2999, 12, 31)
	}

	/**
	Consider in case reactivating the metering point for the same participant, the inactivesince time must be adjusted to the very end time period.
	The activesince time must be left alone as it controls the visibility of the time period in the user client.
	Therefore, the activesince time is only set at creation time.

	IMPROVE: Check the context of the meteringpoint while activating.
	*/
	log.WithField("tenant", tenant).Infof("Change Status. Meters: %v activeSince: %v status: %v", meterId, activeSince, processState)

	flag := model.F_ASSIGNED
	statement, _, err := goqu.Update(TABLE_METERINGPOINT).
		Set(updateRecord).
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

func MeteringPointRevoke(db *sqlx.DB, tenant, meterId string, consentEnd civil.Date) error {

	log.Debugf("Revoke Meter: %s at %v\n", meterId, consentEnd)

	//participant, err := FindParticipantByMeteringPoint(db, tenant, meterId)
	//if err != nil {
	//	return model.ErrFindParticipant(err)
	//}

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
			"process_state": model.INACTIVE,
			"status":        model.INACTIVE,
			"modifiedAt":    civil.Now(),
			"modifiedBy":    "EVU",
			"inactivesince": consentEnd}).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"flag":              model.F_ASSIGNED,
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
					goqu.C("flag").Eq(model.F_ASSIGNED))))
	} else {
		whereClause = goqu.And(
			goqu.C("metering_point_id").Eq(meterId),
			goqu.C("flag").Eq(model.F_ASSIGNED))
	}

	update := tx.Update(TABLE_METERINGPOINT).
		Set(goqu.Record{
			"process_state": goqu.Case().
				When(goqu.C("process_state").Eq("ACTIVE"), model.INACTIVE).Else(goqu.C("process_state")),
			"status": goqu.Case().
				When(goqu.C("status").Eq("INIT"), model.S_INIT).Else(model.S_INACTIVE),
			"modifiedAt":    civil.Now(),
			"modifiedBy":    "EVU",
			"inactivesince": goqu.Case().When(goqu.C("inactivesince").IsNotNull(), consentEnd).Else(goqu.C("inactivesince")),
		}).
		Where(whereClause /*, goqu.ExOr{}*/).
		Returning("tenant").
		Executor()

	stmt, _, err1 := update.ToSQL()
	log.WithField("metering_point_id", meterId).Infof("Update Meteringpoint state: %s - %v", stmt, err1)
	var tenants []string
	if err = update.ScanVals(&tenants); err != nil {
		return nil, model.ErrUpdateMeter(err)
	}

	if len(tenants) != 1 {
		log.Warnf("Meteringpoint %s is not unique. %d-[%+v]", meterId, len(tenants), tenants)
		err = model.ErrUpdateMeter(errors.New(fmt.Sprintf("Meteringpoint %s is not unique", meterId)))
		return nil, err
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

	_, err = tx.Exec(stmt)
	if err != nil {
		log.WithField("SQL", "SELECT").Errorf("Stmt: %s", stmt)
		return model.ErrUpdateMeter(err)
	}
	return tx.Commit()
}

func FindInactiveMeteringById(db *sqlx.DB, tenant string, meterId string) ([]*model.MeteringPoint, error) {
	mode := model.S_INACTIVE
	return findMeteringByIdAndState(db, tenant, []string{meterId}, &mode)
}

func FindNewMeteringById(db *sqlx.DB, tenant string, meterId string) ([]*model.MeteringPoint, error) {
	mode := model.S_INIT
	return findMeteringByIdAndState(db, tenant, []string{meterId}, &mode)
}

func FindActiveMeteringByIds(db *sqlx.DB, tenant string, meterIds []string) ([]*model.MeteringPoint, error) {
	mode := model.S_ACTIVE
	return findMeteringByIdAndState(db, tenant, meterIds, &mode)
}

func FindMeteringByIds(db *sqlx.DB, tenant string, meterIds []string) ([]*model.MeteringPoint, error) {
	return findMeteringByIdAndState(db, tenant, meterIds, nil)
}

func FindAllMeteringByTenant(tx *sqlx.DB, tenant, participantId string, meterIds []string) ([]*model.MeteringPoint, error) {
	var m []*model.MeteringPoint

	stateStmt := pgDialect.From(TABLE_METERINGPOINT).
		Select(
			goqu.C("activesince"),
			goqu.C("inactivesince"),
			goqu.C("flag"),
			goqu.C("metering_point_id").As("mid"),
			goqu.C("tenant").As("mid_tenant"))

	partFactStmt := pgDialect.From(TABLE_PARTITION_FACT_VIEW).
		Select(
			goqu.C("partFact"),
			goqu.C("metering_point_id").As("mpfmid"),
			goqu.C("participant_id").As("mpfpid"),
			goqu.C("tenant").As("mpf_tenant"))

	stmt, _, err := pgDialect.From(TABLE_METERINGPOINT, stateStmt.As("state"), partFactStmt.As("mpfpF2")).Select(&model.MeteringPoint{}).
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
	mode := model.S_ACTIVE
	m, err := findMeteringByIdAndState(tx, tenant, []string{meterId}, &mode)
	if err != nil {
		return nil, err
	}
	if len(m) == 1 {
		return m[0], nil
	}
	return nil, model.ErrFindMeter(errors.New("More as one active Meteringpoint was found"))
}

func FindMeteringByStatus(tx *sqlx.DB, tenant, meterId string, status model.StatusType) (*model.MeteringPoint, error) {
	m, err := findMeteringByIdAndState(tx, tenant, []string{meterId}, &status)
	if err != nil {
		return nil, err
	}
	if len(m) == 1 {
		return m[0], nil
	}
	return nil, model.ErrFindMeter(errors.New("More as one active Meteringpoint was found"))
}

func findMeteringByIdAndState(db *sqlx.DB, tenant string, meterIds []string, status *model.StatusType) ([]*model.MeteringPoint, error) {
	var m []*model.MeteringPoint

	stateStmt := pgDialect.From(TABLE_METERINGPOINT).
		Select(
			goqu.C("activesince"),
			goqu.C("inactivesince"),
			goqu.C("flag"),
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
	if status != nil {
		whereClause["status"] = *status
		whereClause["state.flag"] = model.F_ASSIGNED
	}

	stmt, _, err := pgDialect.From(TABLE_METERINGPOINT, stateStmt.As("state"), partFactStmt.As("mpfpF3")).Select(&model.MeteringPoint{}).
		Where(whereClause).
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

func UpdateActiveMeteringPoints(db *sqlx.DB, tenant string, ml []model.Meter) error {
	tml := model.StandardizeMeteringPointList(ml)

	status := model.S_ACTIVE
	processState := model.ACTIVE
	//meteringPoints := model.ConvertToDbMeterList(tml)
	//partFacts := model.ConvertToDbMeterPartFactList(tml)

	execDB := goqu.New("postgres", db)

	tx, err := execDB.Begin()
	if err != nil {
		return model.ErrOpenTx(err)
	}
	defer func() {
		switch err {
		case nil:
			_ = tx.Commit()
		default:
			_ = tx.Rollback()
		}
	}()

	for _, m := range tml {
		dbMeter := model.ConvertToDbMeter(m)
		dbMeter.Status = &status
		dbMeter.ProcessState = &processState
		//m.Active = calcActivePtr(m.Status)
		//m.Flag = calcFlagPtr(m.Status)
		record, err := exp.NewRecordFromStruct(*dbMeter, false, true)
		if _, ok := record["activesince"]; ok {
			record["activesince"] = goqu.Case().
				When(goqu.COALESCE(goqu.C("activesince"), dbMeter.ActiveSince).Gte(dbMeter.ActiveSince), dbMeter.ActiveSince).Else(goqu.C("activesince"))
		}
		updateMeter := tx.Update(TABLE_METERINGPOINT).
			Set(record).
			Where(goqu.Ex{
				"tenant":            goqu.Op{"eq": tenant},
				"metering_point_id": goqu.Op{"eq": m.MeteringPoint},
				"flag":              goqu.Op{"eq": model.F_ASSIGNED},
			}).
			Returning("participant_id").
			Executor()

		var participantId string
		found, err := updateMeter.ScanVal(&participantId)

		//sql, _, _ := updateMeter.ToSQL()
		//fmt.Printf("STATEMENT: %v\n", sql)
		if err != nil {
			sql, _, _ := updateMeter.ToSQL()
			log.WithField("SQL", "UPDATE").Errorf("Stmt: %s", sql)
			return model.ErrUpdateMeter(err)
		}

		if found {
			partFact := model.ConvertToDbMeterPartFact(m)
			versionStmt := goqu.From(TABLE_PARTITION_FACT).Select(goqu.MAX("version")).Where(goqu.Ex{
				"tenant":            goqu.Op{"eq": tenant},
				"participant_id":    goqu.Op{"eq": participantId},
				"metering_point_id": goqu.Op{"eq": m.MeteringPoint},
			})
			updatePartFact := tx.Update(TABLE_PARTITION_FACT).
				Set(partFact).
				Where(goqu.Ex{
					"tenant":            goqu.Op{"eq": tenant},
					"metering_point_id": goqu.Op{"eq": m.MeteringPoint},
					"participant_id":    goqu.Op{"eq": participantId},
					"version":           goqu.Op{"eq": versionStmt},
				}).
				Executor()

			_, err = updatePartFact.Exec()
			if err != nil {
				sql, _, _ := updatePartFact.ToSQL()
				log.WithField("SQL", "UPDATE").Errorf("Stmt: %s", sql)
				return model.ErrUpdateMeter(err)
			}
		}

		// Todo:
		// Redesign table schema. It is not possible to update partition factor if the metering point exist multiple times in one eeg.
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
		goqu.C("activesince").Lte(civil.DateOf(time.UnixMilli(inactiveSince))),
		goqu.C("tenant").Eq(tenant))

	stmt, _, _ := pgDialect.From(subStmt.As("a")).Select("metering_point_id").
		Where(goqu.I("inactivesince").Gte(civil.DateOf(time.UnixMilli(activeSince)))).
		ToSQL()

	fmt.Printf("FindMeteringPointsActivePeriod Stmt: %s\n", stmt)
	mIds := []string{}
	err := db.Select(&mIds, stmt)
	if err != nil {
		log.WithField("tenant", tenant).WithError(err).Errorf("Stmt: %s", stmt)
		return nil, err
	}
	return findMeteringByIdAndState(db, tenant, mIds, nil)
}

package database

import (
	"at.ourproject/vfeeg-backend/model"
	dbsql "database/sql"
	"errors"
	"fmt"
	"github.com/doug-martin/goqu/v9"
	"github.com/jjeffery/civil"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

const TABLE_PARTICIPANT = "base.participant"

func GetParticipants(db *sqlx.DB, tenant string) ([]model.EegParticipant, error) {
	var participants []model.EegParticipant = []model.EegParticipant{}

	stmt, _, err := pgDialect.From("base.participant").Select(&participants).
		Where(goqu.Ex{
			"base.participant.tenant": tenant, "status": goqu.Op{"neq": "ARCHIVED"}}).Order(goqu.I("lastname").Asc()).ToSQL()
	if err != nil {
		return []model.EegParticipant{}, model.ErrGetParticipant(err)
	}

	err = db.Select(&participants, stmt)
	if err != nil {
		return []model.EegParticipant{}, model.ErrGetParticipant(err)
	}

	for i, _ := range participants {
		err = completeParticipant(db, &participants[i])
		if err != nil {
			log.WithField("tenant", tenant).Errorf("Cannot fetch Participant correct: %s", err.Error())
		}
		if participants[i].MeteringPoint == nil {
			participants[i].MeteringPoint = make([]*model.MeteringPoint, 0)
		}
	}

	return participants, nil
}

func QueryParticipant(db *sqlx.DB, participantId string) (*model.EegParticipant, error) {
	var participant model.EegParticipant = model.EegParticipant{}

	sql, _, err := pgDialect.From("base.participant").Select(&participant).Where(goqu.C("id").Eq(participantId)).ToSQL()
	if err != nil {
		return nil, model.ErrGetParticipant(err)
	}
	err = db.Get(&participant, sql)
	if err != nil {
		return nil, model.ErrGetParticipant(err)
	}

	err = completeParticipant(db, &participant)
	if err != nil {
		return nil, err
	}

	return &participant, nil
}

//func CompleteParticipant(db *sqlx.DB, p *model.EegParticipant) error {
//	sql, _, err := pgDialect.From("base.contactdetail").Select(&p.Contact).Where(goqu.C("participant_id").Eq(p.Id.String())).ToSQL()
//	if err != nil {
//		return err
//	}
//	err = db.Get(&(p.Contact), sql)
//	if err != nil && err != dbsql.ErrNoRows {
//		return err
//	}
//
//	sql, _, err = pgDialect.From("base.bankaccount").Select(&p.BankAccount).Where(goqu.C("participant_id").Eq(p.Id.String())).ToSQL()
//	if err != nil {
//		return err
//	}
//	err = db.Get(&(p.BankAccount), sql)
//	if err != nil && err != dbsql.ErrNoRows {
//		return err
//	}
//
//	sql, _, err = pgDialect.From("base.address").Select(&p.BillingAddress).
//		Where(goqu.C("participant_id").Eq(p.Id.String()), goqu.C("type").Eq("BILLING")).ToSQL()
//	if err != nil {
//		return err
//	}
//	err = db.Get(&(p.BillingAddress), sql)
//	if err != nil && err != dbsql.ErrNoRows {
//		return err
//	}
//
//	sql, _, err = pgDialect.From("base.address").Select(&p.ResidentAddress).
//		Where(goqu.C("participant_id").Eq(p.Id.String()), goqu.C("type").Eq("RESIDENCE")).ToSQL()
//	if err != nil {
//		return err
//	}
//	//fmt.Printf("SQL: %+v\n", sql)
//	err = db.Get(&(p.ResidentAddress), sql)
//	if err != nil && err != dbsql.ErrNoRows {
//		return err
//	}
//	//fmt.Printf("ADDRESS: %+v\n", p.ResidentAddress)
//
//	sql, _, err = pgDialect.From("base.meteringpoint").Select(&p.MeteringPoint).
//		LeftJoin(goqu.T("participant_meter_state").Schema("base"), goqu.On(
//			goqu.S("base").Table("meteringpoint").Col("metering_point_id").
//				Eq(goqu.S("base").Table("participant_meter_state").Col("metering_point")),
//			goqu.S("base").Table("meteringpoint").Col("tenant").
//				Eq(goqu.S("base").Table("participant_meter_state").Col("tenant")),
//			goqu.S("base").Table("meteringpoint").Col("participant_id").
//				Eq(goqu.S("base").Table("participant_meter_state").Col("participant_id")),
//		)).
//		Where(goqu.C("participant_id").Table("meteringpoint").Eq(p.Id.String())).ToSQL()
//	if err != nil {
//		return err
//	}
//	fmt.Printf("STMT: %+v\n", sql)
//	err = db.Select(&(p.MeteringPoint), sql)
//	if err != nil && err != dbsql.ErrNoRows {
//		return err
//	}
//	return nil
//}

func UpdateParticipant(db *sqlx.DB, tenant, user string, participant *model.EegParticipant) error {
	sql, _, _ := goqu.Update("base.participant").
		Set(participant).
		Where(goqu.Ex{
			"tenant": goqu.Op{"eq": tenant},
			"id":     goqu.Op{"eq": participant.Id.String()},
		}).
		ToSQL()
	_, err := db.Exec(sql)
	if err != nil {
		return err
	}

	sql, _, _ = goqu.Update("base.contactdetail").
		Set(participant.Contact).
		Where(goqu.Ex{
			"participant_id": participant.Id.String(),
		}).
		ToSQL()
	_, err = db.Exec(sql)
	if err != nil {
		return err
	}

	sql, _, _ = goqu.Update("base.address").
		Set(participant.ResidentAddress).
		Where(goqu.Ex{
			"type":           model.RESIDENCE,
			"participant_id": participant.Id.String(),
		}).
		ToSQL()
	_, err = db.Exec(sql)
	if err != nil {
		return err
	}

	sql, _, _ = goqu.Update("base.address").
		Set(participant.BillingAddress).
		Where(goqu.Ex{
			"type":           model.BILLING,
			"participant_id": participant.Id.String(),
		}).
		ToSQL()
	_, err = db.Exec(sql)
	if err != nil {
		return err
	}

	sql, _, _ = goqu.Update("base.bankaccount").
		Set(participant.BankAccount).
		Where(goqu.Ex{
			"participant_id": participant.Id.String(),
		}).
		ToSQL()
	_, err = db.Exec(sql)
	if err != nil {
		return err
	}
	return err
}

type ParticipantWithMeta struct {
	*model.EegParticipant
	Tenant           string    `db:"tenant"`
	CreatedBy        string    `db:"createdBy"`
	LastmodifiedBy   string    `db:"lastModifiedBy"`
	LastmodifiedDate time.Time `db:"lastModifiedDate"`
}

// RegisterParticipant func RegisterParticipant(dbConn OpenDbXConnection, tenant, username string, participant *model.EegParticipant) error {
func RegisterParticipant(tx *sqlx.Tx, tenant, username string, participant *model.EegParticipant) error {
	participant.Status = model.PENDING
	participant.Id = uuid.NewUUID()
	//participant.ParticipantSince = time.Now()
	participant.CreatedBy = username
	return saveParticipant(tx, tenant, username, participant, ImportMeteringPoints)
}

// ImportParticipant func ImportParticipant(dbConn OpenDbXConnection, tenant, username string, participant *model.EegParticipant) error {
func ImportParticipant(tx *sqlx.Tx, tenant, username string, participant *model.EegParticipant) error {

	// check if User already exists
	stmt, _, err := pgDialect.From("base.participant").
		Select("id").
		Where(
			goqu.C("firstname").Eq(participant.FirstName),
			goqu.C("lastname").Eq(participant.LastName),
			goqu.C("tenant").Eq(tenant)).ToSQL()
	if err != nil {
		return err
	}
	participantId := ""
	err = tx.Get(&participantId, stmt)
	if err == nil {
		return ImportMeteringPoints(tx, tenant, username, participantId, participant.MeteringPoint)
	}

	participant.Id = uuid.NewUUID()
	return saveParticipant(tx, tenant, username, participant, ImportMeteringPoints)
}

func ConfirmParticipant(db *sqlx.DB, username, participantId string) error {
	_, err := db.Exec("UPDATE base.participant SET status = 'ACTIVE', \"lastModifiedDate\" = 'now()', \"lastModifiedBy\" = $1 WHERE id = $2", username, participantId)
	if err != nil {
		return model.ErrCompleteParticipant(err)
	}
	return nil
}

func saveParticipant(tx *sqlx.Tx, tenant, username string, participant *model.EegParticipant,
	registerMeteringPointsFunc func(*sqlx.Tx, string, string, string, []*model.MeteringPoint) error) error {

	registeringParticipant := ParticipantWithMeta{
		participant, tenant, username, username, time.Now(),
	}

	//if participant.ParticipantSince.IsZero() {
	//	participant.ParticipantSince = time.Now()
	//}

	participantId := ""
	sql, _, _ := pgDialect.Insert("base.participant").Rows(registeringParticipant).Returning("id").ToSQL()
	err := tx.QueryRow(sql).Scan(&participantId)
	if err != nil {
		return model.ErrRegisterParticipant(err)
	}

	contactEntry := struct {
		model.ContactInfo
		Participant_id string
	}{participant.Contact, participantId}
	sql, _, _ = pgDialect.Insert("base.contactdetail").Rows(contactEntry).ToSQL()
	_, err = tx.Exec(sql)
	if err != nil {
		return model.ErrRegisterParticipant(err)
	}

	bankInfoEntry := struct {
		model.BankInfo
		Participant_id string
	}{participant.BankAccount, participantId}

	sql, _, _ = pgDialect.Insert("base.bankaccount").Rows(bankInfoEntry).ToSQL()
	_, err = tx.Exec(sql)
	if err != nil {
		return model.ErrRegisterParticipant(err)
	}

	billingAddrEntry := struct {
		model.Address
		Participant_id string
	}{participant.BillingAddress, participantId}
	residenceAddrEntry := struct {
		model.Address
		Participant_id string
	}{participant.ResidentAddress, participantId}
	sql, _, _ = pgDialect.Insert("base.address").Rows(billingAddrEntry, residenceAddrEntry).ToSQL()
	_, err = tx.Exec(sql)
	if err != nil {
		return model.ErrRegisterParticipant(err)
	}

	err = registerMeteringPointsFunc(tx, tenant, username, participantId, participant.MeteringPoint)
	if err != nil {
		return model.ErrRegisterParticipant(err)
	}
	return nil
}

func ArchiveParticipant(db *sqlx.DB, user string, id string) error {
	stmt, _, err := pgDialect.Update("base.participant").
		Set(goqu.Record{"status": "ARCHIVED", "lastModifiedDate": time.Now(), "lastModifiedBy": user}).
		Where(goqu.Ex{"id": id}).ToSQL()
	if err != nil {
		return model.ErrArchiveParticipant(err)
	}
	_, err = db.Exec(stmt)
	if err != nil {
		return model.ErrArchiveParticipant(err)
	}
	return nil
}

func UpdateParticipantPartial(db *sqlx.DB, participantId, name string, value interface{}) error {

	var stmt *goqu.UpdateDataset
	var sql string
	fields := map[string]interface{}{}

	names := strings.Split(name, ".")
	if len(names) == 2 {
		switch names[0] {
		case "billingAddress":
			stmt = pgDialect.Update("base.address").
				Where(goqu.Ex{"participant_id": goqu.V(participantId)}, goqu.Ex{"type": goqu.V("BILLING")})
		case "residentAddress":
			stmt = pgDialect.Update("base.address").
				Where(goqu.Ex{"participant_id": goqu.V(participantId)}, goqu.Ex{"type": goqu.V("RESIDENCE")})
		case "contact":
			stmt = pgDialect.Update("base.contactdetail").
				Where(goqu.Ex{"participant_id": goqu.V(participantId)})
		case "accountInfo":
			stmt = pgDialect.Update("base.bankaccount").
				Where(goqu.Ex{"participant_id": goqu.V(participantId)})
		default:
			return model.ErrUpdateParticipant(errors.New(fmt.Sprintf("Can not update structure of %s", name)))
		}
		fields[names[1]] = value
		sql, _, _ = stmt.Set(fields).ToSQL()

	} else if len(names) == 1 {
		fields[names[0]] = value
		sql, _, _ = pgDialect.Update("base.participant").Set(fields).
			Where(goqu.Ex{"id": goqu.V(participantId)}).ToSQL()
	} else {
		return model.ErrUpdateParticipant(errors.New(fmt.Sprintf("Can not update structure of %s", name)))
	}

	res, err := db.Exec(sql)
	if err == nil {
		if rows, err := res.RowsAffected(); rows == 0 || err != nil {
			err = InsertParticipantPartial(db, participantId, name, value)
			if err != nil {
				return err
			}
		}
		return nil
	} else {
		return model.ErrUpdateParticipant(err)
	}
}

func InsertParticipantPartial(db *sqlx.DB, participantId, name string, value interface{}) error {

	var stmt *goqu.InsertDataset
	var sql string
	fields := map[string]interface{}{}

	names := strings.Split(name, ".")
	if len(names) == 2 {
		fields["participant_id"] = participantId
		switch names[0] {
		case "billingAddress":
			stmt = pgDialect.Insert("base.address")
			fields["type"] = "BILLING"
		case "residentAddress":
			stmt = pgDialect.Insert("base.address")
			fields["type"] = "RESIDENCE"
		case "contact":
			stmt = pgDialect.Insert("base.contactdetail")
		case "accountInfo":
			stmt = pgDialect.Insert("base.bankaccount")
		default:
			return model.ErrInsertParticipant(errors.New(fmt.Sprintf("Can not update structure of %s", name)))
		}
		fields[names[1]] = value
		sql, _, _ = stmt.Rows(fields).ToSQL()
	} else {
		return model.ErrInsertParticipant(errors.New(fmt.Sprintf("Can not update structure of %s", name)))
	}

	_, err := db.Exec(sql)
	if err != nil {
		return model.ErrInsertParticipant(err)
	}
	return nil
}

func GetParticipant(db *sqlx.DB, participantId string) (*model.EegParticipant, error) {
	participant := model.EegParticipant{}
	stmt, _, err := pgDialect.From("base.participant").Select(&participant).
		Where(goqu.Ex{
			"base.participant.id": participantId}).ToSQL()
	if err != nil {
		return nil, err
	}

	err = db.Get(&participant, stmt)
	if err != nil {
		return nil, err
	}

	err = completeParticipant(db, &participant)
	return &participant, err
}

func completeParticipant(db *sqlx.DB, participant *model.EegParticipant) error {

	participantId := participant.Id.String()

	stmt, _, err := pgDialect.From("base.contactdetail").Select(&participant.Contact).Where(goqu.C("participant_id").Eq(participantId)).ToSQL()
	if err != nil {
		return model.ErrCompleteParticipant(err)
	}
	err = db.Get(&(participant.Contact), stmt)
	if err != nil && !errors.Is(err, dbsql.ErrNoRows) {
		return model.ErrCompleteParticipant(err)
	}

	stmt, _, err = pgDialect.From("base.bankaccount").Select(&participant.BankAccount).Where(goqu.C("participant_id").Eq(participantId)).ToSQL()
	if err != nil {
		return model.ErrCompleteParticipant(err)
	}
	err = db.Get(&(participant.BankAccount), stmt)
	if err != nil && !errors.Is(err, dbsql.ErrNoRows) {
		return model.ErrCompleteParticipant(err)
	}

	stmt, _, err = pgDialect.From("base.address").Select(&participant.BillingAddress).
		Where(goqu.C("participant_id").Eq(participantId), goqu.C("type").Eq("BILLING")).ToSQL()
	if err != nil {
		return model.ErrCompleteParticipant(err)
	}
	err = db.Get(&(participant.BillingAddress), stmt)
	if err != nil && !errors.Is(err, dbsql.ErrNoRows) {
		return model.ErrCompleteParticipant(err)
	}

	stmt, _, err = pgDialect.From("base.address").Select(&participant.ResidentAddress).
		Where(goqu.C("participant_id").Eq(participantId), goqu.C("type").Eq("RESIDENCE")).ToSQL()
	if err != nil {
		return model.ErrCompleteParticipant(err)
	}
	err = db.Get(&(participant.ResidentAddress), stmt)
	if err != nil && !errors.Is(err, dbsql.ErrNoRows) {
		return model.ErrCompleteParticipant(err)
	}

	stateStmt := pgDialect.From("base.meteringpoint").
		Select(
			goqu.C("activesince"),
			goqu.C("inactivesince"),
			goqu.C("active"),
			goqu.C("metering_point_id").As("mid"),
			goqu.C("participant_id").As("pid"))

	partFactStmt := pgDialect.From(TABLE_PARTITION_FACT_VIEW).
		Select(
			goqu.C("partFact"),
			goqu.C("metering_point_id").As("mpfmid"),
			goqu.C("participant_id").As("mpfpid"))

	stmt, _, err = pgDialect.From("base.meteringpoint", stateStmt.As("state"), partFactStmt.As("mpfpF")).Select(&participant.MeteringPoint).
		Where(
			goqu.C("participant_id").Table("meteringpoint").Schema("base").Eq(participantId),
			goqu.C("mid").Eq(goqu.C("metering_point_id")),
			goqu.C("pid").Eq(goqu.C("participant_id")),
			goqu.C("mpfmid").Eq(goqu.C("metering_point_id")),
			goqu.C("mpfpid").Eq(goqu.C("participant_id")),
		).ToSQL()
	if err != nil {
		return model.ErrCompleteParticipant(err)
	}
	err = db.Select(&participant.MeteringPoint, stmt)
	if err != nil && !errors.Is(err, dbsql.ErrNoRows) {
		return model.ErrCompleteParticipant(err)
	}

	if participant.MeteringPoint == nil {
		log.Debugf("Participant (%+v) with zero Meteringpoints", participant.Id.String())
		participant.MeteringPoint = make([]*model.MeteringPoint, 0)
	}

	return nil
}

func FindParticipantByMeteringPoint(db *sqlx.DB, tenant, meteringPoint string) (*model.EegParticipant, error) {

	participant := model.EegParticipant{}

	participantIdStmt := pgDialect.From("base.meteringpoint").Select("participant_id").
		Where(
			goqu.C("metering_point_id").Eq(meteringPoint),
			goqu.C("tenant").Eq(tenant),
			//goqu.C("inactivesince").Gte("now()"),
			//goqu.C("activesince").Lte("now()"),
			goqu.C("inactivesince").Gte(civil.Today()),
			goqu.C("activesince").Lte(civil.Today()),
		)

	stmt, _, err := pgDialect.From(TABLE_PARTICIPANT).Select(&participant).Where(goqu.C("id").Eq(participantIdStmt)).ToSQL()
	if err != nil {
		log.WithField("SQL", "SELECT").Infof("Create Stmt: %+v, %+v", participant, participantIdStmt)
		return nil, err
	}

	err = db.Get(&(participant), stmt)
	if err != nil {
		log.WithField("SQL", "SELECT").Infof("Stmt: %s", stmt)
		return nil, err
	}

	err = completeParticipant(db, &participant)
	if err != nil {
		return nil, err
	}
	return &participant, nil
}

package database

import (
	dbsql "database/sql"
	"errors"
	"fmt"

	"strings"

	"at.ourproject/vfeeg-backend/model"
	"github.com/doug-martin/goqu/v9"
	"github.com/jjeffery/civil"
	"github.com/jmoiron/sqlx"

	//"github.com/mitchellh/mapstructure"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
)

type ParticipantRepository interface {
	GetParticipants(tenant string) ([]*model.EegParticipant, error)
	GetParticipant(participantId string) (*model.EegParticipant, error)
	GetParticipantByName(tenant string, email string) ([]*model.EegParticipant, error)
	ConfirmParticipant(username, participantId string) error
	RegisterParticipant(tenant, username string, participant *model.EegParticipant) error
	QueryParticipant(participantId string) (*model.EegParticipant, error)
	ImportParticipant(tenant, username string, participant *model.EegParticipant) error
	FindParticipantByMeteringPoint(tenant, meteringPoint string) (*model.EegParticipant, error)
	UpdateParticipant(tenant, user string, participant *model.EegParticipant) error
	UpdateParticipantPartial(participantId, name string, value interface{}) error
	UpdateParticipantValues(participantId, tenant string, values map[string]string) error
	DeleteParticipant(participantId string) error
}

func (db *sqlDatabase) GetParticipants(tenant string) ([]*model.EegParticipant, error) {
	return getParticipants(db.db, tenant)
}

func (db *sqlDatabase) GetParticipant(participantId string) (*model.EegParticipant, error) {
	return getParticipant(db.db, participantId)
}

func (db *sqlDatabase) GetParticipantByName(tenant, email string) ([]*model.EegParticipant, error) {
	return getParticipantByName(db.db, tenant, email)
}

func (db *sqlDatabase) RegisterParticipant(tenant, username string, participant *model.EegParticipant) error {
	tx, err := db.db.Beginx()
	if err != nil {
		return err
	}

	err = registerParticipant(tx, tenant, username, participant)
	if err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

func (db *sqlDatabase) ConfirmParticipant(username, participantId string) error {
	return confirmParticipant(db.db, username, participantId)
}

func (db *sqlDatabase) QueryParticipant(participantId string) (*model.EegParticipant, error) {
	return queryParticipant(db.db, participantId)
}

func (db *sqlDatabase) ImportParticipant(tenant, username string, participant *model.EegParticipant) error {
	tx, err := db.db.Beginx()
	if err != nil {
		return err
	}

	err = importParticipant(tx, tenant, username, participant)
	if err != nil {
		return errors.Join(err, tx.Rollback())
	}
	return tx.Commit()
}

func (db *sqlDatabase) FindParticipantByMeteringPoint(tenant, meteringPoint string) (*model.EegParticipant, error) {
	return findParticipantByMeteringPoint(db.db, tenant, meteringPoint)
}

func (db *sqlDatabase) UpdateParticipant(tenant, user string, participant *model.EegParticipant) error {
	return updateParticipant(db.db, tenant, user, participant)
}

func (db *sqlDatabase) UpdateParticipantPartial(participantId, name string, value interface{}) error {
	return updateParticipantPartial(db.db, participantId, name, value)
}

func (db *sqlDatabase) UpdateParticipantValues(participantId, tenant string, values map[string]string) error {
	var err error
	for k, v := range values {
		if err = updateParticipantPartial(db.db, participantId, k, v); err != nil {
			return err
		}
	}
	return nil
}

func (db *sqlDatabase) DeleteParticipant(participantId string) error {
	return deleteParticipant(db.db, participantId)
}

const TABLE_PARTICIPANT = "base.participant"

func getParticipants(db *sqlx.DB, tenant string) ([]*model.EegParticipant, error) {
	var participants []*model.EegParticipant = []*model.EegParticipant{}

	stmt, _, err := buildParticipantQueryStmt().
		Where(goqu.C("tenant").Eq(tenant)).
		ToSQL()
	if err != nil {
		return []*model.EegParticipant{}, model.ErrGetParticipant(err)
	}

	err = db.Select(&participants, stmt)
	if err != nil {
		return []*model.EegParticipant{}, model.ErrGetParticipant(err)
	}

	err = completeParticipants(db, tenant, participants)
	if err != nil {
		return []*model.EegParticipant{}, model.ErrGetParticipant(err)
	}

	//for i, _ := range participants {
	//	err = completeParticipant(db, &participants[i])
	//	if err != nil {
	//		log.WithField("tenant", tenant).Errorf("Cannot fetch Participant correct: %s", err.Error())
	//	}
	//	if participants[i].MeteringPoint == nil {
	//		participants[i].MeteringPoint = make([]*model.MeteringPoint, 0)
	//	}
	//}

	return participants, nil
}

func getParticipant(db *sqlx.DB, participantId string) (*model.EegParticipant, error) {
	participant := &model.EegParticipant{}

	stmt, _, err := buildParticipantQueryStmt().
		Where(goqu.C("id").Eq(participantId)).
		ToSQL()
	if err != nil {
		return participant, model.ErrGetParticipant(err)
	}

	err = db.Get(participant, stmt)
	if err != nil {
		return participant, model.ErrGetParticipant(err)
	}

	if participant != nil {
		err = completeParticipant(db, participant)
		if err != nil {
			log.Errorf("Cannot fetch Participant correct: %s", err.Error())
		}
		if participant.MeteringPoint == nil {
			participant.MeteringPoint = make([]*model.MeteringPoint, 0)
		}
	}

	return participant, nil
}

func getParticipantByName(db *sqlx.DB, tenant, email string) ([]*model.EegParticipant, error) {
	var participants []*model.EegParticipant

	subquery := goqu.
		From("base.contactdetail").
		Select("participant_id").
		Where(goqu.L("LOWER(email) = ?", strings.ToLower(email)))

	stmt, _, err := buildParticipantQueryStmt().
		Where(goqu.C("tenant").Eq(tenant),
			goqu.C("id").In(subquery)).
		ToSQL()

	if err != nil {
		return nil, model.ErrGetParticipant(err)
	}

	err = db.Select(&participants, stmt)
	if err != nil {
		return nil, model.ErrGetParticipant(err)
	}

	err = completeParticipants(db, tenant, participants)
	if err != nil {
		return nil, model.ErrGetParticipant(err)
	}

	//for i, _ := range participants {
	//	err = completeParticipant(db, participants[i])
	//	if err != nil {
	//		log.WithField("tenant", tenant).Errorf("Cannot fetch Participant correct: %s", err.Error())
	//	}
	//	if participants[i].MeteringPoint == nil {
	//		participants[i].MeteringPoint = make([]*model.MeteringPoint, 0)
	//	}
	//}

	return participants, nil
}

func queryParticipant(db *sqlx.DB, participantId string) (*model.EegParticipant, error) {
	var participant model.EegParticipant = model.EegParticipant{}

	sql, _, err := buildParticipantQueryStmt().
		Where(goqu.Ex{"base.participant.id": participantId}).
		ToSQL()

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

func buildParticipantQueryStmt() *goqu.SelectDataset {
	billingAddrStmt := pgDialect.From("base.address").Select(
		goqu.C("participant_id"), &model.Address{}).Where(goqu.Ex{"type": "BILLING"})
	residentAddStmt := pgDialect.From("base.address").
		Select(goqu.C("participant_id"), &model.Address{}).Where(goqu.Ex{"type": "RESIDENCE"})
	bankAccountStmt := pgDialect.From("base.bankaccount").
		Select(goqu.C("participant_id"), &model.BankInfo{})
	contactInfoStmt := pgDialect.From("base.contactdetail").Select(goqu.C("participant_id"), &model.ContactInfo{})

	return pgDialect.From(TABLE_PARTICIPANT).
		LeftJoin(billingAddrStmt.As("billingAddress"), goqu.On(goqu.Ex{"participant.id": goqu.I("billingAddress.participant_id")})).   /*.As("billingAddress")*/
		LeftJoin(residentAddStmt.As("residentAddress"), goqu.On(goqu.Ex{"participant.id": goqu.I("residentAddress.participant_id")})). /*.As("billingAddress")*/
		LeftJoin(bankAccountStmt.As("accountInfo"), goqu.On(goqu.Ex{"participant.id": goqu.I("accountInfo.participant_id")})).
		LeftJoin(contactInfoStmt.As("contact"), goqu.On(goqu.Ex{"participant.id": goqu.I("contact.participant_id")})).Select(&model.EegParticipant{})
}

func updateParticipant(db *sqlx.DB, tenant, user string, participant *model.EegParticipant) error {

	updateValues := struct {
		model.EegParticipantBase
		LastModifiedBy string         `db:"lastModifiedBy"`
		LastModifiedAt civil.DateTime `db:"lastModifiedDate"`
	}{
		participant.EegParticipantBase,
		user,
		civil.Now(),
	}

	sql, _, _ := goqu.Update("base.participant").
		Set(updateValues).
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

//type ParticipantWithMeta struct {
//	model.EegParticipantBase
//	Tenant           string         `db:"tenant"`
//	CreatedBy        string         `db:"createdBy"`
//	LastmodifiedBy   string         `db:"lastModifiedBy"`
//	LastmodifiedDate civil.DateTime `db:"lastModifiedDate"`
//}

// RegisterParticipant func RegisterParticipant(dbConn OpenDbXConnection, tenant, username string, participant *model.EegParticipant) error {
func registerParticipant(tx *sqlx.Tx, tenant, username string, participant *model.EegParticipant) error {
	participant.Status = model.PENDING
	participant.Id = uuid.NewUUID()
	//participant.ParticipantSince = time.Now()
	participant.CreatedBy = username
	return saveParticipant(tx, tenant, username, participant, ImportMeteringPoints)
}

// ImportParticipant func ImportParticipant(dbConn OpenDbXConnection, tenant, username string, participant *model.EegParticipant) error {
func importParticipant(tx *sqlx.Tx, tenant, username string, participant *model.EegParticipant) error {

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

func confirmParticipant(db *sqlx.DB, username, participantId string) error {

	sql, _, err := pgDialect.Update(TABLE_PARTICIPANT).
		Set(goqu.Ex{"status": model.ACTIVE, "lastModifiedDate": civil.Now(), "lastModifiedBy": username}).
		Where(goqu.C("id").Eq(participantId)).
		ToSQL()
	if err != nil {
		log.WithField("SQL", "CREATESTMT").WithError(err)
		return model.ErrCompleteParticipant(err)
	}

	//_, err := db.Exec("UPDATE base.participant SET status = 'ACTIVE', \"lastModifiedDate\" = 'now()', \"lastModifiedBy\" = $1 WHERE id = $2", username, participantId)
	_, err = db.Exec(sql)
	if err != nil {
		log.WithField("SQL", "UPDATE").WithError(err).Error(sql)
		return model.ErrCompleteParticipant(err)
	}
	return nil
}

func deleteParticipant(db *sqlx.DB, participantId string) error {
	stmt, _, err := pgDialect.Delete(TABLE_PARTICIPANT).
		Where(goqu.Ex{"id": participantId}).ToSQL()
	if err != nil {
		return model.ErrDeleteParticipant(err)
	}
	_, err = db.Exec(stmt)
	if err != nil {
		return model.ErrDeleteParticipant(err)
	}
	return nil
}

func saveParticipant(tx *sqlx.Tx, tenant, username string, participant *model.EegParticipant,
	registerMeteringPointsFunc func(*sqlx.Tx, string, string, string, []*model.MeteringPoint) error) error {

	participant.ParticipantSince = civil.NullDate{civil.Today(), true}
	registeringParticipant := struct {
		model.EegParticipantBase
		Tenant           string         `db:"tenant"`
		CreatedBy        string         `db:"createdBy"`
		LastmodifiedBy   string         `db:"lastModifiedBy"`
		LastmodifiedDate civil.DateTime `db:"lastModifiedDate"`
	}{
		participant.EegParticipantBase, tenant, username, username, civil.Now(),
	}

	//if participant.ParticipantSince.IsZero() {
	//	participant.ParticipantSince = time.Now()
	//}

	participantId := ""
	sql, _, _ := pgDialect.Insert(TABLE_PARTICIPANT).Rows(registeringParticipant).Returning("id").ToSQL()
	err := tx.QueryRow(sql).Scan(&participantId)
	if err != nil {
		return model.ErrRegisterParticipant(err)
	}

	extra := map[string]interface{}{"participant_id": participantId}

	sql, _, _ = pgDialect.Insert("base.contactdetail").Rows(toRecord(participant.Contact, extra)).ToSQL()
	_, err = tx.Exec(sql)
	if err != nil {
		return model.ErrRegisterParticipant(err)
	}

	sql, _, _ = pgDialect.Insert("base.bankaccount").Rows(toRecord(participant.BankAccount, extra)).ToSQL()
	_, err = tx.Exec(sql)
	if err != nil {
		return model.ErrRegisterParticipant(err)
	}

	sql, _, err = pgDialect.Insert("base.address").Rows(
		toRecord(participant.BillingAddress, extra),
		toRecord(participant.ResidentAddress, extra),
	).ToSQL()

	if err != nil {
		log.WithField("STMT", "INSERT").WithError(err).Error(sql)
		return model.ErrRegisterParticipant(err)
	}
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

//func decodeField(result interface{}, fields map[string]interface{}) (interface{}, error) {
//	cfg := &mapstructure.DecoderConfig{
//		Result:     result,
//		DecodeHook: StringToNullStringHookFunc,
//	}
//	decoder, err := mapstructure.NewDecoder(cfg)
//	if err != nil {
//		return nil, err
//	}
//	err = decoder.Decode(fields)
//	if err != nil {
//		return nil, err
//	}
//
//	return result, nil
//}

func updateParticipantPartial(db *sqlx.DB, participantId, name string, value interface{}) error {

	var stmt *goqu.UpdateDataset
	var sql string
	var updateValues interface{}
	var err error

	fields := map[string]interface{}{}

	names := strings.Split(name, ".")
	if len(names) == 2 {

		fields[names[1]] = value
		switch names[0] {
		case "billingAddress":
			var result model.Address
			updateValues, err = buildRecordMap(&result, fields)
			if err != nil {
				return err
			}
			stmt = pgDialect.Update("base.address").
				Where(goqu.Ex{"participant_id": goqu.V(participantId)}, goqu.Ex{"type": goqu.V("BILLING")})
		case "residentAddress":
			var result model.Address
			updateValues, err = buildRecordMap(&result, fields)
			if err != nil {
				return err
			}
			stmt = pgDialect.Update("base.address").
				Where(goqu.Ex{"participant_id": goqu.V(participantId)}, goqu.Ex{"type": goqu.V("RESIDENCE")})
		case "contact":
			var result model.ContactInfo
			updateValues, err = buildRecordMap(&result, fields)
			if err != nil {
				return err
			}
			stmt = pgDialect.Update("base.contactdetail").
				Where(goqu.Ex{"participant_id": goqu.V(participantId)})
		case "accountInfo":
			var result model.BankInfo
			updateValues, err = buildRecordMap(&result, fields)
			if err != nil {
				return err
			}

			stmt = pgDialect.Update("base.bankaccount").
				Where(goqu.Ex{"participant_id": goqu.V(participantId)})
		default:
			return model.ErrUpdateParticipant(errors.New(fmt.Sprintf("Can not update structure of %s", name)))
		}

		sql, _, err = stmt.Set(updateValues).ToSQL()

	} else if len(names) == 1 {
		var result model.EegParticipantBase
		fields[names[0]] = value
		if names[0] == "businessRole" && value == "EEG_BUSINESS" {
			fields["lastname"] = ""
			fields["titleBefore"] = ""
			fields["titleAfter"] = ""
		}
		updateValues, err = buildRecordMap(&result, fields)
		if err != nil {
			return err
		}

		sql, _, err = pgDialect.Update("base.participant").Set(updateValues).
			Where(goqu.Ex{"id": goqu.V(participantId)}).ToSQL()
	} else {
		return model.ErrUpdateParticipant(errors.New(fmt.Sprintf("Can not update structure of %s", name)))
	}

	res, err := db.Exec(sql)
	if err == nil {
		if rows, err := res.RowsAffected(); rows == 0 || err != nil {
			err = insertParticipantPartial(db, participantId, name, value)
			if err != nil {
				return err
			}
		}
		return nil
	} else {
		log.WithError(err).Errorf("Update partial participant %s", sql)
		return model.ErrUpdateParticipant(err)
	}
}

func insertParticipantPartial(db *sqlx.DB, participantId, name string, value interface{}) error {

	var stmt *goqu.InsertDataset
	var sql string
	fields := map[string]interface{}{}

	names := strings.Split(name, ".")
	if len(names) == 2 {
		fields["participant_id"] = participantId
		fields[names[1]] = value

		switch names[0] {
		case "billingAddress":
			var result model.Address
			fields["type"] = "BILLING"
			insertValues, err := buildRecordMap(&result, fields)
			if err != nil {
				return err
			}
			stmt = pgDialect.Insert("base.address").Rows(insertValues)
		case "residentAddress":
			var result model.Address
			fields["type"] = "RESIDENCE"
			insertValues, err := buildRecordMap(&result, fields)
			if err != nil {
				return err
			}
			stmt = pgDialect.Insert("base.address").Rows(insertValues)
		case "contact":
			var result model.ContactInfo
			insertValues, err := buildRecordMap(&result, fields)
			if err != nil {
				return err
			}
			stmt = pgDialect.Insert("base.contactdetail").Rows(insertValues)
		case "accountInfo":
			result := struct {
				model.BankInfo
				ParticipantId string `json:"participant_id" db:"participant_id"`
			}{}
			insertValues, err := buildRecordMap(&result, fields)
			if err != nil {
				return err
			}
			stmt = pgDialect.Insert("base.bankaccount").Rows(insertValues)
		default:
			return model.ErrInsertParticipant(errors.New(fmt.Sprintf("Can not update structure of %s", name)))
		}
		sql, _, _ = stmt.ToSQL()
	} else {
		return model.ErrInsertParticipant(errors.New(fmt.Sprintf("Can not update structure of %s", name)))
	}

	_, err := db.Exec(sql)
	if err != nil {
		log.WithError(err).Errorf("Insert partial participant %s", sql)
		return model.ErrInsertParticipant(err)
	}
	return nil
}

func buildGetParticipantQuery() *goqu.SelectDataset {
	stateStmt := pgDialect.From("base.meteringpoint").
		Select(
			goqu.C("activesince"),
			goqu.C("inactivesince"),
			goqu.C("flag"),
			goqu.C("metering_point_id").As("mid"),
			goqu.C("participant_id").As("pid"))

	partFactStmt := pgDialect.From(TABLE_PARTITION_FACT_VIEW).
		Select(
			goqu.C("partFact"),
			goqu.C("metering_point_id").As("mpfmid"),
			goqu.C("participant_id").As("mpfpid"))

	stmt := pgDialect.From("base.meteringpoint", stateStmt.As("state"), partFactStmt.As("mpfpF1")).Select(&model.MeteringPoint{}).
		Where(
			//goqu.C("tenant").Table("meteringpoint").Schema("base").Eq(tenant),
			goqu.C("mid").Eq(goqu.C("metering_point_id")),
			goqu.C("pid").Eq(goqu.C("participant_id")),
			goqu.C("mpfmid").Eq(goqu.C("metering_point_id")),
			goqu.C("mpfpid").Eq(goqu.C("participant_id")),
		)

	return stmt
}

func completeParticipants(db *sqlx.DB, tenant string, participants []*model.EegParticipant) error {
	//stateStmt := pgDialect.From("base.meteringpoint").
	//	Select(
	//		goqu.C("activesince"),
	//		goqu.C("inactivesince"),
	//		goqu.C("flag"),
	//		goqu.C("metering_point_id").As("mid"),
	//		goqu.C("participant_id").As("pid"))
	//
	//partFactStmt := pgDialect.From(TABLE_PARTITION_FACT_VIEW).
	//	Select(
	//		goqu.C("partFact"),
	//		goqu.C("metering_point_id").As("mpfmid"),
	//		goqu.C("participant_id").As("mpfpid"))
	//
	//stmt, _, err := pgDialect.From("base.meteringpoint", stateStmt.As("state"), partFactStmt.As("mpfpF1")).Select(&model.MeteringPoint{}).
	//	Where(
	//		goqu.C("tenant").Table("meteringpoint").Schema("base").Eq(tenant),
	//		goqu.C("mid").Eq(goqu.C("metering_point_id")),
	//		goqu.C("pid").Eq(goqu.C("participant_id")),
	//		goqu.C("mpfmid").Eq(goqu.C("metering_point_id")),
	//		goqu.C("mpfpid").Eq(goqu.C("participant_id")),
	//	).ToSQL()

	stmt, _, err := buildGetParticipantQuery().
		Where(goqu.C("tenant").Table("meteringpoint").Schema("base").Eq(tenant)).ToSQL()

	if err != nil {
		return model.ErrCompleteParticipant(err)
	}

	meteringPoints := []model.MeteringPoint{}

	err = db.Select(&meteringPoints, stmt)
	if err != nil && !errors.Is(err, dbsql.ErrNoRows) {
		return model.ErrCompleteParticipant(err)
	}

	meteringPointsMap := make(map[string][]*model.MeteringPoint)
	for i, meteringPoint := range meteringPoints {
		meteringPointsMap[meteringPoint.ParticipantId] = append(meteringPointsMap[meteringPoint.ParticipantId], &meteringPoints[i])
	}

	for i, participant := range participants {
		m, ok := meteringPointsMap[participant.Id.String()]
		if !ok {
			participants[i].MeteringPoint = []*model.MeteringPoint{}
		} else {
			participants[i].MeteringPoint = m
		}
	}
	return nil
}

func completeParticipant(db *sqlx.DB, participant *model.EegParticipant) error {
	participantId := participant.Id.String()
	//stateStmt := pgDialect.From("base.meteringpoint").
	//	Select(
	//		goqu.C("activesince"),
	//		goqu.C("inactivesince"),
	//		goqu.C("flag"),
	//		goqu.C("metering_point_id").As("mid"),
	//		goqu.C("participant_id").As("pid"))
	//
	//partFactStmt := pgDialect.From(TABLE_PARTITION_FACT_VIEW).
	//	Select(
	//		goqu.C("partFact"),
	//		goqu.C("metering_point_id").As("mpfmid"),
	//		goqu.C("participant_id").As("mpfpid"))
	//
	//stmt, _, err := pgDialect.From("base.meteringpoint", stateStmt.As("state"), partFactStmt.As("mpfpF1")).Select(&participant.MeteringPoint).
	//	Where(
	//		goqu.C("participant_id").Table("meteringpoint").Schema("base").Eq(participantId),
	//		goqu.C("mid").Eq(goqu.C("metering_point_id")),
	//		goqu.C("pid").Eq(goqu.C("participant_id")),
	//		goqu.C("mpfmid").Eq(goqu.C("metering_point_id")),
	//		goqu.C("mpfpid").Eq(goqu.C("participant_id")),
	//	).ToSQL()
	stmt, _, err := buildGetParticipantQuery().Where(
		goqu.C("participant_id").Table("meteringpoint").Schema("base").Eq(participantId)).ToSQL()
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

func findParticipantByMeteringPoint(db *sqlx.DB, tenant, meteringPoint string) (*model.EegParticipant, error) {

	participant := model.EegParticipant{}

	participantIdStmt := pgDialect.From("base.meteringpoint").Select("participant_id").
		Where(
			goqu.C("metering_point_id").Eq(meteringPoint),
			goqu.C("tenant").Eq(tenant),
			goqu.C("flag").Eq(model.F_ASSIGNED),
		)

	stmt, _, err := buildParticipantQueryStmt().Where(goqu.C("id").Eq(participantIdStmt)).ToSQL()
	if err != nil {
		log.WithField("SQL", "SELECT").Infof("Create Stmt: %+v, %+v", participant, participantIdStmt)
		return nil, err
	}

	err = db.Get(&participant, stmt)
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

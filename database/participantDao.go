package database

import (
	"at.ourproject/vfeeg-backend/model"
	dbsql "database/sql"
	"github.com/doug-martin/goqu/v9"
)

func GetParticipant(tenant string) ([]model.EegParticipant, error) {
	var participants []model.EegParticipant = []model.EegParticipant{}
	db, err := GetDBXConnection()
	if err != nil {
		return []model.EegParticipant{}, err
	}
	defer db.Close()

	sql, _, err := pgDialect.From("base.participant").Select(&participants).Where(goqu.C("tenant").Eq(tenant)).ToSQL()
	if err != nil {
		return []model.EegParticipant{}, err
	}

	err = db.Select(&participants, sql)
	if err != nil {
		return []model.EegParticipant{}, err
	}

	for i, p := range participants {
		sql, _, err = pgDialect.From("base.contactdetail").Select(&p.Contact).Where(goqu.C("participant_id").Eq(p.Id.String())).ToSQL()
		if err != nil {
			return []model.EegParticipant{}, err
		}
		err = db.Get(&(participants[i].Contact), sql)
		if err != nil && err != dbsql.ErrNoRows {
			return []model.EegParticipant{}, err
		}

		sql, _, err = pgDialect.From("base.bankaccount").Select(&p.BankAccount).Where(goqu.C("participant_id").Eq(p.Id.String())).ToSQL()
		if err != nil {
			return []model.EegParticipant{}, err
		}
		err = db.Get(&(participants[i].BankAccount), sql)
		if err != nil && err != dbsql.ErrNoRows {
			return []model.EegParticipant{}, err
		}

		sql, _, err = pgDialect.From("base.address").Select(&p.BillingAddress).
			Where(goqu.C("participant_id").Eq(p.Id.String()), goqu.C("type").Eq("BILLING")).ToSQL()
		if err != nil {
			return []model.EegParticipant{}, err
		}
		err = db.Get(&(participants[i].BillingAddress), sql)
		if err != nil && err != dbsql.ErrNoRows {
			return []model.EegParticipant{}, err
		}

		sql, _, err = pgDialect.From("base.address").Select(&p.ResidentAddress).
			Where(goqu.C("participant_id").Eq(p.Id.String()), goqu.C("type").Eq("RESIDENCE")).ToSQL()
		if err != nil {
			return []model.EegParticipant{}, err
		}
		//fmt.Printf("SQL: %+v\n", sql)
		err = db.Get(&(participants[i].ResidentAddress), sql)
		if err != nil && err != dbsql.ErrNoRows {
			return []model.EegParticipant{}, err
		}
		//fmt.Printf("ADDRESS: %+v\n", p.ResidentAddress)

		sql, _, err = pgDialect.From("base.meteringpoint").Select(&p.MeteringPoint).
			Where(goqu.C("participant_id").Eq(p.Id.String())).ToSQL()
		if err != nil {
			return []model.EegParticipant{}, err
		}
		err = db.Select(&(participants[i].MeteringPoint), sql)
		if err != nil && err != dbsql.ErrNoRows {
			return []model.EegParticipant{}, err
		}
	}

	return participants, nil
}

func UpdateParticipant(tenant, participantId string, participant map[string]interface{}) error {
	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	sql, _, _ := goqu.Update("base.participant").
		Set(participant).
		Where(goqu.Ex{
			"tenant": goqu.Op{"eq": tenant},
			"id":     goqu.Op{"eq": participantId},
		}).
		ToSQL()
	_, err = db.Exec(sql)

	return err
}

type ParticipantWithMeta struct {
	*model.EegParticipant
	Tenant         string
	CreatedBy      string
	LastmodifiedBy string
}

func RegisterParticipant(tenant, username string, participant *model.EegParticipant) error {
	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	participant.Status = model.PENDING

	registeredParticipant := ParticipantWithMeta{
		participant, tenant, username, username,
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	participantId := ""
	sql, _, _ := pgDialect.Insert("base.participant").Rows(registeredParticipant).Returning("id").ToSQL()
	err = tx.QueryRow(sql).Scan(&participantId)
	if err != nil {
		return err
	}

	contactEntry := struct {
		model.ContactInfo
		Participant_id string
	}{participant.Contact, participantId}
	sql, _, _ = pgDialect.Insert("base.contactdetail").Rows(contactEntry).ToSQL()
	_, err = tx.Exec(sql)
	if err != nil {
		return err
	}

	bankInfoEntry := struct {
		model.BankInfo
		Participant_id string
	}{participant.BankAccount, participantId}
	sql, _, _ = pgDialect.Insert("base.bankaccount").Rows(bankInfoEntry).ToSQL()
	_, err = tx.Exec(sql)
	if err != nil {
		return err
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
		return err
	}

	err = RegisterMeteringPoints(tx, tenant, participantId, participant.MeteringPoint)
	if err != nil {
		return err
	}
	return tx.Commit()
}

//func SelectParticipant(tenant, participantId string) (*model.EegParticipant, error) {
//
//}

func SaveNotification(tenant string, notification string, msgType, role string) error {
	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO base.notification (tenant, notification, date, type, role) VALUES ($1, $2, NOW(), $3, $4)", tenant, notification, msgType, role)
	return err
}

func InsertParticipant(tenant string, participant *model.EegParticipant) error {
	return nil
}

package database

import (
	"at.ourproject/vfeeg-backend/model"
	"database/sql"
	"github.com/doug-martin/goqu/v9"
	"log"
)

const TABLE_METERINGPOINT = "base.meteringpoint"

type meteringEntryType struct {
	model.MeteringPoint
	Participant_id string
	Tenant         string
}

func RegisterMeteringPoints(tx *sql.Tx, tenant, participantId string, point []model.MeteringPoint) error {
	meteringEntry := []meteringEntryType{}
	for _, p := range point {
		p.Status = model.PENDING
		meteringEntry = append(meteringEntry, meteringEntryType{p, participantId, tenant})
	}
	return saveMeteringPoint(tx, meteringEntry)
}

func ImportMeteringPoints(tx *sql.Tx, tenant, participantId string, point []model.MeteringPoint) error {
	meteringEntry := []meteringEntryType{}
	for _, p := range point {
		meteringEntry = append(meteringEntry, meteringEntryType{p, participantId, tenant})
	}
	return saveMeteringPoint(tx, meteringEntry)
}

func saveMeteringPoint(tx *sql.Tx, meteringEntry []meteringEntryType) error {
	statement, _, _ := pgDialect.Insert(TABLE_METERINGPOINT).Rows(meteringEntry).ToSQL()
	log.Printf("Register Meterings: %+v", statement)
	_, err := tx.Exec(statement)
	return err
}

func RegisterMeteringPoint(tenant, participantId string, point *model.MeteringPoint) error {
	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	type meteringEntryType struct {
		*model.MeteringPoint
		ParticipantId string `db:"participant_id"`
		Tenant        string
	}
	meteringEntry := meteringEntryType{point, participantId, tenant}

	statement, _, _ := pgDialect.Insert(TABLE_METERINGPOINT).Rows(meteringEntry).ToSQL()
	_, err = db.Exec(statement)
	return err
}

func UpdateMeteringPoint(tenant, participantId, meterId string, meteringPoint *model.MeteringPoint) error {
	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()

	statement, _, _ := goqu.Update(TABLE_METERINGPOINT).
		Set(meteringPoint).
		Where(goqu.Ex{
			"tenant":            goqu.Op{"eq": tenant},
			"metering_point_id": goqu.Op{"eq": meterId},
			"participant_id":    goqu.Op{"eq": participantId},
		}).
		ToSQL()
	_, err = db.Exec(statement)

	return err
}

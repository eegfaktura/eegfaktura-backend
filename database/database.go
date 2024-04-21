package database

import (
	"at.ourproject/vfeeg-backend/model"
	"database/sql"
	"errors"
	"fmt"
	"github.com/doug-martin/goqu/v9"
	_ "github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"time"
)

type OpenDbXConnection func() (*sqlx.DB, error)

var (
	ErrTariffUtilized = errors.New("Tariff is currently used")
)

//func init() {
//	loc, err := time.LoadLocation("Europe/Berlin")
//	if err != nil {
//		panic(err)
//	}
//
//	goqu.SetTimeLocation(loc)
//}

var OpenTransaction = func() (*sqlx.Tx, error) {
	return GetTx()
}

func GetTx() (*sqlx.Tx, error) {
	db, err := ConnectToDatabase()
	if err != nil {
		return nil, err
	}

	tx, err := db.Beginx()
	if err != nil {
		log.Error(err)
		return nil, err
	}
	return tx, nil
}

func GetDBConnection() (*sql.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		viper.GetString("database.host"), viper.GetInt("database.port"), viper.GetString("database.user"),
		viper.GetString("database.password"), viper.GetString("database.dbname"))
	return sql.Open("postgres", psqlInfo)
}

var ConnectToDatabase = func() (*sqlx.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		viper.GetString("database.host"), viper.GetInt("database.port"), viper.GetString("database.user"),
		viper.GetString("database.password"), viper.GetString("database.dbname"))
	return sqlx.Open("postgres", psqlInfo)
}

//func GetDBXConnection() (*sqlx.DB, error) {
//	return ConnectToDatabase()
//}

var pgDialect = goqu.Dialect("postgres")

func GetTariff(db *sqlx.DB, tenant string) ([]model.Tariff, error) {
	var tariff []model.Tariff
	err := db.Select(&tariff, `SELECT id, name, "billingPeriod", "useVat", "vatInPercent", "accountNetAmount", "accountGrossAmount", "participantFee", "baseFee", "businessNr", version, type, "centPerKWh", discount, "freeKWh", "meteringPointFee", "useMeteringPointFee", "vatSupplementaryText" `+
		`FROM base.activetariff WHERE tenant = $1`, tenant)
	if errors.Is(err, sql.ErrNoRows) || tariff == nil {
		return []model.Tariff{}, nil
	}

	if err != nil {
		log.WithField("tenant", tenant).Errorf("Error Query Tariff! %s", err.Error())
		return tariff, model.ErrGetTariff(err)
	}
	return tariff, nil
}

func GetTariffNameMap(db *sqlx.DB, tenant string) (map[string]string, error) {
	tariffs, err := GetTariff(db, tenant)
	if err != nil {
		return nil, err
	}
	tariffMap := map[string]string{}
	for _, t := range tariffs {
		tariffMap[t.Id.String()] = t.Name
	}
	return tariffMap, nil
}

func ArchiveTariff(db *sqlx.DB, tenant string, id string) error {
	stmt, _, err := pgDialect.Select("id").From("base.participant").Where(goqu.Ex{"tariffId": id}).ToSQL()
	if err != nil {
		return model.ErrGetTariff(err)
	}
	_, err = db.Query(stmt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return model.ErrTariffUtilized(ErrTariffUtilized)
	}

	stmt, _, err = pgDialect.Select("metering_point_id").From("base.meteringpoint").Where(goqu.Ex{"tariff_id": id, "tenant": tenant}).ToSQL()
	if err != nil {
		return err
	}
	_, err = db.Query(stmt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return model.ErrTariffUtilized(ErrTariffUtilized)
	}

	_, err = db.Exec("UPDATE base.tariff SET status = 'ARCHIVED', \"lastModifiedDate\" = 'now()' WHERE tenant = $1 AND id = $2", tenant, id)
	if err != nil {
		return model.ErrUpdateTariff(err)
	}
	return nil
}

func AddTariff(db *sqlx.DB, tenant, user string, tariff *model.Tariff) error {

	if len(tariff.Id.String()) == 0 {
		tariff.Id = uuid.NewUUID()
	} else {
		tariff.Version = tariff.Version + 1
	}

	type updateType struct {
		Tenant           string    `json:"tenant" db:"tenant"`
		CreatedDate      time.Time `goqu:"omitempty" db:"createdDate"`
		LastModifiedDate time.Time `goqu:"omitempty" db:"lastModifiedDate"`
		CreatedBy        string    `db:"createdBy"`
		*model.Tariff
	}
	update := updateType{tenant, time.Now(), time.Now(), user, tariff}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	//defer func() { _ = tx.Rollback() }()
	defer func() {
		switch err {
		case nil:
			err = tx.Commit()
		default:
			err = tx.Rollback()
		}
	}()
	var stmt string
	stmt, _, err = goqu.Insert("base.tariff").Rows(update).ToSQL()
	if err != nil {
		return model.ErrUpdateTariff(err)
	}
	_, err = tx.Exec(stmt)
	if err != nil {
		log.WithField("SQL", "INSERT").Errorf("Stmt: %v", stmt)
		return model.ErrUpdateTariff(err)
	}

	if tariff.Version > 0 {
		stmt, _, err = goqu.Update("base.tariff").Set(
			map[string]interface{}{"inactiveSince": time.Now(), "lastModifiedDate": time.Now()}).Where(goqu.Ex{
			"version": tariff.Version - 1,
			"id":      tariff.Id.String(),
		}).ToSQL()
		if err != nil {
			log.WithField("tenant", tenant).Errorf("Update previous entry: %v", err)
			return model.ErrUpdateTariff(err)
		}
		_, err = tx.Exec(stmt)
	}
	if err != nil {
		return model.ErrUpdateTariff(err)
	}
	return err
}

func UpdateTariff(dbConn OpenDbXConnection, tenant string, tariff *model.Tariff) error {
	db, err := dbConn()
	if err != nil {
		return err
	}
	defer db.Close()

	if len(tariff.Id.NodeID()) == 0 {
		tariff.Id = uuid.NewUUID()
	} else {
		tariff.Version = tariff.Version + 1
	}

	type updateType struct {
		Tenant string
		model.Tariff
	}

	update := updateType{tenant, *tariff}

	log.Debugf("Tarrif: %+v\n", update)

	stmt, _, err := goqu.Update("base.tariff").Set(&update).Where(goqu.C("tenant").Eq(tenant)).ToSQL()
	_, err = db.Exec(stmt)
	if err != nil {
		log.WithField("SQL", "UPDATE").Errorf("Stmt: %v", stmt)
	}

	//_, err = db.NamedExec(
	//	"UPDATE base.tariff SET \"billingPeriod\"=:billingperiod, \"useVat\"=:usevat, \"vatInPercent\"=:vatinpercent, \"accountNetAmount\"=:accountnetamount, \"accountGrossAmount\"=:accountgrossamount, \"participantFee\"=:participantfee, \"baseFee\"=:basefee, discount=:discount, \"businessNr\"=:businessnr, \"centPerKWh\"=:centperkwh, \"freeKWh\" = :freekwh, \"createdBy\"=:createdby, version=:version WHERE id = :id", &update)

	return err
}

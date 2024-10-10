package database

import (
	"at.ourproject/vfeeg-backend/model"
	"database/sql"
	"errors"
	"fmt"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type OpenDbXConnection func() (*sqlx.DB, error)

var (
	ErrTariffUtilized = errors.New("Tariff is currently used")
)

func GetDBConnection() (*sql.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		viper.GetString("database.host"), viper.GetInt("database.port"), viper.GetString("database.user"),
		viper.GetString("database.password"), viper.GetString("database.dbname"))
	return sql.Open("postgres", psqlInfo)
}

func GetDBXConnection() (*sqlx.DB, error) {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		viper.GetString("database.host"), viper.GetInt("database.port"), viper.GetString("database.user"),
		viper.GetString("database.password"), viper.GetString("database.dbname"))
	return sqlx.Open("postgres", psqlInfo)
}

var pgDialect = goqu.Dialect("postgres")

func GetTariff(tenant string) ([]model.Tariff, error) {

	db, err := GetDBXConnection()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	tariff := []model.Tariff{}
	err = db.Select(&tariff, `SELECT id, name, "billingPeriod", "useVat", "vatInPercent", "accountNetAmount", "accountGrossAmount", "participantFee", "baseFee", "businessNr", version, type, "centPerKWh", discount, "freeKWh" `+
		`FROM base.activetariff WHERE tenant = $1`, tenant)
	if err == sql.ErrNoRows {
		return []model.Tariff{}, nil
	}

	return tariff, err
}

func ArchiveTariff(dbConn OpenDbXConnection, tenant string, id string) error {

	db, err := dbConn()
	if err != nil {
		return err
	}
	defer db.Close()

	stmt, _, err := pgDialect.Select("id").From("base.participant").Where(goqu.Ex{"tariffId": id}).ToSQL()
	if err != nil {
		return err
	}
	_, err = db.Query(stmt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ErrTariffUtilized
	}

	stmt, _, err = pgDialect.Select("id").From("base.meteringpoint").Where(goqu.Ex{"tariffId": id, "tenant": tenant}).ToSQL()
	if err != nil {
		return err
	}
	_, err = db.Query(stmt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ErrTariffUtilized
	}

	_, err = db.Exec("UPDATE base.tariff SET status = 'ARCHIVED' WHERE tenant = $1 AND id = $2", tenant, id)
	return err
}

func AddTariff(dbConn OpenDbXConnection, tenant string, tariff *model.Tariff) error {
	db, err := dbConn()
	if err != nil {
		return err
	}
	defer db.Close()

	if len(tariff.Id.String()) == 0 {
		tariff.Id = uuid.NewUUID()
	} else {
		tariff.Version = tariff.Version + 1
	}

	type updateType struct {
		Tenant string `json:"tenant" db:"tenant"`
		*model.Tariff
	}

	update := updateType{tenant, tariff}
	log.Debugf("Insert new Tariff %+v\n", update)

	log.Debugf("Tarrif: %+v\n", update)

	sql, _, err := goqu.Insert("base.tariff").Rows(update).ToSQL()
	if err != nil {
		return err
	}
	fmt.Printf("Tariff Insert Statement: %s\n", sql)
	_, err = db.Exec(sql)

	//_, err = db.NamedExec(
	//	`INSERT INTO base.tariff (id, tenant, name, type, "billingPeriod", "useVat", "vatInPercent", "accountNetAmount", "accountGrossAmount", "participantFee", "baseFee", discount, "businessNr", "centPerKWh", "freeKWh", "createdBy", version) VALUES (:id, :tenant, :name, :type, :billingPeriod, :useVat, :vatInPercent, :accountNetAmount, :accountGrossAmount, :participantFee, :baseFee, :discount, :businessNr, :centPerKWh, :freeKWh, :tenant, :version)`, &update)

	return err
}

func UpdateTariff(tenant string, tariff *model.Tariff) error {
	db, err := GetDBXConnection()
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
	_, err = db.NamedExec(
		"UPDATE base.tariff SET \"billingPeriod\"=:billingperiod, \"useVat\"=:usevat, \"vatInPercent\"=:vatinpercent, \"accountNetAmount\"=:accountnetamount, \"accountGrossAmount\"=:accountgrossamount, \"participantFee\"=:participantfee, \"baseFee\"=:basefee, discount=:discount, \"businessNr\"=:businessnr, \"centPerKWh\"=:centperkwh, \"freeKWh\" = :freekwh, \"createdBy\"=:createdby, version=:version WHERE id = :id", &update)

	return err
}

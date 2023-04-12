package database

import (
	"at.ourproject/vfeeg-backend/model"
	"database/sql"
	"fmt"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"github.com/pborman/uuid"
	"github.com/spf13/viper"
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
	err = db.Select(&tariff, "SELECT id, name, billingperiod, usevat, vatinpercent, accountnetamount, accountgrossamount, participantfee, basefee, businessnr, version, type, centperKWH, discount, freeKwh "+
		"FROM base.activetariff WHERE tenant = $1", tenant)
	if err == sql.ErrNoRows {
		return []model.Tariff{}, nil
	}

	return tariff, err
}

func DeleteTariff(tenant string, id string) error {

	db, err := GetDBXConnection()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("DELETE FROM base.tariff WHERE tenant = $1 AND id = $2", tenant, id)
	return err
}

func AddTariff(tenant string, tariff *model.Tariff) error {
	db, err := GetDBXConnection()
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
		model.Tariff
	}

	update := updateType{tenant, *tariff}
	fmt.Printf("Insert new Tariff %+v\n", update)

	fmt.Printf("Tarrif: %+v\n", update)
	_, err = db.NamedExec(
		"INSERT INTO base.tariff (id, tenant, name, type, billingperiod, usevat, vatinpercent, accountnetamount, accountgrossamount, participantfee, basefee, discount, businessnr, centperkwh, freeKwh, createdby, version) VALUES (:id, :tenant, :name, :type, :billingperiod, :usevat, :vatinpercent, :accountnetamount, :accountgrossamount, :participantfee, :basefee, :discount, :businessnr, :centperkwh, :freekwh, :tenant, :version) ", &update)

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

	fmt.Printf("Tarrif: %+v\n", update)
	_, err = db.NamedExec(
		"UPDATE base.tariff SET billingperiod=:billingperiod, usevat=:usevat, vatinpercent=:vatinpercent, accountnetamount=:accountnetamount, accountgrossamount=:accountgrossamount, participantfee=:participantfee, basefee=:basefee, discount=:discount, businessnr=:businessnr, centperkwh=:centperkwh, freeKwh = :freekwh, createdby=:createdby, version=:version WHERE id = :id", &update)

	return err
}

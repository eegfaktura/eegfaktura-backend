package database

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var db struct {
	sync.Once
	Database
}

type sqlDatabase struct {
	db *sqlx.DB
}

type Database interface {
	Select(dest interface{}, query string, args ...interface{}) error
	CloseDB() error
	MigrateDB() error
	EegRepository
	ParticipantRepository
	MeteringPointRepository
	NotificationRepository
	ExcelRepository
	TariffRepository
}

func initDB(ctx context.Context) error {
	var err error

	sqlDB := sqlDatabase{}

	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		viper.GetString("database.host"), viper.GetInt("database.port"), viper.GetString("database.user"),
		viper.GetString("database.password"), viper.GetString("database.dbname"))
	sqlDB.db, err = sqlx.ConnectContext(ctx, "postgres", psqlInfo)
	if err != nil {
		return err
	}

	//if viper.GetString("db.sql.datasourcename") != "" {
	//	sqlDB.db, err = sqlx.ConnectContext(ctx, viper.GetString("db.drivername"), viper.GetString("db.sql.datasourcename"))
	//	if err != nil {
	//		return err
	//	}
	//} else {
	//	return errors.New("no datasourcename set")
	//}

	db.Database = &sqlDB

	return nil
}

// GetDB returns the current DB.
func GetDB(ctx context.Context) (Database, error) {
	var err error
	db.Do(func() {
		err = initDB(ctx)
	})
	if err != nil {
		return nil, err //errors.Wrap(err, "failed to initialize DB")
	}
	if db.Database == nil {
		return nil, errors.New("database was not initialized")
	}

	return db.Database, nil
}

func (db *sqlDatabase) CloseDB() error {
	log.Info("Closing database connection")
	if db.db != nil {
		return db.db.Close()
	}
	return errors.New("database was not initialized")
}

func (db *sqlDatabase) Select(dest interface{}, query string, args ...interface{}) error {
	return db.db.Select(dest, query, args...)
}

func (db *sqlDatabase) MigrateDB() error {
	log.Info("Start migration ...")

	dbDriver, err := postgres.WithInstance(db.db.DB, &postgres.Config{SchemaName: "base"})
	if err != nil {
		log.Fatal(err)
		return err
	}

	m, err := migrate.NewWithDatabaseInstance(
		"embed://",
		"postgres", dbDriver)
	if err != nil {
		log.Fatal(err)
		return err
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		log.Fatal(err)
		return err
	}
	return nil
}

package database

import (
	"database/sql"
	"errors"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	log "github.com/sirupsen/logrus"
)

// migrateDB applies the migrations of sourceURL to the database of dsn, on a connection of its own that is closed
// afterwards. It logs the result once: db.migrate.done with the version before and after and the number of
// migrations applied (0: no change), or the error.
func migrateDB(sourceURL, dsn string) error {
	log.Info("Start migration ...")
	src, err := source.Open(sourceURL)
	if err != nil {
		return migrateFailed(err)
	}
	conn, err := sql.Open("postgres", dsn)
	if err != nil {
		_ = src.Close()
		return migrateFailed(err)
	}
	driver, err := postgres.WithInstance(conn, &postgres.Config{SchemaName: "base"})
	if err != nil {
		_ = src.Close()
		_ = conn.Close()
		return migrateFailed(err)
	}
	m, err := migrate.NewWithInstance("migrations", src, "postgres", driver)
	if err != nil {
		_ = src.Close()
		_ = driver.Close()
		return migrateFailed(err)
	}
	defer m.Close()

	before, err := schemaVersion(m)
	if err != nil {
		return migrateFailed(err)
	}
	if err = m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return migrateFailed(err)
	}
	after, err := schemaVersion(m)
	if err != nil {
		return migrateFailed(err)
	}
	applied := appliedMigrations(src, before, after)
	message := "database migrations applied"
	if applied == 0 {
		message = "database schema unchanged (no_change)"
	}
	log.WithFields(log.Fields{
		"event.action":             "db.migrate.done",
		"event.outcome":            "success",
		"migration.version_before": before,
		"migration.version_after":  after,
		"migration.applied":        applied,
	}).Info(message)
	return nil
}

// schemaVersion is the version in base.schema_migrations, 0 for a database without one.
func schemaVersion(m *migrate.Migrate) (uint, error) {
	v, _, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, nil
	}
	return v, err
}

// appliedMigrations counts the migrations of src above before up to after.
func appliedMigrations(src source.Driver, before, after uint) int {
	v, err := src.First()
	if before > 0 {
		v, err = src.Next(before)
	}
	n := 0
	for ; err == nil && v <= after; v, err = src.Next(v) {
		n++
	}
	return n
}

func migrateFailed(err error) error {
	log.WithField("event.action", "db.migrate.failed").WithField("event.outcome", "failure").WithError(err).
		Error("database migration failed")
	return err
}

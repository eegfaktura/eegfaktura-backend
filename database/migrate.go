package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source"
	log "github.com/sirupsen/logrus"
)

// migrationLockTimeout bounds every lock wait of the migrations (eegfaktura-dev#54 #1): the backend runs one replica
// (Recreate) and migrates before it serves, so a migration waiting behind a long transaction or a pg_dump keeps the
// API down. With the bound it fails, the pod exits and restarts; a failed migration file leaves the version dirty
// (README, "Failed migration (dirty)").
const migrationLockTimeout = 60 * time.Second

// migrateDB applies the migrations of sourceURL to the database of dsn, on a connection of its own (lock_timeout
// lockTimeout, also for the migration lock) that is closed afterwards. It logs the result once: db.migrate.done with
// the version before and after and the number of migrations applied (0: no change), or the error.
func migrateDB(sourceURL, dsn string, lockTimeout time.Duration) error {
	log.Info("Start migration ...")
	src, err := source.Open(sourceURL)
	if err != nil {
		return migrateFailed(err)
	}
	// lib/pq passes a parameter it does not know to the server as a run-time setting of the session
	conn, err := sql.Open("postgres", fmt.Sprintf("%s lock_timeout=%d", dsn, lockTimeout.Milliseconds()))
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
	message := "database migration failed"
	// SQLSTATE 55P03; golang-migrate keeps only the text of the driver's error
	if strings.Contains(err.Error(), "lock timeout") {
		message = "database migration failed: a lock wait exceeded lock_timeout, check pg_stat_activity for long " +
			"transactions or a running pg_dump"
	}
	log.WithField("event.action", "db.migrate.failed").WithField("event.outcome", "failure").WithError(err).
		Error(message)
	return err
}

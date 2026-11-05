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
		var dirty migrate.ErrDirty
		if !errors.As(err, &dirty) {
			// a file that failed leaves its version dirty
			if v, d, verr := m.Version(); verr == nil && d {
				dirty.Version = int(v)
			}
		}
		if dirty.Version != 0 {
			return migrateDirty(err, dirty.Version)
		}
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

// migrateDirty logs a failure that leaves the version dirty with the pointer to the recovery: golang-migrate's own
// advice "force version" names a CLI that is not in the image and is wrong for an invalid index.
func migrateDirty(err error, version int) error {
	log.WithFields(log.Fields{"event.action": "db.migrate.dirty", "event.outcome": "failure",
		"migration.version": version}).WithError(err).
		Error(fmt.Sprintf("database migration %d is dirty (started, not finished)%s: recover by hand as in the README "+
			"of eegfaktura-backend, section \"Failed migration (dirty)\"; never only reset the version", version, lockHint(err)))
	return err
}

func migrateFailed(err error) error {
	log.WithField("event.action", "db.migrate.failed").WithField("event.outcome", "failure").WithError(err).
		Error("database migration failed" + lockHint(err))
	return err
}

// lockHint names the cause of a lock timeout: PostgreSQL's lock_timeout (SQLSTATE 55P03; golang-migrate keeps only
// the text of the driver's error) or golang-migrate's own wait for its migration lock (ErrLockTimeout, 15 s).
func lockHint(err error) string {
	if errors.Is(err, migrate.ErrLockTimeout) || strings.Contains(err.Error(), "lock timeout") {
		return ": a lock wait took too long, check pg_stat_activity for long transactions or a running pg_dump"
	}
	return ""
}

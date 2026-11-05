package database

import (
	"bytes"
	"io"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/database/dbtest"
	"at.ourproject/vfeeg-backend/logging"
	"at.ourproject/vfeeg-backend/logging/logtest"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// emptyDB creates the database name with the empty schema base (as in the zones) and returns its connection string and a connection to it;
// dropped when the test ends.
func emptyDB(t *testing.T, name string) (string, *sqlx.DB) {
	t.Helper()
	_, err := testDB.DbInstance.Exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`)
	require.NoError(t, err)
	_, err = testDB.DbInstance.Exec(`CREATE DATABASE ` + name)
	require.NoError(t, err)
	dsn := strings.Replace(testDB.ConnectionString, "dbname="+dbtest.DbName, "dbname="+name, 1)
	dbx, err := sqlx.Connect("postgres", dsn)
	require.NoError(t, err)
	_, err = dbx.Exec(`CREATE SCHEMA base`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = dbx.Close()
		_, _ = testDB.DbInstance.Exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`)
	})
	return dsn, dbx
}

// migrationsURL is the file source of the migrations directory.
func migrationsURL(t *testing.T) string {
	t.Helper()
	_, path, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return "file:" + filepath.Dir(path) + "/../migrations"
}

// migrationVersions returns the versions of the up migrations, ascending, as numbers in a JSON log line.
func migrationVersions(t *testing.T) []float64 {
	t.Helper()
	_, path, _, _ := runtime.Caller(0)
	files, err := filepath.Glob(filepath.Join(filepath.Dir(path), "..", "migrations", "*.up.sql"))
	require.NoError(t, err)
	versions := make([]float64, len(files))
	for i, f := range files {
		versions[i], err = strconv.ParseFloat(strings.SplitN(filepath.Base(f), "_", 2)[0], 64)
		require.NoError(t, err)
	}
	return versions
}

func migrateLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	return logtest.Capture(t, func(w io.Writer) {
		logging.Setup(w, logging.Config{Level: slog.LevelInfo, JSON: true, Version: "test"})
	})
}

// lineOf returns the one captured line with event.action event.
func lineOf(t *testing.T, buf *bytes.Buffer, event string) map[string]any {
	t.Helper()
	var found []map[string]any
	for _, l := range logtest.Lines(t, buf) {
		if l["event.action"] == event {
			found = append(found, l)
		}
	}
	require.Len(t, found, 1, buf.String())
	return found[0]
}

// eegfaktura-dev#54 #4: the result of a migration run is in the log, also when nothing changed.
func TestMigrateDBLogsTheResult(t *testing.T) {
	dsn, _ := emptyDB(t, "migrate_result_test")
	versions := migrationVersions(t)
	latest := versions[len(versions)-1]

	buf := migrateLog(t)
	require.NoError(t, migrateDB(migrationsURL(t), dsn))
	done := lineOf(t, buf, "db.migrate.done")
	assert.Equal(t, "info", done["log.level"])
	assert.Equal(t, "database migrations applied", done["message"])
	assert.Equal(t, float64(0), done["migration.version_before"])
	assert.Equal(t, latest, done["migration.version_after"])
	assert.Equal(t, float64(len(versions)), done["migration.applied"])

	buf.Reset()
	require.NoError(t, migrateDB(migrationsURL(t), dsn))
	done = lineOf(t, buf, "db.migrate.done")
	assert.Equal(t, "database schema unchanged (no_change)", done["message"])
	assert.Equal(t, latest, done["migration.version_before"])
	assert.Equal(t, latest, done["migration.version_after"])
	assert.Equal(t, float64(0), done["migration.applied"])
}

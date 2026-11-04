package database

import (
	"context"
	"database/sql"
	"testing"

	"at.ourproject/vfeeg-backend/database/dbtest"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

var testDB *dbtest.TestDatabase

// GetTestDB opens the container database once and makes it the database GetDB returns.
func GetTestDB(ctx context.Context, dbInstance *dbtest.TestDatabase) (Database, error) {
	if dbInstance.DbInstance == nil {
		if err := dbInstance.Open(ctx); err != nil {
			return nil, err
		}
	}
	d := NewDatabase(dbInstance.DbInstance)
	SetDB(d)
	return d, nil
}

// seedDB is the database as migrated and seeded, before any test wrote to it (dbtest.SeedDbName).
// Only for tests that read and assert the seed data; they would otherwise depend on the tests run
// before them. Do not write to it.
func seedDB(t *testing.T) Database {
	t.Helper()
	conn, err := sqlx.Connect("postgres", testDB.SeedConnectionString())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return NewDatabase(conn)
}

type MockDatabase struct {
	Mock sqlmock.Sqlmock
	db   *sql.DB
}

func GetDatabaseMock() (*MockDatabase, error) {
	var err error
	m := &MockDatabase{}
	m.db, m.Mock, err = sqlmock.New()

	return m, err
}

func InitMockDatabase() (Database, sqlmock.Sqlmock, error) {
	dbInstance, mock, err := sqlmock.New()
	if err != nil {
		return nil, nil, err
	}
	return NewDatabase(sqlx.NewDb(dbInstance, "mock")), mock, nil
}

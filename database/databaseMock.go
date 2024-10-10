package database

import (
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

type mockDatabase struct {
	Mock sqlmock.Sqlmock
	db   *sql.DB
}

func GetDatabaseMock() (*mockDatabase, error) {
	var err error
	m := &mockDatabase{}
	m.db, m.Mock, err = sqlmock.New()

	return m, err
}

func (m *mockDatabase) OpenMockDb() (*sqlx.DB, error) {
	return sqlx.NewDb(m.db, "mock"), nil
}

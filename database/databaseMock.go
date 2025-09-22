package database

import (
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

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

func (m *MockDatabase) OpenMockDb() (*sqlx.DB, error) {
	return sqlx.NewDb(m.db, "mock"), nil
}

func (m *MockDatabase) Close() error {
	return m.db.Close()
}

func GetMockDb() (*MockDatabase, error) {
	mockDb, err := GetDatabaseMock()
	if err != nil {
		return nil, err
	}
	testDb, err := mockDb.OpenMockDb()
	InitTestDb(testDb)
	return mockDb, nil
}

func InitMockDatabase() (Database, sqlmock.Sqlmock, error) {
	dbInstance, mock, err := sqlmock.New()
	if err != nil {
		return nil, nil, err
	}
	return &sqlDatabase{db: sqlx.NewDb(dbInstance, "mock")}, mock, nil
}

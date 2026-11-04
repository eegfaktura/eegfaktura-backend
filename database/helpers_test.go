package database

import (
	"context"
	"database/sql"

	"at.ourproject/vfeeg-backend/database/dbtest"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
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

func GetMockDb() (*MockDatabase, error) {
	mockDb, err := GetDatabaseMock()
	if err != nil {
		return nil, err
	}
	SetDB(NewDatabase(sqlx.NewDb(mockDb.db, "mock")))
	return mockDb, nil
}

func InitMockDatabase() (Database, sqlmock.Sqlmock, error) {
	dbInstance, mock, err := sqlmock.New()
	if err != nil {
		return nil, nil, err
	}
	return NewDatabase(sqlx.NewDb(dbInstance, "mock")), mock, nil
}

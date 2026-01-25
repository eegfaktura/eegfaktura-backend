package database

import (
	"context"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	testDB = SetupTestDatabase()
	db, err := GetTestDB(context.Background(), testDB)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = db.CloseDB()
		testDB.TearDown()
	}()
	os.Exit(m.Run())
}

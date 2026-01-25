package database

import (
	"at.ourproject/vfeeg-backend/model"
	"context"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
	"testing"
)

//func openTestDb() (*sqlx.DB, error) {
//	testDB.Open(context.Background())
//	return testDB.DbInstance, nil
//}

//func TestMain(m *testing.M) {
//	testDB = SetupTestDatabase()
//	_, err := GetTestDB(context.Background())
//	if err != nil {
//		panic(err)
//	}
//	defer func() {
//		//db.Close()
//		testDB.TearDown()
//	}()
//	os.Exit(m.Run())
//}

func TestAddTariff(t *testing.T) {
	tariff := model.Tariff{Version: 0, Name: "Sepp", UseVat: false, BillingPeriod: "monthly", FreeKWh: null.IntFrom(100), CentPerKWh: 12}
	var mockDb, err = GetDatabaseMock()
	require.NoError(t, err)

	dbx := sqlx.NewDb(mockDb.db, "mock")

	//stmt := "INSERT INTO (.+) VALUES \\(0, 0, 0, 'monthly', 0, 12, 0, 100, DEFAULT, 'Sepp', 0, 'sepp', '', FALSE, 0, 1\\)"
	//stmt := "INSERT INTO \"base\".\"tariff\" \\(\"accountGrossAmount\", \"accountNetAmount\", \"baseFee\", \"billingPeriod\", \"businessNr\", \"centPerKWh\", \"createdBy\", \"createdDate\", \"discount\", \"freeKWh\", \"id\", \"lastModifiedDate\", \"meteringPointFee\", \"name\", \"participantFee\", \"tenant\", \"type\", \"useMeteringPointFee\", \"useVat\", \"vatInPercent\", \"vatSupplementaryText\", \"version\"\\) VALUES (.+)"

	mockDb.Mock.ExpectBegin()
	mockDb.Mock.ExpectExec("INSERT INTO \"base\".\"tariff\" (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
	//mockDb.Mock.ExpectExec("Update \"base\".\"tariff\" SET (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
	mockDb.Mock.ExpectCommit()

	err = AddTariff(dbx, "TE000001", "sepp", &tariff)
	assert.NoError(t, err)
	assert.NoError(t, mockDb.Mock.ExpectationsWereMet())
}

func TestUpdateTariff(t *testing.T) {
	tariff := model.Tariff{Version: 1, Name: "Sepp", UseVat: false, BillingPeriod: "monthly", FreeKWh: null.IntFrom(100), CentPerKWh: 12}

	db, err := GetDB(context.Background())
	require.NoError(t, err)

	err = db.AddTariff("TE000001", "sepp", &tariff)
	require.NoError(t, err)

	tariffSlice, err := db.GetTariff("TE000001")
	require.NoError(t, err)
	require.Equal(t, 2, len(tariffSlice))

	updateTariff := tariffSlice[0]
	updateTariff.UseMeteringFee = true
	updateTariff.MeteringFee = null.FloatFrom(200.11)

	err = db.AddTariff("TE000001", "sepp", &updateTariff)
	require.NoError(t, err)
	updatedTariff, err := db.GetTariff("TE000001")
	require.NoError(t, err)

	require.Equal(t, 2, len(updatedTariff))
	assert.Equal(t, 2, updatedTariff[0].Version)
	assert.Equal(t, true, updatedTariff[0].UseMeteringFee)
	assert.Equal(t, null.FloatFrom(200.11), updatedTariff[0].MeteringFee)
}

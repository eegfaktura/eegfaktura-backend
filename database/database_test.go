package database

import (
	"at.ourproject/vfeeg-backend/model"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAddTariff(t *testing.T) {
	tariff := model.Tariff{Version: 1, Name: "Sepp", UseVat: false, BillingPeriod: "monthly", FreeKWh: 100, CentPerKWh: 12}
	var mockDb, err = GetDatabaseMock()
	require.NoError(t, err)

	stmt := "INSERT INTO (.+) VALUES \\(0, 0, 0, 'monthly', 0, 12, 0, 100, DEFAULT, 'Sepp', 0, 'sepp', '', FALSE, 0, 1\\)"

	mockDb.Mock.ExpectExec(stmt).WillReturnResult(sqlmock.NewResult(1, 1))

	err = AddTariff(mockDb.OpenMockDb, "sepp", &tariff)
	assert.NoError(t, err)
	assert.NoError(t, mockDb.Mock.ExpectationsWereMet())
}

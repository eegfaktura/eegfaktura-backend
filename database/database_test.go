package database

import (
	"at.ourproject/vfeeg-backend/model"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/doug-martin/goqu/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAddTariff(t *testing.T) {
	tariff := model.Tariff{Version: 1, Name: "Sepp", UseVat: false, BillingPeriod: "monthly", FreeKWh: 100, CentPerKWh: 12}
	var mockDb, err = openDb()
	require.NoError(t, err)

	type updateType struct {
		Tenant string `json:"tenant" db:"tenant"`
		*model.Tariff
	}

	update := updateType{"sepp", &tariff}
	sql, _, err := goqu.Insert("base.tariff").Rows(update).ToSQL()
	assert.NoError(t, err)

	mockDb.mock.ExpectExec(sql).WillReturnResult(sqlmock.NewResult(1, 1))

	err = AddTariff(mockDb.mockDb, "sepp", &tariff)
	assert.NoError(t, err)
}

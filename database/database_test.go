package database

import (
	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAddTariff(t *testing.T) {
	tariff := model.Tariff{Version: 1, Name: "Sepp", UseVat: false, BillingPeriod: "monthly", FreeKWh: 100, CentPerKWh: 12}

	err := AddTariff("sepp", &tariff)
	assert.NoError(t, err)
}

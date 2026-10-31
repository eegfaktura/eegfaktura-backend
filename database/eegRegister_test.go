package database

import (
	"context"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
)

func registerEegFixture(tenant, rcNumber, name string) *model.Eeg {
	return &model.Eeg{
		Id: tenant, RcNumber: rcNumber, Name: name, Description: "Registrierungstest",
		Area: "LOCAL", Legal: "verein", AllocationMode: "DYNAMIC", SettlementInterval: "MONTHLY",
		GridOperator: "AT009999", OperatorName: "Test Operator", CommunityId: "AT00999900000TC000011000000000001",
		EegAddress: model.EegAddress{Street: "Solarweg", StreetNumber: "1", Zip: "4020", City: "Linz"},
		Contact:    model.Contact{Email: null.StringFrom("register-test@example.org")},
	}
}

// A second registration of a tenant must be refused and must not touch the first one (261015-ca1).
func TestInsertEegRefusesAnExistingTenant(t *testing.T) {
	db, err := GetDB(context.Background())
	require.NoError(t, err)
	ctx := context.Background()
	cleanup := func() {
		_, err := testDB.DbInstance.Exec(`DELETE FROM base.eeg WHERE upper(tenant) LIKE 'TE000031%'`)
		require.NoError(t, err)
	}
	cleanup()
	t.Cleanup(cleanup)

	require.NoError(t, db.InsertEeg(ctx, "TE000031", registerEegFixture("TE000031", "TE000031", "Erste Registrierung")))

	t.Run("same tenant", func(t *testing.T) {
		err := db.InsertEeg(ctx, "TE000031", registerEegFixture("TE000031", "TE000031", "Zweite Registrierung"))
		assert.ErrorIs(t, err, ErrEegExists)
	})

	t.Run("same tenant in another spelling", func(t *testing.T) {
		err := db.InsertEeg(ctx, "te000031", registerEegFixture("te000031", "TE000031", "Dritte Registrierung"))
		assert.ErrorIs(t, err, ErrEegExists)
	})

	t.Run("same tenant with an invalid mail address", func(t *testing.T) {
		eeg := registerEegFixture("TE000031", "TE000031", "Vierte Registrierung")
		eeg.Contact.Email = null.StringFrom("foo@")
		assert.ErrorIs(t, db.InsertEeg(ctx, "TE000031", eeg), ErrEegExists)
	})

	t.Run("a new tenant on an existing RC number outside GEA", func(t *testing.T) {
		err := db.InsertEeg(ctx, "TE000031", registerEegFixture("TE000031X", "te000031", "Tippfehler im Tenant"))
		assert.ErrorIs(t, err, ErrEegExists)
	})

	t.Run("the first registration stays as it was", func(t *testing.T) {
		var names []string
		require.NoError(t, testDB.DbInstance.Select(&names, `SELECT name FROM base.eeg WHERE upper(tenant) LIKE 'TE000031%'`))
		assert.Equal(t, []string{"Erste Registrierung"}, names)
	})

	// GEA: stem and sub communities share the RC number, each one is a tenant of its own
	t.Run("a new GEA tenant of an existing RC number stays possible", func(t *testing.T) {
		sub := registerEegFixture("TE000031-001", "TE000031", "GEA-Teilgemeinschaft")
		sub.Area = "GEA"
		assert.NoError(t, db.InsertEeg(ctx, "TE000031", sub))
	})
}

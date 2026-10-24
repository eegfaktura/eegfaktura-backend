package database

import (
	"context"
	"errors"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// archiveTariff must refuse a tariff that a member or a metering point of the tenant still uses
// (code 4002) and leave it unarchived; an unused tariff is archived.
func TestArchiveTariff_inUse(t *testing.T) {
	ctx := context.Background()
	db, err := GetTestDB(ctx, testDB)
	require.NoError(t, err)

	const tenant = "TE000417"
	const partId = "a4170000-0000-0000-0000-000000000001"
	exec := func(t *testing.T, q string, args ...interface{}) {
		_, err := testDB.DbInstance.Exec(q, args...)
		require.NoError(t, err)
	}
	newTariff := func(t *testing.T, name string) string {
		tariff := model.Tariff{Name: name, Type: "EEG", BillingPeriod: "monthly"}
		require.NoError(t, db.AddTariff(tenant, "test", &tariff))
		return tariff.Id.String()
	}
	status := func(t *testing.T, id string) string {
		var s string
		require.NoError(t, testDB.DbInstance.Get(&s, `SELECT DISTINCT COALESCE(status, '') FROM base.tariff WHERE tenant = $1 AND id = $2`, tenant, id))
		return s
	}
	assertInUse := func(t *testing.T, id string) {
		err := db.ArchiveTariff(tenant, id)
		var ve *model.VfeegError
		require.True(t, errors.As(err, &ve), "want a VfeegError, got %v", err)
		assert.Equal(t, 4002, ve.Code)
		assert.NotEqual(t, "ARCHIVED", status(t, id))
	}
	t.Cleanup(func() {
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.participant WHERE tenant = $1`, tenant)
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.tariff WHERE tenant = $1`, tenant)
	})

	participantTariff := newTariff(t, "Mitglied")
	meterTariff := newTariff(t, "Zaehlpunkt")
	unused := newTariff(t, "frei")

	exec(t, `INSERT INTO base.participant (id, tenant, firstname, lastname, status, "createdBy", "lastModifiedBy", "tariffId")
		VALUES ($1, $2, 'Ta', 'Rif', 'INACTIVE', 'test', 'test', $3)`, partId, tenant, participantTariff)
	exec(t, `INSERT INTO base.meteringpoint (metering_point_id, participant_id, tenant, status, tariff_id)
		VALUES ('AT0030000000000000000000000417001', $1, $2, 'INACTIVE', $3)`, partId, tenant, meterTariff)

	t.Run("member tariff in use", func(t *testing.T) { assertInUse(t, participantTariff) })
	t.Run("metering point tariff in use", func(t *testing.T) { assertInUse(t, meterTariff) })

	t.Run("unused tariff is archived", func(t *testing.T) {
		require.NoError(t, db.ArchiveTariff(tenant, unused))
		assert.Equal(t, "ARCHIVED", status(t, unused))
	})

	t.Run("use in another tenant does not count", func(t *testing.T) {
		other := newTariff(t, "fremd")
		exec(t, `INSERT INTO base.participant (id, tenant, firstname, lastname, status, "createdBy", "lastModifiedBy", "tariffId")
			VALUES ('a4170000-0000-0000-0000-000000000002', 'TE000418', 'An', 'Ders', 'ACTIVE', 'test', 'test', $1)`, other)
		t.Cleanup(func() {
			_, _ = testDB.DbInstance.Exec(`DELETE FROM base.participant WHERE tenant = 'TE000418'`)
		})
		require.NoError(t, db.ArchiveTariff(tenant, other))
		assert.Equal(t, "ARCHIVED", status(t, other))
	})

	t.Run("unknown tariff is not found", func(t *testing.T) {
		assertUnknown(t, db.ArchiveTariff(tenant, "a4170000-0000-0000-0000-0000000000ff"))
	})

	t.Run("tariff of another tenant is not found and stays", func(t *testing.T) {
		foreign := model.Tariff{Name: "fremder Tarif", Type: "EEG", BillingPeriod: "monthly"}
		require.NoError(t, db.AddTariff("TE000418", "test", &foreign))
		t.Cleanup(func() {
			_, _ = testDB.DbInstance.Exec(`DELETE FROM base.tariff WHERE tenant = 'TE000418'`)
		})
		assertUnknown(t, db.ArchiveTariff(tenant, foreign.Id.String()))
		var s string
		require.NoError(t, testDB.DbInstance.Get(&s, `SELECT DISTINCT COALESCE(status, '') FROM base.tariff WHERE tenant = 'TE000418' AND id = $1`, foreign.Id.String()))
		assert.NotEqual(t, "ARCHIVED", s)
	})
}

// An unknown tariff id (or one of another tenant) answers like AddTariff does: ErrUpdateTariff (4001).
func assertUnknown(t *testing.T, err error) {
	var ve *model.VfeegError
	require.True(t, errors.As(err, &ve), "want a VfeegError, got %v", err)
	assert.Equal(t, 4001, ve.Code)
	assert.EqualError(t, ve, "unknown tariff")
}

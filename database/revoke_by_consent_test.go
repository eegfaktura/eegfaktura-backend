package database

import (
	"context"
	"testing"

	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The same metering point is assigned in two communities (e.g. after a move). AUFHEBUNG_CCMI/CCMC
// carry no community id: before the fix both rows matched ("Meteringpoint … is not unique") and
// the revocation was dropped silently.
func TestMeteringPointRevokeByConsentId_sameMeterInTwoTenants(t *testing.T) {
	ctx := context.Background()
	db, err := GetTestDB(ctx, testDB)
	require.NoError(t, err)

	const meterA = "AT0030000000000000000000000111001"
	const meterB = "AT0030000000000000000000000111002"
	setup := func(tenant, communityId string, meters ...string) {
		_, err := testDB.DbInstance.Exec(`INSERT INTO base.eeg
			(tenant, name, description, "rcNumber", area, gridoperator_code, gridoperator_name, "communityId",
			 street, "streetNumber", city, zip, email)
			VALUES ($1, 'REVOKE-TEST', 'Revoke-Test', $2, 'LOCAL', 'AT003000', 'Netz OÖ', $3,
			 'Weg', '1', 'Linz', '4020', 'revoke-test@example.org')
			ON CONFLICT (tenant) DO NOTHING`, tenant, tenant, communityId)
		require.NoError(t, err)
		var rows [][]interface{}
		for _, m := range meters {
			rows = append(rows, []interface{}{"AT003000", communityId, "4020", "Linz", "Weg", "1",
				m, "CONSUMPTION", "Rev", "Oke" + tenant, "privat", "", "", "", "ACTIVE", "", ""})
		}
		f := buildImportSheet(t, rows)
		buf, err := f.WriteToBuffer()
		require.NoError(t, err)
		require.NoError(t, db.ImportMasterdataFromExcel(ctx, buf, "test.xlsx", "EEG Stammdaten", tenant))
	}
	// old community without consent id, new community with consent id
	setup("TE000011", "AT00300000000TC000011000000000001", meterA, meterB)
	setup("TE000012", "AT00300000000TC000012000000000001", meterA, meterB)
	_, err = testDB.DbInstance.Exec(`UPDATE base.meteringpoint SET consent_id = 'CONSENT-A' WHERE tenant = 'TE000012' AND metering_point_id = $1`, meterA)
	require.NoError(t, err)

	end := civil.DateFor(2026, 10, 1)
	state := func(tenant, meter string) string {
		var s string
		require.NoError(t, testDB.DbInstance.Get(&s, `SELECT process_state FROM base.meteringpoint WHERE tenant = $1 AND metering_point_id = $2`, tenant, meter))
		return s
	}

	t.Run("exact consent id wins, even without receiving tenant", func(t *testing.T) {
		consent := "CONSENT-A"
		tenant, err := db.MeteringPointRevokeByConsentId(ctx, "", &consent, meterA, end)
		require.NoError(t, err)
		assert.Equal(t, "TE000012", *tenant)
		assert.Equal(t, "INACTIVE", state("TE000012", meterA))
		assert.Equal(t, "ACTIVE", state("TE000011", meterA))
	})

	t.Run("receiving tenant decides when no consent id is stored", func(t *testing.T) {
		consent := "CONSENT-UNKNOWN"
		tenant, err := db.MeteringPointRevokeByConsentId(ctx, "te000011", &consent, meterB, end)
		require.NoError(t, err)
		assert.Equal(t, "TE000011", *tenant)
		assert.Equal(t, "INACTIVE", state("TE000011", meterB))
		assert.Equal(t, "ACTIVE", state("TE000012", meterB))
	})

	t.Run("still ambiguous without tenant and consent match: nothing changes", func(t *testing.T) {
		consent := "CONSENT-UNKNOWN"
		_, err := db.MeteringPointRevokeByConsentId(ctx, "", &consent, meterB, end)
		assert.ErrorContains(t, err, "not unique")
		assert.Equal(t, "ACTIVE", state("TE000012", meterB))
	})
}

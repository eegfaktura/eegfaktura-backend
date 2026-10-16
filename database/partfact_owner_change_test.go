package database

import (
	"context"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// support ticket 66110076: after an owner change a metering point exists twice in a community — the
// old member's entry (flag 0, INACTIVE) and the new member's (flag 1). A participation factor change
// (ANTWORT_CPF or the member form) must only reach the current entry.
func TestMeteringPointChangePartFactor_onlyCurrentEntryAfterOwnerChange(t *testing.T) {
	ctx := context.Background()
	db, err := GetTestDB(ctx, testDB)
	require.NoError(t, err)

	const tenant, meter = "TE000017", "AT0020000000000000000000100129467"
	const oldOwner, newOwner = "b1170000-0000-0000-0000-000000000001", "b1170000-0000-0000-0000-000000000002"
	exec := func(q string, args ...interface{}) {
		_, err := testDB.DbInstance.Exec(q, args...)
		require.NoError(t, err)
	}
	for _, p := range []string{oldOwner, newOwner} {
		exec(`INSERT INTO base.participant (id, tenant, firstname, lastname, status, "createdBy", "lastModifiedBy")
			VALUES ($1, $2, 'Owner', 'Change', 'ACTIVE', 'test', 'test')`, p, tenant)
	}
	exec(`INSERT INTO base.meteringpoint (metering_point_id, participant_id, tenant, direction, status, process_state,
		"modifiedAt", "modifiedBy", "registeredSince", activesince, inactivesince, flag)
		VALUES ($1, $2, $3, 'GENERATION', 'INACTIVE', 'INACTIVE', now(), 'test', '2023-07-06', '2023-07-06', '2025-03-02', 0)`,
		meter, oldOwner, tenant)
	exec(`INSERT INTO base.meteringpoint (metering_point_id, participant_id, tenant, direction, status, process_state,
		"modifiedAt", "modifiedBy", "registeredSince", activesince, inactivesince, flag)
		VALUES ($1, $2, $3, 'GENERATION', 'ACTIVE', 'ACTIVE', now(), 'test', '2025-03-05', '2025-03-06', '2999-12-31', 1)`,
		meter, newOwner, tenant)
	t.Cleanup(func() {
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.participant WHERE id IN ($1, $2)`, oldOwner, newOwner)
	})

	require.NoError(t, db.MeteringPointChangePartFactor(ctx, tenant, []model.Meter{
		{MeteringPoint: meter, Direction: model.GENERATOR, PartFact: 99},
	}))

	var rows []struct {
		Participant string `db:"participant_id"`
		PartFact    int    `db:"partFact"`
	}
	require.NoError(t, testDB.DbInstance.Select(&rows,
		`SELECT participant_id::text AS participant_id, "partFact" FROM base.metering_partition_factor
		 WHERE tenant = $1 AND metering_point_id = $2`, tenant, meter))
	require.Len(t, rows, 1, "only the current entry gets the new factor")
	assert.Equal(t, newOwner, rows[0].Participant)
	assert.Equal(t, 99, rows[0].PartFact)
}

package database

import (
	"context"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platform#272: the ABSCHLUSS date of the grid operator replaces a provisional activesince on the first activation
// of an entry (status INIT). A re-activation of the same member (INACTIVE) keeps the first date, so its older energy
// data stay visible. After an owner change the new member has its own entry: a first activation.
func TestMeteringPointsSetStatus_firstActivation(t *testing.T) {
	ctx := context.Background()
	db, err := GetTestDB(ctx, testDB)
	require.NoError(t, err)

	const tenant = "TE000017" // shared with other tests: own participants only
	const (
		first    = "c2720000-0000-0000-0000-000000000001"
		again    = "c2720000-0000-0000-0000-000000000002"
		oldOwner = "c2720000-0000-0000-0000-000000000003"
		newOwner = "c2720000-0000-0000-0000-000000000004"
	)
	const (
		meterFirst = "AT0030000000000000000000000272001"
		meterAgain = "AT0030000000000000000000000272002"
		meterOwner = "AT0030000000000000000000000272003"
	)
	exec := func(q string, args ...interface{}) {
		t.Helper()
		_, err := testDB.DbInstance.Exec(q, args...)
		require.NoError(t, err)
	}
	cleanup := func() {
		// cascades to the metering point and partition factor rows
		exec(`DELETE FROM base.participant WHERE id IN ($1, $2, $3, $4)`, first, again, oldOwner, newOwner)
	}
	cleanup()
	t.Cleanup(cleanup)
	for _, p := range []string{first, again, oldOwner, newOwner} {
		exec(`INSERT INTO base.participant (id, tenant, firstname, lastname, status, "createdBy", "lastModifiedBy")
			VALUES ($1, $2, 'First', 'Activation', 'ACTIVE', 'test', 'test')`, p, tenant)
	}
	insert := func(meter, participant, status, processState string, activeSince, inactiveSince interface{}, flag model.ProcessFlag) {
		exec(`INSERT INTO base.meteringpoint (metering_point_id, participant_id, tenant, direction, status, process_state,
			"modifiedAt", "modifiedBy", "registeredSince", activesince, inactivesince, flag)
			VALUES ($1, $2, $3, 'CONSUMPTION', $4, $5, now(), 'test', '2026-12-01', $6, $7, $8)`,
			meter, participant, tenant, status, processState, activeSince, inactiveSince, flag)
	}
	// (a) never active, with a provisional date entered by hand
	insert(meterFirst, first, "INIT", "APPROVED", "2026-11-01", nil, model.F_ASSIGNED)
	// (b) active from 2025-03-01 until a revoke, now registered again for the same member
	insert(meterAgain, again, "INACTIVE", "APPROVED", "2025-03-01", "2026-06-30", model.F_ASSIGNED)
	// (c) owner change: the old member's moved entry and the new member's entry
	insert(meterOwner, oldOwner, "INACTIVE", "INACTIVE", "2025-01-01", "2026-11-30", model.F_MOVED)
	insert(meterOwner, newOwner, "INIT", "APPROVED", nil, nil, model.F_ASSIGNED)

	gridOperatorDate := civil.DateFor(2026, 12, 15)
	var code int16 = 0
	require.NoError(t, db.MeteringPointsSetStatus(ctx, tenant, model.ACTIVE, &code,
		[]string{meterFirst, meterAgain, meterOwner}, &gridOperatorDate, nil))

	since := func(meter, participant string) (string, string) {
		var activeSince, status string
		require.NoError(t, testDB.DbInstance.QueryRow(`SELECT to_char(activesince, 'YYYY-MM-DD'), status
			FROM base.meteringpoint WHERE metering_point_id = $1 AND participant_id = $2`, meter, participant).
			Scan(&activeSince, &status))
		return activeSince, status
	}
	t.Run("first activation replaces a provisional date", func(t *testing.T) {
		d, status := since(meterFirst, first)
		assert.Equal(t, "2026-12-15", d)
		assert.Equal(t, "ACTIVE", status)
	})
	t.Run("re-activation keeps the first date", func(t *testing.T) {
		d, status := since(meterAgain, again)
		assert.Equal(t, "2025-03-01", d)
		assert.Equal(t, "ACTIVE", status)
	})
	t.Run("owner change: the new member's entry gets the new date, the old one keeps its own", func(t *testing.T) {
		d, status := since(meterOwner, newOwner)
		assert.Equal(t, "2026-12-15", d)
		assert.Equal(t, "ACTIVE", status)
		d, status = since(meterOwner, oldOwner)
		assert.Equal(t, "2025-01-01", d)
		assert.Equal(t, "INACTIVE", status)
	})
}

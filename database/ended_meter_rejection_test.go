package database

import (
	"context"
	"database/sql"
	"os"
	"regexp"
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/jjeffery/civil"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// konzept-zaehlpunkt-ausblenden.md, part 2: a rejected RE-registration of a metering point whose
// participation has ended keeps the process state INACTIVE; the rejected state goes to
// last_process_state, the code to statusCode. Never-active (INIT) and ACTIVE rows keep today's
// behaviour.

const endedRejectParticipant = "b1180000-0000-0000-0000-000000000001"

type endedRejectRow struct {
	ProcessState     string         `db:"process_state"`
	LastProcessState sql.NullString `db:"last_process_state"`
	Status           string         `db:"status"`
	StatusCode       sql.NullInt64  `db:"statusCode"`
	ModifiedBy       sql.NullString `db:"modifiedBy"`
}

func insertEndedRejectFixture(t *testing.T, tenant string, rows map[string][3]string) {
	exec := func(q string, args ...interface{}) {
		_, err := testDB.DbInstance.Exec(q, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO base.participant (id, tenant, firstname, lastname, status, "createdBy", "lastModifiedBy")
		VALUES ($1, $2, 'Beendet', 'Abgelehnt', 'ACTIVE', 'test', 'test')`, endedRejectParticipant, tenant)
	t.Cleanup(func() {
		_, _ = testDB.DbInstance.Exec(`DELETE FROM base.participant WHERE id = $1`, endedRejectParticipant)
	})
	// rows: meter -> {status, process_state, inactivesince ("" = NULL)}
	for meter, r := range rows {
		var inactive interface{}
		if r[2] != "" {
			inactive = r[2]
		}
		exec(`INSERT INTO base.meteringpoint (metering_point_id, participant_id, tenant, direction, status, process_state,
			"modifiedAt", "modifiedBy", "registeredSince", activesince, inactivesince)
			VALUES ($1, $2, $3, 'CONSUMPTION', $4, $5, now(), 'test', '2024-03-06', '2024-03-06', $6)`,
			meter, endedRejectParticipant, tenant, r[0], r[1], inactive)
	}
}

func readEndedRejectRow(t *testing.T, q interface {
	QueryRowx(string, ...interface{}) *sqlx.Row
}, tenant, meter string) endedRejectRow {
	t.Helper()
	var r endedRejectRow
	require.NoError(t, q.QueryRowx(`SELECT process_state, last_process_state, status, "statusCode", "modifiedBy"
		FROM base.meteringpoint WHERE tenant = $1 AND metering_point_id = $2 AND flag = 1`, tenant, meter).StructScan(&r))
	return r
}

func TestMeteringPointsSetStatus_endedMeterRejection(t *testing.T) {
	ctx := context.Background()
	db, err := GetTestDB(ctx, testDB)
	require.NoError(t, err)

	const tenant = "TE000018"
	const (
		endedInvalid  = "AT0030000000000000000000990018001"
		endedRejected = "AT0030000000000000000000990018002"
		neverActive   = "AT0030000000000000000000990018003"
		active        = "AT0030000000000000000000990018004"
		endedNoDate   = "AT0030000000000000000000990018005"
	)
	insertEndedRejectFixture(t, tenant, map[string][3]string{
		endedInvalid:  {"INACTIVE", "INIT", "2025-01-22"},
		endedRejected: {"INACTIVE", "PENDING", "2025-01-22"},
		neverActive:   {"INIT", "PENDING", ""},
		active:        {"ACTIVE", "ACTIVE", "2999-12-31"},
		endedNoDate:   {"INACTIVE", "INIT", ""},
	})

	code57, code99 := int16(57), int16(99)
	require.NoError(t, db.MeteringPointsSetStatus(ctx, tenant, model.INVALID, &code57,
		[]string{endedInvalid, neverActive, active, endedNoDate}, nil, nil))
	require.NoError(t, db.MeteringPointsSetStatus(ctx, tenant, model.REJECTED, &code99,
		[]string{endedRejected}, nil, nil))

	r := readEndedRejectRow(t, testDB.DbInstance, tenant, endedInvalid)
	assert.Equal(t, "INACTIVE", r.ProcessState, "ended meter stays INACTIVE")
	assert.Equal(t, "INVALID", r.LastProcessState.String, "rejected state kept for traceability")
	assert.Equal(t, "INACTIVE", r.Status)
	assert.Equal(t, int64(57), r.StatusCode.Int64)
	assert.Equal(t, "EVU", r.ModifiedBy.String)

	r = readEndedRejectRow(t, testDB.DbInstance, tenant, endedRejected)
	assert.Equal(t, "INACTIVE", r.ProcessState)
	assert.Equal(t, "REJECTED", r.LastProcessState.String)
	assert.Equal(t, int64(99), r.StatusCode.Int64)

	r = readEndedRejectRow(t, testDB.DbInstance, tenant, neverActive)
	assert.Equal(t, "INVALID", r.ProcessState, "never-active meter: as before")
	assert.False(t, r.LastProcessState.Valid)
	assert.Equal(t, "INIT", r.Status)
	assert.Equal(t, int64(57), r.StatusCode.Int64)

	r = readEndedRejectRow(t, testDB.DbInstance, tenant, endedNoDate)
	assert.Equal(t, "INVALID", r.ProcessState, "INACTIVE without inactivesince: as before")
	assert.False(t, r.LastProcessState.Valid)

	// an ACTIVE row keeps today's behaviour (the state follows the message, status stays ACTIVE)
	r = readEndedRejectRow(t, testDB.DbInstance, tenant, active)
	assert.Equal(t, "INVALID", r.ProcessState)
	assert.False(t, r.LastProcessState.Valid)
	assert.Equal(t, "ACTIVE", r.Status)

	// a later rejection with COMPETING_PROCESSES restores the state before the registration attempt:
	// an ended meter goes back to INACTIVE, not to the stored rejection
	_, err = testDB.DbInstance.Exec(`UPDATE base.meteringpoint SET process_state = 'INIT'
		WHERE tenant = $1 AND metering_point_id = $2`, tenant, endedInvalid)
	require.NoError(t, err)
	require.NoError(t, db.RestoreMeteringPointProcessState(ctx, tenant, endedInvalid))
	r = readEndedRejectRow(t, testDB.DbInstance, tenant, endedInvalid)
	assert.Equal(t, "INACTIVE", r.ProcessState)

	// other process states (e.g. PENDING) on an ended meter are unchanged
	require.NoError(t, db.MeteringPointsSetStatus(ctx, tenant, model.PENDING, nil, []string{endedRejected}, nil, nil))
	r = readEndedRejectRow(t, testDB.DbInstance, tenant, endedRejected)
	assert.Equal(t, "PENDING", r.ProcessState)
}

// Review backend#107: a successful re-registration clears last_process_state, so a later restore
// after code 86 does not set the now ACTIVE meter back to the old rejection.
func TestEndedMeterRejection_reRegisteredActiveIgnoresOldRejection(t *testing.T) {
	ctx := context.Background()
	db, err := GetTestDB(ctx, testDB)
	require.NoError(t, err)

	const tenant, meter = "TE000020", "AT0030000000000000000000990020001"
	insertEndedRejectFixture(t, tenant, map[string][3]string{meter: {"INACTIVE", "INIT", "2025-01-22"}})

	code57 := int16(57)
	require.NoError(t, db.MeteringPointsSetStatus(ctx, tenant, model.INVALID, &code57, []string{meter}, nil, nil))
	require.Equal(t, "INVALID", readEndedRejectRow(t, testDB.DbInstance, tenant, meter).LastProcessState.String)

	require.NoError(t, db.MeteringPointsSetStatus(ctx, tenant, model.INIT, nil, []string{meter}, nil, nil))
	today := civil.Today()
	require.NoError(t, db.MeteringPointsSetStatus(ctx, tenant, model.ACTIVE, nil, []string{meter}, &today, nil))
	r := readEndedRejectRow(t, testDB.DbInstance, tenant, meter)
	require.Equal(t, "ACTIVE", r.ProcessState)
	assert.False(t, r.LastProcessState.Valid, "the old rejection is cleared")

	require.NoError(t, db.RestoreMeteringPointProcessState(ctx, tenant, meter))
	assert.Equal(t, "ACTIVE", readEndedRejectRow(t, testDB.DbInstance, tenant, meter).ProcessState)
}

// scripts/ended-meter-rejection/: check.sql lists, fix.sql moves existing ended + rejected rows to
// INACTIVE (re-run changes nothing), rollback.sql restores them.
func TestEndedMeterRejectionScripts(t *testing.T) {
	_, err := GetTestDB(context.Background(), testDB)
	require.NoError(t, err)

	const tenant = "TE000019"
	const (
		endedInvalid  = "AT0030000000000000000000990019001"
		endedRejected = "AT0030000000000000000000990019002"
		neverActive   = "AT0030000000000000000000990019003"
	)
	insertEndedRejectFixture(t, tenant, map[string][3]string{
		endedInvalid:  {"INACTIVE", "INVALID", "2025-01-22"},
		endedRejected: {"INACTIVE", "REJECTED", "2025-01-22"},
		neverActive:   {"INIT", "INVALID", ""},
	})

	// the scripts work on all tenants: run them in one transaction and roll it back, so the
	// fixture tenants of other tests stay untouched
	tx, err := testDB.DbInstance.Beginx()
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()

	script := func(name string) string {
		b, err := os.ReadFile("../scripts/ended-meter-rejection/" + name)
		require.NoError(t, err)
		return string(b)
	}
	// the operator scripts open their own transaction; inside the test transaction they run without it
	stripTx := func(s string) string {
		return regexp.MustCompile(`(?m)^BEGIN;$`).ReplaceAllString(s, "")
	}
	countFor := func() int {
		rows, err := tx.Queryx(script("check.sql"))
		require.NoError(t, err)
		defer rows.Close()
		n := 0
		for rows.Next() {
			r := map[string]interface{}{}
			require.NoError(t, rows.MapScan(r))
			if r["tenant"] == tenant {
				n = int(r["anzahl"].(int64))
			}
		}
		return n
	}
	assert.Equal(t, 2, countFor())

	// fix.sql reports how many rows it changed (the operator compares it with check.sql)
	fix := stripTx(script("fix.sql"))
	changed := func(sql string) int {
		var n, tenants int
		require.NoError(t, tx.QueryRowx(sql).Scan(&n, &tenants))
		return n
	}
	assert.GreaterOrEqual(t, changed(fix), 2)
	assert.Equal(t, 0, countFor())

	r := readEndedRejectRow(t, tx, tenant, endedInvalid)
	assert.Equal(t, "INACTIVE", r.ProcessState)
	assert.Equal(t, "INVALID", r.LastProcessState.String)
	assert.Equal(t, "cleanup-2026-10-24", r.ModifiedBy.String)
	r = readEndedRejectRow(t, tx, tenant, endedRejected)
	assert.Equal(t, "INACTIVE", r.ProcessState)
	assert.Equal(t, "REJECTED", r.LastProcessState.String)
	r = readEndedRejectRow(t, tx, tenant, neverActive)
	assert.Equal(t, "INVALID", r.ProcessState, "never-active meter is not touched")
	assert.Equal(t, "test", r.ModifiedBy.String)

	// a second run changes nothing
	assert.Equal(t, 0, changed(fix))
	r = readEndedRejectRow(t, tx, tenant, endedInvalid)
	assert.Equal(t, "INVALID", r.LastProcessState.String)

	assert.GreaterOrEqual(t, changed(stripTx(script("rollback.sql"))), 2)
	r = readEndedRejectRow(t, tx, tenant, endedInvalid)
	assert.Equal(t, "INVALID", r.ProcessState)
	assert.False(t, r.LastProcessState.Valid)
	r = readEndedRejectRow(t, tx, tenant, endedRejected)
	assert.Equal(t, "REJECTED", r.ProcessState)
	assert.Equal(t, 2, countFor())
}

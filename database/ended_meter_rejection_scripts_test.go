package database

import (
	"context"
	"io"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
)

// runOperatorScript runs a script of scripts/ended-meter-rejection/ the way the operator does with the
// pgz helper: psql reads it from stdin, with ON_ERROR_STOP=1 and the given -v variables. The
// scripts commit for real.
func runOperatorScript(t *testing.T, name string, vars ...string) (int, string) {
	t.Helper()
	ctx := context.Background()
	b, err := os.ReadFile("../scripts/ended-meter-rejection/" + name)
	require.NoError(t, err)
	require.NoError(t, testDB.container.CopyToContainer(ctx, b, "/tmp/"+name, 0o644))
	cmd := "psql -U " + DbUser + " -d " + DbName + " -X -v ON_ERROR_STOP=1"
	for _, v := range vars {
		cmd += " -v " + v
	}
	code, r, err := testDB.container.Exec(ctx, []string{"sh", "-c", cmd + " < /tmp/" + name}, tcexec.Multiplexed())
	require.NoError(t, err)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	t.Logf("%s %v: exit %d: %s", name, vars, code, out)
	return code, string(out)
}

// checkTotals returns check.sql's total row (anzahl, beendet, aktiv, ohne_datum) and the row of tenant.
func checkTotals(t *testing.T, tenant string) (total, own map[string]int64) {
	t.Helper()
	b, err := os.ReadFile("../scripts/ended-meter-rejection/check.sql")
	require.NoError(t, err)
	rows, err := testDB.DbInstance.Queryx(string(b))
	require.NoError(t, err)
	defer rows.Close()
	total, own = map[string]int64{}, map[string]int64{}
	for rows.Next() {
		r := map[string]interface{}{}
		require.NoError(t, rows.MapScan(r))
		target := total
		if r["tenant"] == tenant {
			target = own
		} else if r["tenant"] != nil {
			continue
		}
		for _, k := range []string{"anzahl", "beendet", "aktiv", "ohne_datum", "aktiv_andere", "tenants"} {
			target[k] = r[k].(int64)
		}
	}
	return total, own
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// scripts/ended-meter-rejection/ (konzept-zaehlpunkt-ausblenden.md, part 2): check.sql counts the
// ended and the active variant, fix.sql changes exactly them only with the right expected count
// (self-guarding in one psql run), a re-run changes nothing, rollback.sql restores them.
func TestEndedMeterRejectionScripts(t *testing.T) {
	_, err := GetTestDB(context.Background(), testDB)
	require.NoError(t, err)

	const tenant = "TE000019"
	const (
		endedInvalid   = "AT0030000000000000000000990019001"
		endedRejected  = "AT0030000000000000000000990019002"
		neverActive    = "AT0030000000000000000000990019003"
		endedNoDate    = "AT0030000000000000000000990019004"
		activeRejected = "AT0030000000000000000000990019005"
		activeOther    = "AT0030000000000000000000990019006"
	)
	insertEndedRejectFixture(t, tenant, map[string][3]string{
		endedInvalid:   {"INACTIVE", "INVALID", "2025-01-22"},
		endedRejected:  {"INACTIVE", "REJECTED", "2025-01-22"},
		neverActive:    {"INIT", "INVALID", ""},
		endedNoDate:    {"INACTIVE", "INVALID", ""},
		activeRejected: {"ACTIVE", "REJECTED", "2999-12-31"},
		activeOther:    {"ACTIVE", "INVALID", "2999-12-31"},
	})
	// active variant: only request-level codes (188) are cleaned up; 57 concerns the metering point
	_, err = testDB.DbInstance.Exec(`UPDATE base.meteringpoint SET "statusCode" = CASE metering_point_id
		WHEN $2 THEN 188 WHEN $3 THEN 57 ELSE 57 END WHERE tenant = $1`, tenant, activeRejected, activeOther)
	require.NoError(t, err)
	row := func(meter string) endedRejectRow { return readEndedRejectRow(t, testDB.DbInstance, tenant, meter) }

	total, own := checkTotals(t, tenant)
	assert.Equal(t, map[string]int64{"anzahl": 3, "beendet": 2, "aktiv": 1, "ohne_datum": 1, "aktiv_andere": 1, "tenants": 1}, own)
	expected := total["anzahl"]

	// without or with a wrong expected count nothing is committed
	code, out := runOperatorScript(t, "fix.sql")
	assert.NotEqual(t, 0, code, out)
	code, out = runOperatorScript(t, "fix.sql", "expected="+itoa(expected+1))
	assert.NotEqual(t, 0, code, out)
	assert.Equal(t, "INVALID", row(endedInvalid).ProcessState)
	assert.Equal(t, "REJECTED", row(activeRejected).ProcessState)

	code, out = runOperatorScript(t, "fix.sql", "expected="+itoa(expected))
	require.Equal(t, 0, code, out)
	assert.Contains(t, out, "COMMIT")

	r := row(endedInvalid)
	assert.Equal(t, "INACTIVE", r.ProcessState)
	assert.Equal(t, "INVALID", r.LastProcessState.String)
	assert.Equal(t, "cleanup-2026-10-24", r.ModifiedBy.String)
	r = row(endedRejected)
	assert.Equal(t, "INACTIVE", r.ProcessState)
	assert.Equal(t, "REJECTED", r.LastProcessState.String)
	r = row(activeRejected)
	assert.Equal(t, "ACTIVE", r.ProcessState)
	assert.Equal(t, "REJECTED", r.LastProcessState.String)
	assert.Equal(t, "INVALID", row(neverActive).ProcessState, "never-active meter is not touched")
	assert.Equal(t, "test", row(neverActive).ModifiedBy.String)
	assert.Equal(t, "INVALID", row(endedNoDate).ProcessState, "no end date: not touched")
	assert.Equal(t, "INVALID", row(activeOther).ProcessState, "active + code 57: not touched")

	total, _ = checkTotals(t, tenant)
	assert.Equal(t, int64(0), total["anzahl"])

	// a second run changes nothing (expected 0)
	code, out = runOperatorScript(t, "fix.sql", "expected=0")
	require.Equal(t, 0, code, out)

	// an entry that became history meanwhile (flag 0) is not rolled back
	_, err = testDB.DbInstance.Exec(`UPDATE base.meteringpoint SET flag = 0 WHERE tenant = $1 AND metering_point_id = $2`, tenant, endedRejected)
	require.NoError(t, err)
	var toRestore int64
	require.NoError(t, testDB.DbInstance.Get(&toRestore, `SELECT count(*) FROM base.meteringpoint
		WHERE flag = 1 AND "modifiedBy" = 'cleanup-2026-10-24'`))

	code, out = runOperatorScript(t, "rollback.sql", "expected="+itoa(toRestore+1))
	assert.NotEqual(t, 0, code, out)
	assert.Equal(t, "INACTIVE", row(endedInvalid).ProcessState)

	code, out = runOperatorScript(t, "rollback.sql", "expected="+itoa(toRestore))
	require.Equal(t, 0, code, out)
	r = row(endedInvalid)
	assert.Equal(t, "INVALID", r.ProcessState)
	assert.False(t, r.LastProcessState.Valid)
	assert.Equal(t, "REJECTED", row(activeRejected).ProcessState)
	var historyState string
	require.NoError(t, testDB.DbInstance.Get(&historyState, `SELECT process_state FROM base.meteringpoint
		WHERE tenant = $1 AND metering_point_id = $2 AND flag = 0`, tenant, endedRejected))
	assert.Equal(t, "INACTIVE", historyState)
	_, own = checkTotals(t, tenant)
	assert.Equal(t, int64(2), own["anzahl"])
}

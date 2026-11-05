package database

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// support ticket 6611200: base.activeMeteringPartition numbered the factors of all communities before the
// tenant filter of the caller applied (the window was partitioned without tenant). These tests compare the
// view with its former definition, which the down migration restores.

const (
	ampTenantA, ampTenantB = "TE900001", "TE900002"
	ampMeter, ampMeter2    = "AT0030000000000000000000000900001", "AT0030000000000000000000000900002"
	ampOldOwner            = "b6611200-0000-0000-0000-00000000000a"
	ampNewOwner            = "b6611200-0000-0000-0000-00000000000b"
	ampOtherTenant         = "b6611200-0000-0000-0000-00000000000c"
)

// ampTx opens a transaction (rolled back when the test ends) with the test rows and the former definition of
// the view as the temporary view amp_old.
func ampTx(t *testing.T) *sqlx.Tx {
	t.Helper()
	_, err := GetTestDB(t.Context(), testDB)
	require.NoError(t, err)
	tx, err := testDB.DbInstance.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	exec := func(q string, args ...interface{}) {
		t.Helper()
		_, err := tx.Exec(q, args...)
		require.NoError(t, err)
	}

	down, err := os.ReadFile("../migrations/20261105140000_activemeteringpartition_tenant.down.sql")
	require.NoError(t, err)
	const create = "CREATE OR REPLACE VIEW base.activeMeteringPartition AS"
	require.Equal(t, 1, strings.Count(string(down), create))
	exec(strings.Replace(string(down), create, "CREATE TEMP VIEW amp_old AS", 1))

	for p, tenant := range map[string]string{ampOldOwner: ampTenantA, ampNewOwner: ampTenantA, ampOtherTenant: ampTenantB} {
		exec(`INSERT INTO base.participant (id, tenant, firstname, lastname, status, "createdBy", "lastModifiedBy")
			VALUES ($1, $2, 'Partition', 'Factor', 'ACTIVE', 'test', 'test')`, p, tenant)
	}
	meter := func(m, p, tenant string, flag int) {
		exec(`INSERT INTO base.meteringpoint (metering_point_id, participant_id, tenant, flag) VALUES ($1, $2, $3, $4)`,
			m, p, tenant, flag)
	}
	meter(ampMeter, ampOldOwner, ampTenantA, 0) // owner change: the old member's moved entry
	meter(ampMeter, ampNewOwner, ampTenantA, 1)
	meter(ampMeter2, ampNewOwner, ampTenantA, 1)
	meter(ampMeter, ampOtherTenant, ampTenantB, 1) // the same number in a second community
	// several versions each; the last one per (metering point, participant) is the active factor
	for _, f := range []struct {
		m, p, tenant string
		fact         int
	}{
		{ampMeter, ampOldOwner, ampTenantA, 100}, {ampMeter, ampNewOwner, ampTenantA, 60}, {ampMeter, ampOldOwner, ampTenantA, 90},
		{ampMeter, ampOtherTenant, ampTenantB, 30}, {ampMeter2, ampNewOwner, ampTenantA, 10}, {ampMeter, ampNewOwner, ampTenantA, 50},
		{ampMeter, ampOtherTenant, ampTenantB, 40},
	} {
		exec(`INSERT INTO base.metering_partition_factor (metering_point_id, participant_id, tenant, "partFact", "createdBy")
			VALUES ($1, $2, $3, $4, 'test')`, f.m, f.p, f.tenant, f.fact)
	}
	return tx
}

func TestActiveMeteringPartition_sameRowsAsFormerDefinition(t *testing.T) {
	tx := ampTx(t)

	var active []string
	require.NoError(t, tx.Select(&active, `SELECT tenant || ' ' || metering_point_id || ' ' || participant_id || ' ' || "partFact"
		FROM base.activeMeteringPartition WHERE tenant IN ($1, $2)`, ampTenantA, ampTenantB))
	sort.Strings(active)
	assert.Equal(t, []string{
		ampTenantA + " " + ampMeter + " " + ampOldOwner + " 90",
		ampTenantA + " " + ampMeter + " " + ampNewOwner + " 50",
		ampTenantA + " " + ampMeter2 + " " + ampNewOwner + " 10",
		ampTenantB + " " + ampMeter + " " + ampOtherTenant + " 40",
	}, active, "the last factor per member, each community on its own")

	// all rows, the seed data included, all columns
	for _, q := range []string{
		`SELECT count(*) FROM (SELECT * FROM amp_old EXCEPT ALL SELECT * FROM base.activeMeteringPartition) d`,
		`SELECT count(*) FROM (SELECT * FROM base.activeMeteringPartition EXCEPT ALL SELECT * FROM amp_old) d`,
	} {
		var n int
		require.NoError(t, tx.Get(&n, q))
		assert.Zero(t, n, q)
	}
}

func TestActiveMeteringPartition_tenantFilterBelowWindow(t *testing.T) {
	tx := ampTx(t)
	for view, pushed := range map[string]bool{"base.activeMeteringPartition": true, "amp_old": false} {
		plan := explainJSON(t, tx, `EXPLAIN (FORMAT JSON) SELECT "partFact" FROM `+view+` WHERE tenant = '`+ampTenantA+`'`)
		windows := planNodes(plan, "WindowAgg")
		require.Len(t, windows, 1, view)
		assert.Equal(t, pushed, tenantCondition(windows[0]["Plans"].([]interface{})[0].(map[string]interface{})),
			"%s: tenant condition below the WindowAgg", view)
	}
}

// The plan of the ticket: the view on the inner side of a nested loop, scanned once per metering point. The
// LATERAL reference forces that shape (no materialisation, no memoize). Row counts only, no timing.
func TestActiveMeteringPartition_innerSideOfNestedLoopReadsOneCommunity(t *testing.T) {
	tx := ampTx(t)
	_, err := tx.Exec(`SET LOCAL enable_material = off; SET LOCAL enable_memoize = off`)
	require.NoError(t, err)
	var all, community, meters float64
	require.NoError(t, tx.Get(&all, `SELECT count(*) FROM base.metering_partition_factor`))
	require.NoError(t, tx.Get(&community, `SELECT count(*) FROM base.metering_partition_factor WHERE tenant = $1`, ampTenantA))
	require.NoError(t, tx.Get(&meters, `SELECT count(*) FROM base.meteringpoint WHERE tenant = $1`, ampTenantA))
	require.Less(t, community, all)

	for view, perLoop := range map[string]float64{"base.activeMeteringPartition": community, "amp_old": all} {
		plan := explainJSON(t, tx, `EXPLAIN (ANALYZE, FORMAT JSON) SELECT m.metering_point_id, v."partFact"
			FROM base.meteringpoint m,
			     LATERAL (SELECT "partFact", metering_point_id AS mpfmid, participant_id AS mpfpid FROM `+view+`
			              WHERE tenant = '`+ampTenantA+`' AND m.flag IS NOT NULL OFFSET 0) v
			WHERE v.mpfmid = m.metering_point_id AND v.mpfpid = m.participant_id AND m.tenant = '`+ampTenantA+`'`)
		require.Equal(t, "Nested Loop", plan["Node Type"], view)
		assert.EqualValues(t, 3, plan["Actual Rows"], view)
		windows := planNodes(plan, "WindowAgg")
		require.Len(t, windows, 1, view)
		assert.Equal(t, meters, windows[0]["Actual Loops"], "%s: one scan of the view per metering point", view)
		input := windows[0]["Plans"].([]interface{})[0].(map[string]interface{})
		assert.Equal(t, perLoop, input["Actual Rows"], "%s: factor rows numbered per scan of the view", view)
	}
}

// Migrations 20261105140000 (view) and 20261105140100 (index, CONCURRENTLY): up, down and up again.
func TestActiveMeteringPartitionMigrationUpDown(t *testing.T) {
	dbx, m := migrationDB(t, "activemeteringpartition_migration_test")
	state := func() (partitionedByTenant, index bool) {
		t.Helper()
		var def string
		require.NoError(t, dbx.Get(&def, `SELECT pg_get_viewdef('base.activemeteringpartition'::regclass)`))
		require.NoError(t, dbx.Get(&index, `SELECT to_regclass('base.metering_partition_factor_tenant_idx') IS NOT NULL`))
		return strings.Contains(def, "PARTITION BY metering_partition_factor.tenant,"), index
	}
	const indexMigration = 20261105140100
	require.NoError(t, m.Migrate(indexMigration))
	byTenant, index := state()
	assert.True(t, byTenant && index, "up")
	require.NoError(t, m.Steps(-2))
	byTenant, index = state()
	assert.False(t, byTenant || index, "down")
	require.NoError(t, m.Migrate(indexMigration))
	byTenant, index = state()
	assert.True(t, byTenant && index, "up again")
}

func explainJSON(t *testing.T, tx *sqlx.Tx, q string) map[string]interface{} {
	t.Helper()
	var raw string
	require.NoError(t, tx.Get(&raw, q))
	var out []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(raw), &out))
	return out[0]["Plan"].(map[string]interface{})
}

// planNodes are the nodes of the given type in the plan tree.
func planNodes(n map[string]interface{}, nodeType string) []map[string]interface{} {
	var found []map[string]interface{}
	if n["Node Type"] == nodeType {
		found = append(found, n)
	}
	children, _ := n["Plans"].([]interface{})
	for _, c := range children {
		found = append(found, planNodes(c.(map[string]interface{}), nodeType)...)
	}
	return found
}

// tenantCondition: a node of the subtree filters or searches on tenant.
func tenantCondition(n map[string]interface{}) bool {
	for _, k := range []string{"Filter", "Index Cond", "Recheck Cond"} {
		if c, ok := n[k].(string); ok && strings.Contains(c, "tenant") {
			return true
		}
	}
	children, _ := n["Plans"].([]interface{})
	for _, c := range children {
		if tenantCondition(c.(map[string]interface{})) {
			return true
		}
	}
	return false
}

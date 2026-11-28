package database

import (
	"encoding/json"
	"testing"

	"github.com/jjeffery/civil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platform#282: base.processhistory had no index besides the primary key; both reads scanned the whole table.

// Migrations 20261128120000 and 20261128120100 (indexes, CONCURRENTLY): up, down and up again.
func TestProcessHistoryIndexMigrationUpDown(t *testing.T) {
	dbx, m := migrationDB(t, "processhistory_index_migration_test")
	indexes := func() (dateIdx, conversationIdx bool) {
		t.Helper()
		require.NoError(t, dbx.Get(&dateIdx, `SELECT to_regclass('base.processhistory_tenant_protocol_date_idx') IS NOT NULL`))
		require.NoError(t, dbx.Get(&conversationIdx, `SELECT to_regclass('base.processhistory_tenant_conversation_idx') IS NOT NULL`))
		return dateIdx, conversationIdx
	}
	const conversationMigration = 20261128120100
	require.NoError(t, m.Migrate(conversationMigration))
	date, conversation := indexes()
	assert.True(t, date && conversation, "up")
	require.NoError(t, m.Steps(-2))
	date, conversation = indexes()
	assert.False(t, date || conversation, "down")
	require.NoError(t, m.Migrate(conversationMigration))
	date, conversation = indexes()
	assert.True(t, date && conversation, "up again")
}

// The queries of fetchEdaHistory and FindHistoryMessage use their index on a table with many rows of other
// communities and conversations.
func TestProcessHistoryQueriesUseIndex(t *testing.T) {
	_, err := GetTestDB(t.Context(), testDB)
	require.NoError(t, err)
	tx, err := testDB.DbInstance.BeginTxx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })

	// 20,000 rows: 50 communities, a conversation per row, two protocols, one day after the other
	_, err = tx.Exec(`INSERT INTO base.processhistory (tenant, "conversationId", type, date, issuer, protocol)
		SELECT 'TE9' || lpad((i % 50)::text, 5, '0'), 'conv-' || i, 'ANTWORT_CCMS', timestamp '2020-01-01' + i * interval '1 hour',
			'ADMIN', CASE WHEN i % 2 = 0 THEN 'CR_MSG' ELSE 'CM_REV_SP' END
		FROM generate_series(1, 20000) i`)
	require.NoError(t, err)
	_, err = tx.Exec(`ANALYZE base.processhistory`)
	require.NoError(t, err)

	explain := func(q string, args ...interface{}) map[string]interface{} {
		t.Helper()
		var raw string
		require.NoError(t, tx.Get(&raw, "EXPLAIN (FORMAT JSON) "+q, args...))
		var out []map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(raw), &out))
		return out[0]["Plan"].(map[string]interface{})
	}
	usesIndex := func(plan map[string]interface{}, index string) bool {
		for _, n := range planNodes(plan, "") {
			if n["Index Name"] == index {
				return true
			}
		}
		return false
	}

	stmt, _, err := edaHistorySelect("TE900007", []string{"CR_MSG", "CM_REV_SP"},
		civil.DateFor(2020, 3, 1), civil.DateFor(2020, 3, 31)).ToSQL()
	require.NoError(t, err)
	plan := explain(stmt)
	assert.True(t, usesIndex(plan, "processhistory_tenant_protocol_date_idx"), "Prozess History: %v", plan)

	plan = explain(findHistoryMessageQuery, "TE900007", "conv-1007", "ANTWORT_CCMS")
	assert.True(t, usesIndex(plan, "processhistory_tenant_conversation_idx"), "FindHistoryMessage: %v", plan)
}

package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// backend 261128-mm2: the end bound of fetchEdaHistory was inclusive, so a message at 00:00 of the day after a range
// came in two adjacent ranges. (The page size reaching the SQL is checked in api/processController_test.go.)
func TestFetchEdaHistoryRange(t *testing.T) {
	_, err := GetTestDB(t.Context(), testDB)
	require.NoError(t, err)
	db := testDB.DbInstance
	const tenant = "TE900261"
	cleanup := func() { _, _ = db.Exec(`DELETE FROM base.processhistory WHERE tenant = $1`, tenant) }
	cleanup()
	t.Cleanup(cleanup)

	// three messages: 01.03. 10:00, 02.03. 23:59:59 and exactly 03.03. 00:00
	for conv, ts := range map[string]string{"conv-a": "2020-03-01 10:00:00", "conv-b": "2020-03-02 23:59:59",
		"conv-c": "2020-03-03 00:00:00"} {
		_, err = db.Exec(`INSERT INTO base.processhistory (tenant, "conversationId", type, date, issuer, protocol, message)
			VALUES ($1, $2, 'ANTWORT_ECON', $3::timestamp, 'ADMIN', 'EC_REQ_ONL', '{}')`, tenant, conv, ts)
		require.NoError(t, err)
	}
	ms := func(day int) int64 { return time.Date(2020, 3, day, 0, 0, 0, 0, time.Local).UnixMilli() }
	conversations := func(start, end int64, pageSize uint) []string {
		t.Helper()
		res, err := fetchEdaHistory(t.Context(), db, tenant, "EC_REQ_ONL", start, end, pageSize)
		require.NoError(t, err)
		raw, err := json.Marshal(res)
		require.NoError(t, err)
		var out struct {
			Data map[string]map[string]json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(raw, &out))
		convs := []string{}
		for c := range out.Data["EC_REQ_ONL"] {
			convs = append(convs, c)
		}
		return convs
	}

	// web asks for 01.03.–02.03. as start = first day, end = day after the last day
	assert.ElementsMatch(t, []string{"conv-a", "conv-b"}, conversations(ms(1), ms(3), 0), "end is exclusive")
	assert.ElementsMatch(t, []string{"conv-c"}, conversations(ms(3), ms(4), 0), "00:00 belongs to the next range")

}

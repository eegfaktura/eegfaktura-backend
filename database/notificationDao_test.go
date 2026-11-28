package database

import (
	"at.ourproject/vfeeg-backend/model"
	"context"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"maps"
	"math"
	"slices"
	"testing"
	"time"
)

func TestFetchEdaHistory(t *testing.T) {
	mDB, mock := newMockDatabase(t)

	start, _ := time.Parse(time.RFC3339Nano, "2023-10-03T17:00:00.000Z")
	end, _ := time.Parse(time.RFC3339Nano, "2023-10-04T18:00:00.000Z")

	//stmt := "SELECT \"conversationId\", \"date\", \"direction\", \"issuer\", \"message\", \"protocol\", \"tenant\", \"type\" FROM \"base\".\"processhistory\" WHERE ((\"tenant\" = 'RC100298') AND (\"protocol\" IS NOT NULL) AND (\"date\" BETWEEN '2023-10-03T19:00:00+02:00' AND '2023-10-04T20:00:00+02:00'))"
	stmt := "SELECT (.+) FROM \"base\".\"processhistory\" WHERE (.+)"

	rows := sqlmock.NewRows([]string{"conversationId", "date", "direction", "issuer", "message", "protocol", "tenant", "type"}).
		AddRow("1", time.Now(), "CONSUMPTION", "ADMIN", "{}", "CR_MSG", "RC100298", model.EBMS_ONLINE_REG_APPROVAL)
	mock.ExpectQuery(stmt).WillReturnRows(rows)
	//res, err := FetchEdaHistory(mockDb.OpenMockDb, "RC100298", (time.Now().Add(25 * time.Hour * -1)).UnixMilli(), time.Now().UnixMilli())
	res, err := mDB.FetchEdaHistory(context.Background(), "RC100298", "", start.UnixMilli(), end.UnixMilli(), 0)
	require.NoError(t, err)
	require.NotNil(t, res)
}

// backend 261128-mm4: a message that is no JSON object (the column is json, so it is valid JSON) made fetchEdaHistory
// return the history together with the error (handler 400) when it was the last row, and the row came with
// "message": null otherwise, as did a stored JSON null. Now such a row is skipped and the rest is answered.
func TestFetchEdaHistorySkipsBrokenMessage(t *testing.T) {
	mDB, mock := newMockDatabase(t)
	rows := sqlmock.NewRows([]string{"conversationId", "date", "direction", "issuer", "message", "protocol", "tenant", "type"})
	for _, r := range [][2]string{{"conv-a", `{"meter": "AT001"}`}, {"conv-x", `"text"`}, {"conv-b", `{}`}, {"conv-n", `null`}, {"conv-z", `[1]`}} {
		rows.AddRow(r[0], time.Now(), "IN", "ADMIN", []byte(r[1]), "EC_REQ_ONL", "RC100298", model.EBMS_ONLINE_REG_APPROVAL)
	}
	mock.ExpectQuery("SELECT (.+) FROM \"base\".\"processhistory\" WHERE (.+)").WillReturnRows(rows)

	res, err := mDB.FetchEdaHistory(context.Background(), "RC100298", "EC_REQ_ONL", 0, time.Now().UnixMilli(), 0)
	require.NoError(t, err)
	raw, err := json.Marshal(res)
	require.NoError(t, err)
	var out struct {
		Data map[string]map[string][]struct {
			Message map[string]interface{} `json:"message"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))
	got := out.Data["EC_REQ_ONL"]
	require.ElementsMatch(t, []string{"conv-a", "conv-b"}, slices.Collect(maps.Keys(got)), "broken rows are skipped")
	require.Equal(t, "AT001", got["conv-a"][0].Message["meter"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFetchEdaHistory_pageSizeBounds(t *testing.T) {
	start, _ := time.Parse(time.RFC3339Nano, "2023-10-03T17:00:00.000Z")
	end, _ := time.Parse(time.RFC3339Nano, "2023-10-04T18:00:00.000Z")

	type result struct {
		History struct {
			Next struct {
				Start    int64 `json:"start"`
				PageSize uint  `json:"page_size"`
			} `json:"next"`
		}
		Data map[string]map[string][]json.RawMessage `json:"data"`
	}
	fetch := func(pageSize uint) result {
		mDB, mock := newMockDatabase(t)
		rows := sqlmock.NewRows([]string{"conversationId", "date", "message", "protocol", "tenant"}).
			AddRow("1", start, "{}", "CR_MSG", "RC100298").
			AddRow("2", start, "{}", "CR_MSG", "RC100298").
			AddRow("3", end, "{}", "CR_MSG", "RC100298")
		mock.ExpectQuery(`SELECT (.+) FROM "base"."processhistory" WHERE (.+)`).WillReturnRows(rows)

		var res interface{}
		var err error
		require.NotPanics(t, func() {
			res, err = mDB.FetchEdaHistory(context.Background(), "RC100298", "CR_MSG", start.UnixMilli(), end.UnixMilli(), pageSize)
		})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())

		b, err := json.Marshal(res)
		require.NoError(t, err)
		var out result
		require.NoError(t, json.Unmarshal(b, &out))
		return out
	}

	// Page size smaller than the result: first page plus next cursor.
	out := fetch(2)
	require.Len(t, out.Data["CR_MSG"], 2)
	require.Equal(t, uint(2), out.History.Next.PageSize)
	require.Equal(t, end.Unix()*1000, out.History.Next.Start)

	// Page size larger than the result, up to the largest uint: all rows, no next cursor.
	for _, ps := range []uint{3, 1000, math.MaxUint} {
		out = fetch(ps)
		require.Len(t, out.Data["CR_MSG"], 3)
		require.Zero(t, out.History.Next.PageSize)
	}
}

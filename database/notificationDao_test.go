package database

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestFetchEdaHistory(t *testing.T) {
	var mDB, mock, err = InitMockDatabase()
	require.NoError(t, err)

	start, _ := time.Parse(time.RFC3339Nano, "2023-10-03T17:00:00.000Z")
	end, _ := time.Parse(time.RFC3339Nano, "2023-10-04T18:00:00.000Z")

	//stmt := "SELECT \"conversationId\", \"date\", \"direction\", \"issuer\", \"message\", \"protocol\", \"tenant\", \"type\" FROM \"base\".\"processhistory\" WHERE ((\"tenant\" = 'RC100298') AND (\"protocol\" IS NOT NULL) AND (\"date\" BETWEEN '2023-10-03T19:00:00+02:00' AND '2023-10-04T20:00:00+02:00'))"
	stmt := "SELECT (.+) FROM \"base\".\"processhistory\" WHERE (.+)"

	rows := sqlmock.NewRows([]string{"conversationId", "date", "direction", "issuer", "message", "protocol", "tenant", "type"}).
		AddRow("1", time.Now(), "CONSUMPTION", "ADMIN", "{}", "CR_MSG", "RC100298", model.EBMS_ONLINE_REG_APPROVAL)
	mock.ExpectQuery(stmt).WillReturnRows(rows)
	//res, err := FetchEdaHistory(mockDb.OpenMockDb, "RC100298", (time.Now().Add(25 * time.Hour * -1)).UnixMilli(), time.Now().UnixMilli())
	res, err := mDB.FetchEdaHistory("RC100298", "", start.UnixMilli(), end.UnixMilli(), 0)
	require.NoError(t, err)
	require.NotNil(t, res)
}

func TestFetchEdaHistory_appliesPageSizeLimit(t *testing.T) {
	var mDB, mock, err = InitMockDatabase()
	require.NoError(t, err)

	start, _ := time.Parse(time.RFC3339Nano, "2023-10-03T17:00:00.000Z")
	end, _ := time.Parse(time.RFC3339Nano, "2023-10-04T18:00:00.000Z")

	// pageSize 2 must fetch one extra row (LIMIT 3) to detect a next page.
	rows := sqlmock.NewRows([]string{"conversationId", "date", "message", "protocol", "tenant"}).
		AddRow("1", start, "{}", "CR_MSG", "RC100298").
		AddRow("2", start, "{}", "CR_MSG", "RC100298").
		AddRow("3", end, "{}", "CR_MSG", "RC100298")
	mock.ExpectQuery(`SELECT (.+) FROM "base"."processhistory" WHERE (.+) LIMIT 3$`).WillReturnRows(rows)

	res, err := mDB.FetchEdaHistory("RC100298", "CR_MSG", start.UnixMilli(), end.UnixMilli(), 2)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	b, err := json.Marshal(res)
	require.NoError(t, err)
	var out struct {
		History struct {
			Next struct {
				Start    int64 `json:"start"`
				PageSize uint  `json:"page_size"`
			} `json:"next"`
		}
		Data map[string]map[string][]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(b, &out))
	require.Len(t, out.Data["CR_MSG"], 2)
	require.Equal(t, uint(2), out.History.Next.PageSize)
	require.Equal(t, end.Unix()*1000, out.History.Next.Start)
}

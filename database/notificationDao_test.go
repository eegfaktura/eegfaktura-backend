package database

import (
	"at.ourproject/vfeeg-backend/model"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestFetchEdaHistory(t *testing.T) {
	var mockDb, err = GetDatabaseMock()
	require.NoError(t, err)

	start, _ := time.Parse(time.RFC3339Nano, "2023-10-03T17:00:00.000Z")
	end, _ := time.Parse(time.RFC3339Nano, "2023-10-04T18:00:00.000Z")

	stmt := "SELECT \"conversationId\", \"date\", \"direction\", \"issuer\", \"message\", \"protocol\", \"tenant\", \"type\" FROM \"base\".\"processhistory\" WHERE \\(\\(\"tenant\" = 'RC100298'\\) AND \\(\"protocol\" IS NOT NULL\\) AND \\(\"date\" BETWEEN '2023-10-03T17:00:00Z' AND '2023-10-04T18:00:00Z'\\)\\)"

	rows := sqlmock.NewRows([]string{"conversationId", "date", "direction", "issuer", "message", "protocol", "tenant", "type"}).
		AddRow("1", time.Now(), "CONSUMPTION", "ADMIN", "{}", "CR_MSG", "SEPP", model.EBMS_ONLINE_REG_APPROVAL)
	mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)
	//res, err := FetchEdaHistory(mockDb.OpenMockDb, "RC100298", (time.Now().Add(25 * time.Hour * -1)).UnixMilli(), time.Now().UnixMilli())
	res, err := FetchEdaHistory(mockDb.OpenMockDb, "RC100298", start.UnixMilli(), end.UnixMilli())
	require.NoError(t, err)

	for k, v := range res {
		fmt.Printf("K: %v\n", k)
		for _, e := range v {
			fmt.Printf("    V: %v\n", e)
		}
	}
}

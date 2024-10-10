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

	stmt := "SELECT \"conversationId\", \"date\", \"direction\", \"issuer\", \"message\", \"protocol\", \"tenant\", \"type\" FROM \"base\".\"processhistory\" WHERE \\(\\(\"tenant\" = 'RC100298'\\) AND \\(\"protocol\" IS NOT NULL\\)\\)"

	rows := sqlmock.NewRows([]string{"conversationId", "date", "direction", "issuer", "message", "protocol", "tenant", "type"}).
		AddRow("1", time.Now(), "CONSUMPTION", "ADMIN", "{}", "CR_MSG", "SEPP", model.EBMS_ONLINE_REG_APPROVAL)
	mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)
	res, err := FetchEdaHistory(mockDb.OpenMockDb, "RC100298")
	require.NoError(t, err)

	for k, v := range res {
		fmt.Printf("K: %v\n", k)
		for _, e := range v {
			fmt.Printf("    V: %v\n", e)
		}
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"at.ourproject/vfeeg-backend/database"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestParseHistoryPageSize(t *testing.T) {
	valid := map[string]uint{
		"0": 0, "1": 1, "50": 50, "1000": maxHistoryPageSize,
		"1001": maxHistoryPageSize, "18446744073709551616": maxHistoryPageSize, "+5": 5,
	}
	for in, want := range valid {
		got, err := parseHistoryPageSize(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "abc", "-1", "-9223372036854775808", "-99999999999999999999", "1.5"} {
		_, err := parseHistoryPageSize(in)
		require.Error(t, err, in)
	}
}

func TestFetchProcessHistory_pageSize(t *testing.T) {
	tests := []struct {
		ps     string
		status int
		query  string
	}{
		{ps: "-1", status: http.StatusBadRequest},
		{ps: "abc", status: http.StatusBadRequest},
		{ps: "0", status: http.StatusOK, query: `SELECT (.+) FROM "base"."processhistory" WHERE (.+) ORDER BY (.+) ASC$`},
		{ps: "1000", status: http.StatusOK, query: `SELECT (.+) LIMIT 1001$`},
		{ps: "1001", status: http.StatusOK, query: `SELECT (.+) LIMIT 1001$`},
		{ps: "99999999999999999999", status: http.StatusOK, query: `SELECT (.+) LIMIT 1001$`},
	}
	for _, tt := range tests {
		t.Run(tt.ps, func(t *testing.T) {
			db, mock, err := database.InitMockDatabase()
			require.NoError(t, err)
			if tt.query != "" {
				mock.ExpectQuery(tt.query).
					WillReturnRows(sqlmock.NewRows([]string{"conversationId"}))
			}

			r := httptest.NewRequest(http.MethodGet, "/process/history?start=0&end=1000&protocol=CR_MSG&ps="+tt.ps, nil)
			w := httptest.NewRecorder()
			NewProcessHandler(db).fetchProcessHistory()(w, r, nil, "TE000001")

			require.Equal(t, tt.status, w.Code)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

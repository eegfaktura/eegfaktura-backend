package database

import (
	"at.ourproject/vfeeg-backend/model"
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
	"io"
	"os"
	"testing"
)

func TestImportMasterdataFromExcel(t *testing.T) {
	var mockDb, err = GetDatabaseMock()
	require.NoError(t, err)

	reader, err := os.Open("../tests/TE100200-Muster-Stammdatenimport.xlsx")
	require.NoError(t, err)
	defer reader.Close()

	type args struct {
		dbConn   OpenDbXConnection
		r        io.Reader
		filename string
		sheet    string
		tenant   string
	}
	tests := []struct {
		name    string
		args    args
		prepare func()
		test    func(t *testing.T, args args)
	}{
		{
			name: "import file",
			args: args{
				dbConn:   mockDb.OpenMockDb,
				r:        reader,
				filename: "TE100200-Muster-Stammdatenimport.xlsx",
				sheet:    "EEG Stammdaten",
				tenant:   "TE100200",
			},
			prepare: func() {

			},
			test: func(t *testing.T, args args) {
				require.NoError(t, err)
				//mockDb.mock.ExpectExec("^INSERT INTO (.+) VALUES (.+)") //.WithArgs("firstname", "lastname")
				mockDb.Mock.ExpectQuery("^SELECT (.+)").WillReturnError(sql.ErrNoRows)
				mockDb.Mock.ExpectBegin()
				// , 'excel', 'Mustermann', '001', '2023-08-19T13:15:07.776003233Z', 'ACTIVE', '001-9876', 'TE100200', '', '', '', DEFAULT
				mockDb.Mock.ExpectQuery("^INSERT (.+)").WillReturnRows(sqlmock.NewRows([]string{"id"}).FromCSVString("1")) //.WillReturnResult(sqlmock.NewResult(1, 1)) //.WithArgs("firstname", "lastname")
				mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
				mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
				mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
				mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))

				require.NoError(t, ImportMasterdataFromExcel(args.dbConn, args.r, args.filename, args.sheet, args.tenant))
				require.NoError(t, mockDb.Mock.ExpectationsWereMet())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.test(t, tt.args)
		})
	}
}

func Test_transformExcelData(t *testing.T) {
	reader, err := os.Open("../tests/TE100200-Muster-Stammdatenimport.xlsx")
	require.NoError(t, err)
	defer reader.Close()

	f, err := openReader(reader, "TE100200-Muster-Stammdatenimport.xlsx")
	require.NoError(t, err)
	defer f.Close()

	rows, err := f.Rows("EEG Stammdaten")
	require.NoError(t, err)

	participants := transformExcelData(rows)

	findParticipant := func(n string, p []*model.EegParticipant) *model.EegParticipant {
		for i := range p {
			if p[i].LastName == n {
				return p[i]
			}
		}
		return &model.EegParticipant{}
	}

	assert.Equal(t, 5, len(participants))
	assert.Equal(t, 2, len(findParticipant("Finnegan", participants).MeteringPoint))
	assert.Equal(t, "001-3456", findParticipant("Finnegan", participants).TaxNumber)

	assert.Equal(t, null.StringFrom("003"), findParticipant("Finnegan", participants).ParticipantNumber)
	assert.Equal(t, null.StringFrom("005"), findParticipant("Beckett", participants).ParticipantNumber)
	assert.Equal(t, null.StringFrom("Silvia.Beckett@eegfaktura.at"), findParticipant("Beckett", participants).Contact.Email)

}

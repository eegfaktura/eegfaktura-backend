package database

import (
	"at.ourproject/vfeeg-backend/model"
	"bytes"
	"fmt"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"gopkg.in/guregu/null.v4"
	"io"
	"os"
	"testing"
	"time"
)

func Test_transformExcelData(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	reader, err := os.Open("../tests/TE100200-Muster-Stammdatenimport.xlsx")
	require.NoError(t, err)
	defer reader.Close()

	f, err := openReader(reader, "TE100200-Muster-Stammdatenimport.xlsx")
	require.NoError(t, err)
	defer f.Close()

	rows, err := f.Rows("EEG Stammdaten")
	require.NoError(t, err)

	participants := transformExcelData(rows, func(id string) string { return id })

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
	assert.Equal(t, null.StringFrom("001-3456"), findParticipant("Finnegan", participants).TaxNumber)

	assert.Equal(t, null.StringFrom("003"), findParticipant("Finnegan", participants).ParticipantNumber)
	assert.Equal(t, null.StringFrom("005"), findParticipant("Beckett", participants).ParticipantNumber)
	assert.Equal(t, null.StringFrom("Silvia.Beckett@eegfaktura.at"), findParticipant("Beckett", participants).Contact.Email)
	assert.Equal(t, null.StringFrom("AT009999"), findParticipant("Beckett", participants).MeteringPoint[0].GridOperatorId)
	assert.Equal(t, null.StringFrom("AT009999"), findParticipant("Beckett", participants).MeteringPoint[0].GridOperatorName)

}

func TestImportMasterdataFromExcel(t *testing.T) {
	log.SetLevel(log.DebugLevel)

	findParticipant := func(ps []model.EegParticipant, firstname, lastname string) *model.EegParticipant {
		for _, p := range ps {
			if p.FirstName == firstname && p.LastName == lastname {
				return &p
			}
		}
		return nil
	}

	reader, err := os.Open("../tests/TE100200-Muster-Stammdatenimport.xlsx")
	require.NoError(t, err)
	defer reader.Close()

	dbx, err := openTestDb()
	require.NoError(t, err)
	defer dbx.Close()

	type args struct {
		db       *sqlx.DB
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
				db:       dbx,
				r:        reader,
				filename: "TE100200-Muster-Stammdatenimport.xlsx",
				sheet:    "EEG Stammdaten",
				tenant:   "TE100200",
			},
			prepare: func() {

			},
			test: func(t *testing.T, args args) {
				require.NoError(t, ImportMasterdataFromExcel(args.db, args.r, args.filename, args.sheet, args.tenant))
				ps, err := GetParticipants(args.db, args.tenant)
				require.NoError(t, err)
				assert.Equal(t, 5, len(ps))

				p := findParticipant(ps, "Max", "Mustermann")
				require.NotNil(t, p)
				require.Equal(t, 1, len(p.MeteringPoint))

				assert.Equal(t, "Test Operator", p.MeteringPoint[0].GridOperatorName.String)
				assert.Equal(t, time.Date(time.Now().Year(), 1, 1, 0, 0, 0, 0, time.UTC).Local(), p.MeteringPoint[0].State.ActiveSince.Local())
				assert.Equal(t, time.Date(2999, 12, 31, 0, 0, 0, 0, time.UTC).Local(), p.MeteringPoint[0].State.InactiveSince.Local())
				assert.Equal(t, 0, p.MeteringPoint[0].State.Flag)
				assert.Equal(t, model.ACTIVE, p.MeteringPoint[0].Status)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.test(t, tt.args)
		})
	}
}

//func TestImportMasterdataFromExcel(t *testing.T) {
//	var mockDb, err = GetDatabaseMock()
//	require.NoError(t, err)
//
//	reader, err := os.Open("../tests/TE100200-Muster-Stammdatenimport.xlsx")
//	require.NoError(t, err)
//	defer reader.Close()
//
//	type args struct {
//		dbConn   OpenDbXConnection
//		r        io.Reader
//		filename string
//		sheet    string
//		tenant   string
//	}
//	tests := []struct {
//		name    string
//		args    args
//		prepare func()
//		test    func(t *testing.T, args args)
//	}{
//		{
//			name: "import file",
//			args: args{
//				dbConn:   mockDb.OpenMockDb,
//				r:        reader,
//				filename: "TE100200-Muster-Stammdatenimport.xlsx",
//				sheet:    "EEG Stammdaten",
//				tenant:   "TE100200",
//			},
//			prepare: func() {
//
//			},
//			test: func(t *testing.T, args args) {
//				require.NoError(t, err)
//				//mockDb.mock.ExpectExec("^INSERT INTO (.+) VALUES (.+)") //.WithArgs("firstname", "lastname")
//				//mockDb.Mock.ExpectQuery("^SELECT (.+)").WillReturnError(sql.ErrNoRows)
//				mockDb.Mock.ExpectBegin()
//				// , 'excel', 'Mustermann', '001', '2023-08-19T13:15:07.776003233Z', 'ACTIVE', '001-9876', 'TE100200', '', '', '', DEFAULT
//				mockDb.Mock.ExpectQuery("INSERT (.+)").WillReturnRows(sqlmock.NewRows([]string{"id"}).FromCSVString("1")) //.WillReturnResult(sqlmock.NewResult(1, 1)) //.WithArgs("firstname", "lastname")
//				mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
//				mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
//				mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
//				mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
//				//mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
//				//mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
//
//				require.NoError(t, ImportMasterdataFromExcel(args.dbConn, args.r, args.filename, args.sheet, args.tenant))
//				//require.NoError(t, mockDb.Mock.ExpectationsWereMet())
//			},
//		},
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			tt.test(t, tt.args)
//		})
//	}
//}

func TestExportMasterdataToExcel(t *testing.T) {
	db, err := openTestDb()
	require.NoError(t, err)
	defer db.Close()

	tenant := "TE000002"
	eeg, err := GetEeg(db, tenant)
	require.NoError(t, err)

	participants, err := GetParticipants(db, tenant)
	require.NoError(t, err)

	tariffMap, err := GetTariffNameMap(db, tenant)
	require.NoError(t, err)

	type args struct {
		participants []model.EegParticipant
		eeg          *model.Eeg
		tariffMap    map[string]string
	}
	tests := []struct {
		name    string
		args    args
		check   func(t *testing.T, bytes *bytes.Buffer)
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "Export Tenant",
			args: args{participants: participants, eeg: eeg, tariffMap: tariffMap},
			check: func(t *testing.T, buff *bytes.Buffer) {
				r := bytes.NewReader(buff.Bytes())
				f, err := openReader(r, "test")
				require.NoError(t, err)

				rows, err := f.Rows("Mitglieder")
				require.NoError(t, err)
				defer rows.Close()

				var cols [][]string
				for rows.Next() {
					c, err := rows.Columns(excelize.Options{RawCellValue: true})
					require.NoError(t, err)
					cols = append(cols, c)
					fmt.Printf("Col: %+v\n", c)
				}

				fmt.Printf("Street %v\n", cols[1][16])
				assert.Equal(t, 5, len(cols))
				assert.Equal(t, "6", cols[1][16])
			},
			wantErr: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExportMasterdataToExcel(tt.args.participants, tt.args.eeg, tt.args.tariffMap)
			if !tt.wantErr(t, err, fmt.Sprintf("ExportMasterdataToExcel(%v, %v, %v)", tt.args.participants, tt.args.eeg, tt.args.tariffMap)) {
				return
			}
			tt.check(t, got)
		})
	}
}

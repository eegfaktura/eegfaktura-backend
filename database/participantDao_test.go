package database

import (
	"at.ourproject/vfeeg-backend/model"
	dbsql "database/sql"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestUpdateParticipant(t *testing.T) {
	var tests = []struct {
		name     string
		line     func(table string, param interface{}) (sql string, params []interface{}, err error)
		params   interface{}
		database string
		result   []float64
	}{
		{
			name: "Test One",
			line: func(table string, param interface{}) (sql string, params []interface{}, err error) {
				sql, params, err = goqu.Insert("base.participant").Rows(param).ToSQL()
				return
			},
			params:   map[string]interface{}{"firstname": "hans"},
			database: "participant",
			result:   []float64{0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, _, _ := tt.line(tt.database, tt.params)
			println(sql)
		})
	}
}

func TestRegisterParticipant(t *testing.T) {

	participantJson := `{"businessRole":"EEG_PRIVATE","firstname":"Peter","lastname":"Obermüller","residentAddress":{"street":"Lambacherstraße","streetNumber":39,"zip":"4680","city":"Haag am Hausruck","type":"RESIDENCE"},"contact":{"phone":"06603611758","email":"obermueller.peter@gmail.com"},"accountInfo":{},"optionals":{},"status":"NEW","id":"e98b8619-7b6a-4836-baff-5489fb539535","role":"EEG_USER","billingAddress":{"street":"Lambacherstraße","streetNumber":39,"zip":"4680","city":"Haag am Hausruck","type":"BILLING"},"meters":[{"direction":"CONSUMPTION","status":"NEW","meteringPoint":"AT48124817243712897412","participantId":"e98b8619-7b6a-4836-baff-5489fb539535","tariffId":"a48d1990-a5a2-40c9-8d0a-77bed8e7dbcd","street":"Lambacherstraße","streetNumber":"39","zip":"4680","city":"Haag am Hausruck"}]}`

	var p model.EegParticipant
	err := json.NewDecoder(strings.NewReader(participantJson)).Decode(&p)
	assert.NoError(t, err)

	fmt.Printf("Participant: %+v\n", p)

	err = RegisterParticipant("RC200200", "petero", &p)
	assert.NoError(t, err)
}

func TestGetParticipant(t *testing.T) {
	participants, err := GetParticipant("RC100298")
	assert.NoError(t, err)

	assert.NotEmpty(t, participants)
	fmt.Printf("Participants: %+v\n", participants)
}

func Test_saveParticipant(t *testing.T) {
	type args struct {
		db                         *sqlx.DB
		tenant                     string
		username                   string
		participant                *model.EegParticipant
		registerMeteringPointsFunc func(*dbsql.Tx, string, string, []*model.MeteringPoint) error
	}

	mDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	participantJson := `{"businessRole":"EEG_PRIVATE","firstname":"Peter","lastname":"Obermüller","residentAddress":{"street":"Lambacherstraße","streetNumber":39,"zip":"4680","city":"Haag am Hausruck","type":"RESIDENCE"},"contact":{"phone":"06603611758","email":"obermueller.peter@gmail.com"},"accountInfo":{},"optionals":{},"status":"NEW","id":"e98b8619-7b6a-4836-baff-5489fb539535","role":"EEG_USER","billingAddress":{"street":"Lambacherstraße","streetNumber":39,"zip":"4680","city":"Haag am Hausruck","type":"BILLING"},"meters":[{"direction":"CONSUMPTION","status":"NEW","meteringPoint":"AT48124817243712897412","participantId":"e98b8619-7b6a-4836-baff-5489fb539535","tariffId":"a48d1990-a5a2-40c9-8d0a-77bed8e7dbcd","street":"Lambacherstraße","streetNumber":"39","zip":"4680","city":"Haag am Hausruck"}]}`

	var p model.EegParticipant
	err = json.NewDecoder(strings.NewReader(participantJson)).Decode(&p)
	assert.NoError(t, err)

	mdb := sqlx.NewDb(mDB, "mock")

	sql, _, _ := pgDialect.Insert("base.participant").Rows(p).Returning("id").ToSQL()

	mock.ExpectBegin()
	mock.ExpectQuery(sql).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("11"))
	mock.ExpectCommit()

	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "Save Participant", // TODO: Add test cases.
			args:    args{db: mdb, tenant: "te100001", username: "tester", participant: &p, registerMeteringPointsFunc: ImportMeteringPoints},
			wantErr: assert.NoError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := saveParticipant(tt.args.db, tt.args.tenant, tt.args.username, tt.args.participant, tt.args.registerMeteringPointsFunc)
			assert.NoError(t, mock.ExpectationsWereMet())
			require.NoError(t, err)

		})
	}
}

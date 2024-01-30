package database

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
	"strings"
	"testing"
	"time"
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

	mockDb, err := GetDatabaseMock()

	participantJson := `{"businessRole":"EEG_PRIVATE","firstname":"Peter","lastname":"Obermüller","residentAddress":{"street":"Lambacherstraße","streetNumber":"39","zip":"4680","city":"Haag am Hausruck","type":"RESIDENCE"},"contact":{"phone":"06603611758","email":"obermueller.peter@gmail.com"},"accountInfo":{},"optionals":{},"status":"NEW","id":"e98b8619-7b6a-4836-baff-5489fb539535","role":"EEG_USER","billingAddress":{"street":"Lambacherstraße","streetNumber":"39","zip":"4680","city":"Haag am Hausruck","type":"BILLING"},"meters":[{"direction":"CONSUMPTION","status":"NEW","meteringPoint":"AT48124817243712897412","participantId":"e98b8619-7b6a-4836-baff-5489fb539535","tariffId":"a48d1990-a5a2-40c9-8d0a-77bed8e7dbcd","street":"Lambacherstraße","streetNumber":"39","zip":"4680","city":"Haag am Hausruck"}]}`

	var p model.EegParticipant
	err = json.NewDecoder(strings.NewReader(participantJson)).Decode(&p)
	assert.NoError(t, err)

	fmt.Printf("Participant: %+v\n", p)

	mockDb.Mock.ExpectBegin()
	mockDb.Mock.ExpectQuery("INSERT (.+)").WillReturnRows(sqlmock.NewRows([]string{"id"}).FromCSVString("1")) //.WillReturnResult(sqlmock.NewResult(1, 1)) //.WithArgs("firstname", "lastname")
	mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
	mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
	mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
	mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
	//	mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
	mockDb.Mock.ExpectCommit()

	db, _ := mockDb.OpenMockDb()
	tx, err := db.Beginx()

	err = RegisterParticipant(tx, "RC200200", "petero", &p)

	assert.NoError(t, tx.Commit())
	assert.NoError(t, err)
}

func TestGetParticipant(t *testing.T) {
	mockDb, err := GetDatabaseMock()

	participantRows := sqlmock.NewRows([]string{
		"id", "firstname", "lastname", "role", "businessRole", "titleBefore", "titleAfter", "participantSince",
		"vatNumber", "taxNumber", "companyRegisterNumber", "status", "createdBy", //"createdDate", "lastModifiedBy", "lastModifiedDate",
		"version", "tariffId", "participantNumber"}).
		AddRow(uuid.New(), "Sepp", "Huber", "EEG_USER", "EEG_PRIVATE", "", "", time.Now(),
			"", "", "", "NEW", "admin", //time.Now(), "petero", time.Now(),
			1, uuid.New(), "001")
	mockDb.Mock.ExpectQuery("SELECT (.+) FROM \"base\".\"participant\" (.+)").WillReturnRows(participantRows)

	contactDetailsRows := sqlmock.NewRows([]string{"email", "phone"}).AddRow("mail@test.com", "+4325622 232311 32323")
	mockDb.Mock.ExpectQuery("SELECT (.+) FROM \"base\".\"contactdetail\" (.+)").WillReturnRows(contactDetailsRows)

	bankaccountRows := sqlmock.NewRows([]string{"iban", "owner"}).AddRow("AT12 3456 7987 9887 7765", "Sepp Huber")
	mockDb.Mock.ExpectQuery("SELECT (.+) FROM \"base\".\"bankaccount\" (.+)").WillReturnRows(bankaccountRows)

	addressRows := sqlmock.NewRows([]string{"city", "street", "streetNumber", "type", "zip"}).
		AddRow("Solarcity", "Energieweg", "12a", "BILLING", "1234")
	mockDb.Mock.ExpectQuery("SELECT (.+) FROM \"base\".\"address\" (.+)").WillReturnRows(addressRows)

	addressResidenceRows := sqlmock.NewRows([]string{"city", "street", "streetNumber", "type", "zip"}).
		AddRow("Solarcity", "Energieweg", "12a", "RESIDENCE", "1234")
	mockDb.Mock.ExpectQuery("SELECT (.+) FROM \"base\".\"address\" (.+)").WillReturnRows(addressResidenceRows)

	meterRows := sqlmock.NewRows([]string{"city", "direction", "equipmentName", "equipmentNumber", "inverterid", "metering_point_id",
		"modifiedAt", "modifiedBy", "registeredSince", "status", "street", "streetNumber", "tariff_id", "transformer", "zip"}).
		AddRow("Solarcity", "GENERATOR", "", "", "", "AT0020001110000010011111001",
			time.Now(), "admin", time.Now(), "NEW", "Energieweg", "12a", uuid.New(), "", "1234")
	mockDb.Mock.ExpectQuery("SELECT (.+) FROM \"base\".\"participant_meter_state\" (.+)").WillReturnRows(meterRows)

	participants, err := GetParticipants(mockDb.OpenMockDb, "RC100298")
	assert.NoError(t, err)

	assert.NotEmpty(t, participants)
	fmt.Printf("Participants: %+v\n", participants)
}

func Test_GetParticipants(t *testing.T) {
	participants, err := GetParticipants(openTestDb, "TE000002")
	require.NoError(t, err)

	require.Equal(t, 1, len(participants))
	p := participants[0]

	assert.Equal(t, "Peter", p.FirstName)
	assert.Equal(t, 4, len(p.MeteringPoint))

	findMeter := func(m []*model.MeteringPoint, mid string) *model.MeteringPoint {
		for i := range m {
			if m[i].MeteringPoint == mid {
				return m[i]
			}
		}
		return nil
	}

	expectedMeter := &model.MeteringPoint{
		MeteringPoint:    "AT0030000000000000000000030041724",
		Transformer:      null.String{},
		Direction:        model.GENERATOR,
		Status:           model.ACTIVE,
		TariffId:         null.StringFrom("f9b640dc-efe3-11ed-9f81-6ad19f4af00f"),
		EquipmentNumber:  null.StringFrom("GERZ02"),
		EquipmentName:    null.String{},
		InverterId:       null.String{},
		Street:           null.StringFrom("Imperndorf"),
		StreetNumber:     null.StringFrom("9"),
		City:             null.StringFrom("Waizenkirchen"),
		Zip:              null.StringFrom("4730"),
		RegisteredSince:  time.Date(2023, 8, 16, 0, 0, 0, 0, time.FixedZone("", 0)),
		ModifiedAt:       time.Date(2023, 11, 15, 17, 42, 41, 335283000, time.FixedZone("", 0)),
		ModifiedBy:       null.StringFrom("petero"),
		GridOperatorId:   null.String{},
		GridOperatorName: null.String{},
		State: &model.MeterState{
			ActiveSince:   time.Date(2023, 1, 1, 0, 0, 0, 0, time.FixedZone("", 0)),
			InactiveSince: time.Date(2999, 12, 31, 0, 0, 0, 0, time.FixedZone("", 0)),
			Active:        1,
			Flag:          0,
		},
	}
	m := findMeter(p.MeteringPoint, expectedMeter.MeteringPoint)
	assert.NotNil(t, m)
	assert.Equal(t, *expectedMeter.State, *m.State)
	assert.Equal(t, expectedMeter, m)
}

func Test_saveParticipant(t *testing.T) {
	type args struct {
		db                         *sqlx.DB
		tenant                     string
		username                   string
		participant                *model.EegParticipant
		registerMeteringPointsFunc func(*sqlx.Tx, string, string, string, []*model.MeteringPoint) error
	}

	mDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	participantJson := `{"businessRole":"EEG_PRIVATE","firstname":"Peter","lastname":"Obermüller","residentAddress":{"street":"Lambacherstraße","streetNumber":"39","zip":"4680","city":"Haag am Hausruck","type":"RESIDENCE"},"contact":{"phone":"06603611758","email":"obermueller.peter@gmail.com"},"accountInfo":{},"optionals":{},"status":"NEW","id":"e98b8619-7b6a-4836-baff-5489fb539535","role":"EEG_USER","billingAddress":{"street":"Lambacherstraße","streetNumber":"39","zip":"4680","city":"Haag am Hausruck","type":"BILLING"},"meters":[{"direction":"CONSUMPTION","status":"NEW","meteringPoint":"AT48124817243712897412","participantId":"e98b8619-7b6a-4836-baff-5489fb539535","tariffId":"a48d1990-a5a2-40c9-8d0a-77bed8e7dbcd","street":"Lambacherstraße","streetNumber":"39","zip":"4680","city":"Haag am Hausruck"}]}`

	var p model.EegParticipant
	err = json.NewDecoder(strings.NewReader(participantJson)).Decode(&p)
	assert.NoError(t, err)

	mdb := sqlx.NewDb(mDB, "mock")

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT (.+) \"base\".\"participant\"").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("11"))
	mock.ExpectExec("INSERT (.+) \"base\".\"contactdetail\"").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT (.+) \"base\".\"bankaccount\"").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT (.+) \"base\".\"address\"").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT (.+) \"base\".\"meteringpoint\"").WillReturnResult(sqlmock.NewResult(1, 1))
	//	mock.ExpectExec("INSERT (.+) \"base\".\"participant_meter_state\"").WillReturnResult(sqlmock.NewResult(1, 1))
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
			tx, err := tt.args.db.Beginx()
			assert.NoError(t, err)
			err = saveParticipant(tx, tt.args.tenant, tt.args.username, tt.args.participant, tt.args.registerMeteringPointsFunc)
			assert.NoError(t, tx.Commit())
			assert.NoError(t, mock.ExpectationsWereMet())
			require.NoError(t, err)

		})
	}
}

//func Test_findParticipantByMeteringPoint(t *testing.T) {
//	_, err := FindParticipantByMeteringPoint(nil, "TE100110", "AT0020000000000000000000020793777")
//	assert.NoError(t, err)
//}

func TestImportParticipant(t *testing.T) {

	var tests = []struct {
		name   string
		mp     string
		params *model.EegParticipant
		test   func(t *testing.T, p *model.EegParticipant)
	}{
		{
			name: "Test Import New Participant",
			mp:   "AT00300000000000000000000000000001",
			params: &model.EegParticipant{
				ParticipantNumber: null.String{},
				FirstName:         "Max",
				LastName:          "Mustermann",
				Contact:           model.ContactInfo{},
				BillingAddress: model.Address{
					Type:         model.BILLING,
					Street:       "Solargasse",
					StreetNumber: "11a",
					Zip:          "1111",
					City:         "Solarcity",
				},
				ResidentAddress: model.Address{
					Type:         model.RESIDENCE,
					Street:       "Solargasse",
					StreetNumber: "11a",
					Zip:          "1111",
					City:         "Solarcity",
				},
				BankAccount: model.BankInfo{},
				MeteringPoint: []*model.MeteringPoint{&model.MeteringPoint{
					MeteringPoint: "AT00300000000000000000000000000001",
					Transformer:   null.String{},
					Direction:     model.GENERATOR,
					Street:        null.StringFrom("Solargasse"),
					StreetNumber:  null.StringFrom("11a"),
					City:          null.StringFrom("Solarcity"),
					Zip:           null.StringFrom("1111"),
				}},
				Status: model.NEW,
			},
			test: func(t *testing.T, p *model.EegParticipant) {
				assert.Equal(t, 1, len(p.MeteringPoint))
				m := p.MeteringPoint[0]

				fmt.Printf("P: %+v\n", p.ParticipantSince)
				fmt.Printf("M: %+v\n", m)

				assert.Equal(t, time.Now().Truncate(24*time.Hour), p.ParticipantSince.Local())
				assert.Equal(t, time.Now().Truncate(24*time.Hour), m.RegisteredSince.Truncate(24*time.Hour).Local())
				assert.Equal(t, time.Now().Truncate(24*time.Hour), m.State.ActiveSince.Truncate(24*time.Hour).Local())
				assert.Equal(t, time.Date(2999, 12, 31, 1, 0, 0, 0, time.Local), m.State.InactiveSince.Local())

				assert.Equal(t, model.NEW, p.Status)
				assert.Equal(t, model.NEW, m.Status)

				assert.Equal(t, "Max", p.FirstName)
			},
		},
		{
			name: "Test Import Activated Participant",
			mp:   "AT00300000000000000000000000000002",
			params: &model.EegParticipant{
				ParticipantNumber: null.String{},
				FirstName:         "Maria",
				LastName:          "Mustermann",
				Contact:           model.ContactInfo{},
				BillingAddress: model.Address{
					Type:         model.BILLING,
					Street:       "Solargasse",
					StreetNumber: "11a",
					Zip:          "1111",
					City:         "Solarcity",
				},
				ResidentAddress: model.Address{
					Type:         model.RESIDENCE,
					Street:       "Solargasse",
					StreetNumber: "11a",
					Zip:          "1111",
					City:         "Solarcity",
				},
				BankAccount:      model.BankInfo{},
				ParticipantSince: time.Date(2023, 10, 6, 0, 0, 0, 0, time.UTC).Local(),
				MeteringPoint: []*model.MeteringPoint{&model.MeteringPoint{
					MeteringPoint:   "AT00300000000000000000000000000002",
					Transformer:     null.String{},
					Direction:       model.GENERATOR,
					Street:          null.StringFrom("Solargasse"),
					StreetNumber:    null.StringFrom("11a"),
					City:            null.StringFrom("Solarcity"),
					Zip:             null.StringFrom("1111"),
					Status:          model.ACTIVE,
					RegisteredSince: time.Date(2023, 10, 6, 0, 0, 0, 0, time.UTC),
				}},
				Status: model.ACTIVE,
			},
			test: func(t *testing.T, p *model.EegParticipant) {
				assert.Equal(t, 1, len(p.MeteringPoint))
				m := p.MeteringPoint[0]

				fmt.Printf("P: %+v\n", p.ParticipantSince)
				fmt.Printf("M: %+v\n", m)

				assert.Equal(t, time.Date(2023, 10, 6, 0, 0, 0, 0, time.UTC).Local(), p.ParticipantSince.Local())
				assert.Equal(t, time.Date(2023, 10, 6, 0, 0, 0, 0, time.UTC).Local(), m.RegisteredSince.Truncate(24*time.Hour).Local())
				assert.Equal(t, time.Date(2023, 10, 6, 0, 0, 0, 0, time.UTC).Local(), m.State.ActiveSince.Truncate(24*time.Hour).Local())
				assert.Equal(t, time.Date(2999, 12, 31, 1, 0, 0, 0, time.Local), m.State.InactiveSince.Local())

				assert.Equal(t, model.ACTIVE, p.Status)
				assert.Equal(t, model.ACTIVE, m.Status)

				assert.Equal(t, "Maria", p.FirstName)
			},
		},
		{
			name: "Test Import Participant - empty state",
			mp:   "AT00300000000000000000000000000003",
			params: &model.EegParticipant{
				ParticipantNumber: null.String{},
				FirstName:         "Helmut",
				LastName:          "Mustermann",
				Contact:           model.ContactInfo{},
				BillingAddress: model.Address{
					Type:         model.BILLING,
					Street:       "Solargasse",
					StreetNumber: "11a",
					Zip:          "1111",
					City:         "Solarcity",
				},
				ResidentAddress: model.Address{
					Type:         model.RESIDENCE,
					Street:       "Solargasse",
					StreetNumber: "11a",
					Zip:          "1111",
					City:         "Solarcity",
				},
				BankAccount: model.BankInfo{},
				MeteringPoint: []*model.MeteringPoint{&model.MeteringPoint{
					MeteringPoint: "AT00300000000000000000000000000003",
					Transformer:   null.String{},
					Direction:     model.GENERATOR,
					Street:        null.StringFrom("Solargasse"),
					StreetNumber:  null.StringFrom("11a"),
					City:          null.StringFrom("Solarcity"),
					Zip:           null.StringFrom("1111"),
				}},
			},
			test: func(t *testing.T, p *model.EegParticipant) {
				assert.Equal(t, 1, len(p.MeteringPoint))
				m := p.MeteringPoint[0]

				fmt.Printf("P: %+v\n", p.ParticipantSince)
				fmt.Printf("M: %+v\n", m)

				assert.Equal(t, time.Now().Truncate(24*time.Hour), p.ParticipantSince.Local())
				assert.Equal(t, time.Now().Truncate(24*time.Hour), m.RegisteredSince.Truncate(24*time.Hour).Local())
				assert.Equal(t, time.Now().Truncate(24*time.Hour), m.State.ActiveSince.Truncate(24*time.Hour).Local())
				assert.Equal(t, time.Date(2999, 12, 31, 1, 0, 0, 0, time.Local), m.State.InactiveSince.Local())

				assert.Equal(t, model.NEW, p.Status)
				assert.Equal(t, model.NEW, m.Status)

				assert.Equal(t, "Helmut", p.FirstName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := openTestDb()
			tx, err := db.Beginx()
			assert.NoError(t, err)

			err = ImportParticipant(tx, "TE000001", "test", tt.params)
			assert.NoError(t, err)

			tx.Commit()

			p, err := FindParticipantByMeteringPoint(db, "TE000001", tt.mp)
			assert.NoError(t, err)

			tt.test(t, p)
		})
	}
}

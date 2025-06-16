package database

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/doug-martin/goqu/v9"
	"github.com/jjeffery/civil"
	"github.com/jmoiron/sqlx"
	"github.com/mitchellh/mapstructure"
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
	mockDb.Mock.ExpectExec("INSERT (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
	mockDb.Mock.ExpectCommit()

	db, _ := mockDb.OpenMockDb()
	tx, err := db.Beginx()

	err = RegisterParticipant(tx, "RC200200", "petero", &p)

	assert.NoError(t, tx.Commit())
	assert.NoError(t, err)
}

func TestGetParticipant(t *testing.T) {
	mockDb, err := GetDatabaseMock()
	dbx := sqlx.NewDb(mockDb.db, "mock")

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

	participants, err := GetParticipants(dbx, "RC100298")
	assert.NoError(t, err)

	assert.NotEmpty(t, participants)
	fmt.Printf("Participants: %+v\n", participants)
}

func Test_GetParticipants(t *testing.T) {
	db, err := openTestDb()
	require.NoError(t, err)
	defer db.Close()

	participants, err := GetParticipants(db, "TE000002")
	require.NoError(t, err)

	require.Equal(t, 1, len(participants))
	p := participants[0]

	assert.Equal(t, "Peter", p.FirstName)
	assert.Equal(t, "Schulberg", p.ResidentAddress.Street.String)
	assert.Equal(t, "Sparberweg", p.BillingAddress.Street.String)
	assert.Nil(t, p.BankAccount.Iban.Ptr())

	assert.Equal(t, 5, len(p.MeteringPoint))

	findMeter := func(m []*model.MeteringPoint, mid string) *model.MeteringPoint {
		for i := range m {
			if m[i].MeteringPoint == mid {
				return m[i]
			}
		}
		return nil
	}

	expectedMeter := &model.MeteringPoint{
		MeteringPoint:    "AT0030000000000000000000030041725",
		Transformer:      null.String{},
		Direction:        model.GENERATOR,
		Status:           model.S_ACTIVE,
		ProcessState:     model.ACTIVE,
		TariffId:         null.StringFrom("f9b640dc-efe3-11ed-9f81-6ad19f4af00f"),
		EquipmentNumber:  null.StringFrom("GERZ02"),
		EquipmentName:    null.String{},
		InverterId:       null.String{},
		Street:           null.StringFrom("Imperndorf"),
		StreetNumber:     null.StringFrom("9"),
		City:             null.StringFrom("Waizenkirchen"),
		Zip:              null.StringFrom("4730"),
		RegisteredSince:  civil.DateFor(2023, 8, 16),
		ModifiedAt:       civil.DateTimeFor(2023, 11, 15, 17, 42, 41),
		ModifiedBy:       null.StringFrom("petero"),
		GridOperatorId:   null.String{},
		GridOperatorName: null.String{},
		State: &model.MeterState{
			ActiveSince:   civil.NullDate{Date: civil.DateOf(time.Date(2023, 1, 1, 0, 0, 0, 0, time.FixedZone("", 0))), Valid: true},
			InactiveSince: civil.NullDate{Date: civil.DateOf(time.Date(2999, 12, 31, 0, 0, 0, 0, time.FixedZone("", 0))), Valid: true},
			Active:        0,
			Flag:          1,
		},
		PartFact: 100,
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
	mock.ExpectExec("INSERT (.+) \"base\".\"metering_partition_factor\"").WillReturnResult(sqlmock.NewResult(1, 1))
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
				EegParticipantBase: model.EegParticipantBase{
					ParticipantNumber: null.String{},
					FirstName:         "Max",
					LastName:          "Mustermann",
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
				Contact: model.ContactInfo{},
				BillingAddress: model.Address{
					Type:         model.BILLING,
					Street:       null.StringFrom("Solargasse"),
					StreetNumber: null.StringFrom("11a"),
					Zip:          null.StringFrom("1111"),
					City:         null.StringFrom("Solarcity"),
				},
				ResidentAddress: model.Address{
					Type:         model.RESIDENCE,
					Street:       null.StringFrom("Solargasse"),
					StreetNumber: null.StringFrom("11a"),
					Zip:          null.StringFrom("1111"),
					City:         null.StringFrom("Solarcity"),
				},
				BankAccount: model.BankInfo{},
			},
			test: func(t *testing.T, p *model.EegParticipant) {
				assert.Equal(t, 1, len(p.MeteringPoint))
				m := p.MeteringPoint[0]

				fmt.Printf("P: %+v\n", p.ParticipantSince)
				fmt.Printf("M: %+v\n", m)

				assert.Equal(t, civil.Today(), p.ParticipantSince.Date)
				assert.Equal(t, civil.Today(), m.RegisteredSince)
				assert.Nil(t, m.State.ActiveSince.Ptr())
				assert.Nil(t, m.State.InactiveSince.Ptr())

				assert.Equal(t, model.NEW, p.Status)
				assert.Equal(t, model.S_INIT, m.Status)

				assert.Equal(t, "Max", p.FirstName)
			},
		},
		{
			name: "Test Import Activated Participant",
			mp:   "AT00300000000000000000000000000002",
			params: &model.EegParticipant{
				EegParticipantBase: model.EegParticipantBase{
					ParticipantNumber: null.String{},
					FirstName:         "Maria",
					LastName:          "Mustermann",
					ParticipantSince:  civil.NullDate{},
					MeteringPoint: []*model.MeteringPoint{&model.MeteringPoint{
						MeteringPoint:   "AT00300000000000000000000000000002",
						Transformer:     null.String{},
						Direction:       model.GENERATOR,
						Street:          null.StringFrom("Solargasse"),
						StreetNumber:    null.StringFrom("11a"),
						City:            null.StringFrom("Solarcity"),
						Zip:             null.StringFrom("1111"),
						ProcessState:    model.ACTIVE,
						RegisteredSince: civil.DateFor(2023, 10, 6),
					}},
					Status: model.ACTIVE,
				},
				Contact: model.ContactInfo{},
				BillingAddress: model.Address{
					Type:         model.BILLING,
					Street:       null.StringFrom("Solargasse"),
					StreetNumber: null.StringFrom("11a"),
					Zip:          null.StringFrom("1111"),
					City:         null.StringFrom("Solarcity"),
				},
				ResidentAddress: model.Address{
					Type:         model.RESIDENCE,
					Street:       null.StringFrom("Solargasse"),
					StreetNumber: null.StringFrom("11a"),
					Zip:          null.StringFrom("1111"),
					City:         null.StringFrom("Solarcity"),
				},
				BankAccount: model.BankInfo{},
			},
			test: func(t *testing.T, p *model.EegParticipant) {
				assert.Equal(t, 1, len(p.MeteringPoint))
				m := p.MeteringPoint[0]

				fmt.Printf("P: %+v\n", p.ParticipantSince)
				fmt.Printf("M: %+v\n", m)

				require.NotNil(t, p.ParticipantSince.Ptr())
				assert.Equal(t, civil.Today(), p.ParticipantSince.Date)
				assert.Equal(t, civil.DateFor(2023, 10, 6), m.RegisteredSince)
				assert.Equal(t, civil.DateFor(2023, 10, 6), m.State.ActiveSince.Date)
				assert.Equal(t, civil.DateFor(2999, 12, 31), m.State.InactiveSince.Date)

				assert.Equal(t, model.ACTIVE, p.Status)
				assert.Equal(t, model.S_ACTIVE, m.Status)

				assert.Equal(t, "Maria", p.FirstName)
			},
		},
		{
			name: "Test Import Participant - empty state",
			mp:   "AT00300000000000000000000000000003",
			params: &model.EegParticipant{
				EegParticipantBase: model.EegParticipantBase{
					ParticipantNumber: null.String{},
					FirstName:         "Helmut",
					LastName:          "Mustermann",
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
				Contact: model.ContactInfo{},
				BillingAddress: model.Address{
					Type:         model.BILLING,
					Street:       null.StringFrom("Solargasse"),
					StreetNumber: null.StringFrom("11a"),
					Zip:          null.StringFrom("1111"),
					City:         null.StringFrom("Solarcity"),
				},
				ResidentAddress: model.Address{
					Type:         model.RESIDENCE,
					Street:       null.StringFrom("Solargasse"),
					StreetNumber: null.StringFrom("11a"),
					Zip:          null.StringFrom("1111"),
					City:         null.StringFrom("Solarcity"),
				},
				BankAccount: model.BankInfo{},
			},
			test: func(t *testing.T, p *model.EegParticipant) {
				assert.Equal(t, 1, len(p.MeteringPoint))
				m := p.MeteringPoint[0]

				fmt.Printf("P: %+v\n", p.ParticipantSince)
				fmt.Printf("M: %+v\n", m)

				assert.Equal(t, civil.Today(), p.ParticipantSince.Date)
				assert.Equal(t, civil.Today(), m.RegisteredSince)
				assert.Nil(t, m.State.ActiveSince.Ptr())
				assert.Nil(t, m.State.InactiveSince.Ptr())

				assert.Equal(t, model.NEW, p.Status)
				assert.Equal(t, model.S_INIT, m.Status)

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

			err = tx.Commit()
			require.NoError(t, err)

			p, err := FindParticipantByMeteringPoint(db, "TE000001", tt.mp)
			assert.NoError(t, err)

			tt.test(t, p)
		})
	}
}

func TestUpdateParticipant1(t *testing.T) {
	db, _ := openTestDb()
	type args struct {
		tenant      string
		user        string
		participant *model.EegParticipant
	}
	tests := []struct {
		name    string
		args    args
		wantErr func(t *testing.T, p, e *model.EegParticipant)
	}{
		{
			name: "Update Participant",
			args: args{
				tenant: "TE000001",
				user:   "",
				participant: &model.EegParticipant{
					EegParticipantBase: model.EegParticipantBase{
						Id:                    uuid.Parse("ea9942da-03da-11ee-b82b-5a985b4b033a"),
						ParticipantNumber:     null.StringFrom("041"),
						BusinessRole:          "EEG_PRIVATE",
						Role:                  "EEG_USER",
						FirstName:             "Peter",
						LastName:              "Obermüller",
						TitleBefore:           null.String{},
						TitleAfter:            null.String{},
						ParticipantSince:      civil.NullDate{},
						VatNumber:             null.String{},
						TaxNumber:             null.String{},
						CompanyRegisterNumber: null.String{},
						TariffId:              null.String{},
						Status:                "ACTIVE",
						Version:               0,
						CreatedBy:             "petero",
					},
					Contact:         model.ContactInfo{},
					BillingAddress:  model.Address{Type: "BILLING"},
					ResidentAddress: model.Address{Type: "RESIDENT"},
					BankAccount:     model.BankInfo{},
				},
			},
			wantErr: func(t *testing.T, underTest, org *model.EegParticipant) {
				assert.Equal(t, "041", underTest.ParticipantNumber.String)
				fmt.Printf("ParticipantSince %v\n", underTest.ParticipantSince.Date.String())
				assert.Equal(t, civil.DateFor(2023, 10, 11), underTest.ParticipantSince.Date)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UpdateParticipant(db, tt.args.tenant, tt.args.user, tt.args.participant)
			assert.NoError(t, err)

			pUnderTest, err := QueryParticipant(db, tt.args.participant.Id.String())
			assert.NoError(t, err)

			tt.wantErr(t, pUnderTest, tt.args.participant)
		})
	}
}

func TestUpdateParticipantPartial(t *testing.T) {
	input := map[string]interface{}{"mandateDate": "2025-06-04T08:14:39.000Z"}
	var result model.BankInfo

	cfg := &mapstructure.DecoderConfig{
		Result:     &result,
		DecodeHook: StringToNullStringHookFunc,
	}
	decoder, err := mapstructure.NewDecoder(cfg)
	require.NoError(t, err)
	err = decoder.Decode(input)
	require.NoError(t, err)
}

package database

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
	"strings"
	"testing"
	"time"
)

func Test_RegisterMeteringPoint(t *testing.T) {
	type args struct {
		tenant        string
		participantId string
		point         *model.MeteringPoint
	}

	log.SetLevel(log.DebugLevel)

	tests := []struct {
		name string
		args args
	}{
		{
			name: "insert",
			args: args{tenant: "DR", participantId: "12", point: &model.MeteringPoint{
				MeteringPoint:   "",
				Transformer:     null.String{},
				Direction:       "",
				Status:          "",
				TariffId:        null.String{},
				EquipmentNumber: null.String{},
				EquipmentName:   null.String{},
				InverterId:      null.String{},
				Street:          null.String{},
				StreetNumber:    null.String{},
				City:            null.String{},
				Zip:             null.String{},
				RegisteredSince: time.Time{},
				ModifiedAt:      time.Time{},
				ModifiedBy:      null.String{},
				State:           nil,
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mock, err = GetDatabaseMock()
			dbx := sqlx.NewDb(mock.db, "mock")
			if err != nil {
				t.Fatalf("An error occurred while creating mock: %s", err)
			}
			defer mock.Close()

			mock.Mock.ExpectBegin()
			mock.Mock.ExpectExec("INSERT (.+) \"base\".\"meteringpoint\"").WillReturnResult(sqlmock.NewResult(1, 1))
			mock.Mock.ExpectExec("INSERT INTO \"base\".\"participant_meter_state\" (.+)").WillReturnResult(sqlmock.NewResult(1, 1))

			assert.NoError(t, RegisterMeteringPoint(dbx, tt.args.tenant, "userId", tt.args.participantId, tt.args.point))
		})
	}
}

func Test_MeteringPointRevoke(t *testing.T) {
	consentEnd := time.Now().Truncate(24 * time.Hour)
	db, err := openTestDb()
	require.NoError(t, err)
	err = MeteringPointRevoke(db, "TE000003", "AT0030000000000000000000030003010", "INACTIVE", consentEnd)
	assert.NoError(t, err)

	meters, err := FindInactiveMeteringById(db, "AT0030000000000000000000030003010")
	assert.NoError(t, err)
	require.NotNil(t, meters)
	require.Equal(t, 1, len(meters))

	meter := meters[0]

	assert.Equal(t, consentEnd.Add(1*time.Hour).Local(), meter.State.InactiveSince.Local())
	assert.Equal(t, model.INACTIVE, meter.Status)
}

func Test_AddMultipleMeteringPoints(t *testing.T) {

	meter := &model.MeteringPoint{
		MeteringPoint: "AT0030000000000000000000030000100",
		Direction:     model.GENERATOR,
		Street:        null.StringFrom("Solargasse"),
		StreetNumber:  null.StringFrom("1"),
		City:          null.StringFrom("Solarcity"),
		Zip:           null.StringFrom("1111"),
	}

	db, err := openTestDb()
	require.NoError(t, err)
	defer db.Close()

	// Register new Meteringpoint
	err = RegisterMeteringPoint(db, "TE000001", "test", "ea9942da-03da-11ee-b82b-5a985b4b033a", meter)
	require.NoError(t, err)

	newParticipant := &model.EegParticipant{
		BusinessRole:  "EEG_PRIVATE",
		Role:          "EEG_USER",
		FirstName:     "Michael",
		LastName:      "Obermüller",
		MeteringPoint: []*model.MeteringPoint{meter},
	}

	tx, err := db.Beginx()
	require.NoError(t, err)
	defer tx.Rollback()

	// Try to register new Participant with already registerd meteringpoint -> should fail
	err = RegisterParticipant(tx, "TE000001", "test", newParticipant)
	require.Error(t, err)
	require.NoError(t, tx.Rollback())
	//tx.Commit()

	// Send Revoke message to inactive meteringpoint
	consentEnd := time.Now().Truncate(42 * time.Hour).Local()
	err = MeteringPointRevoke(db, "TE000001", meter.MeteringPoint, "INACTIVE", consentEnd)
	assert.NoError(t, err)

	tx, err = db.Beginx()
	require.NoError(t, err)
	err = RegisterParticipant(tx, "TE000001", "test", newParticipant)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	consentEnd = time.Now().Truncate(42 * time.Hour).Local()
	err = MeteringPointRevoke(db, "TE000001", meter.MeteringPoint, "INACTIVE", consentEnd)
	assert.NoError(t, err)

	// Register new participant with revoked meteringpoint
	newParticipant.FirstName = "Paula"
	tx, err = db.Beginx()
	require.NoError(t, err)
	err = RegisterParticipant(tx, "TE000001", "test", newParticipant)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var names []string
	stmt, _, err := pgDialect.From("base.meteringpoint").Select("metering_point_id").
		Where(goqu.C("metering_point_id").Eq(meter.MeteringPoint)).ToSQL()
	fmt.Printf("STMT: %v\n", stmt)
	err = db.Select(&names, stmt)
	require.NoError(t, err)
	require.Equal(t, 3, len(names))

	findP := func(p []model.EegParticipant, firstname, lastname string) *model.EegParticipant {
		for i := range p {
			if p[i].FirstName == firstname && p[i].LastName == lastname {
				return &p[i]
			}
		}
		return nil
	}

	p, err := GetParticipants(db, "TE000001")
	require.NoError(t, err)

	assert.Equal(t, 3, len(p))

	p1 := findP(p, "Peter", "Obermüller")
	assert.NotNil(t, p1)
	assert.Equal(t, 1, len(p1.MeteringPoint))
	assert.Equal(t, model.INACTIVE, p1.MeteringPoint[0].Status)

	p2 := findP(p, "Paula", "Obermüller")
	assert.NotNil(t, p2)
	assert.Equal(t, 1, len(p2.MeteringPoint))
	assert.Equal(t, model.NEW, p2.MeteringPoint[0].Status)
}

func Test_ActivateRevokedMeteringPoint(t *testing.T) {
	meter := &model.MeteringPoint{
		MeteringPoint: "AT0030000000000000000000030000400",
		Direction:     model.GENERATOR,
		Street:        null.StringFrom("Solargasse"),
		StreetNumber:  null.StringFrom("1"),
		City:          null.StringFrom("Solarcity"),
		Zip:           null.StringFrom("1111"),
	}

	newParticipant := &model.EegParticipant{
		BusinessRole:  "EEG_PRIVATE",
		Role:          "EEG_USER",
		FirstName:     "Michael",
		LastName:      "Obermüller",
		MeteringPoint: []*model.MeteringPoint{meter},
	}

	db, err := openTestDb()
	require.NoError(t, err)
	defer db.Close()

	tx, err := db.Beginx()
	require.NoError(t, err)
	defer tx.Rollback()

	// Try to register new Participant with already registerd meteringpoint -> should fail
	err = RegisterParticipant(tx, "TE000004", "test", newParticipant)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	p, err := GetParticipants(db, "TE000004")
	require.NoError(t, err)

	for _, pp := range p {
		fmt.Printf("P: %+v\n", pp)
	}

	assert.Equal(t, 1, len(p))
	//participant := p[0]

	// Send Revoke message to inactive meteringpoint
	consentEnd := time.Now().Truncate(1 * time.Hour).Local()
	err = MeteringPointRevoke(db, "TE000004", meter.MeteringPoint, "INACTIVE", consentEnd)
	assert.NoError(t, err)

	p, err = GetParticipants(db, "TE000004")
	require.NoError(t, err)

	require.Equal(t, 1, len(p))
	require.Equal(t, 1, len(p[0].MeteringPoint))
	assert.Equal(t, model.INACTIVE, p[0].MeteringPoint[0].Status)
	assert.Equal(t, consentEnd.Add(1*time.Hour).Local(), p[0].MeteringPoint[0].State.InactiveSince.Local())
	assert.Equal(t, time.Now().Add(1*time.Hour).Local().Truncate(time.Hour), p[0].MeteringPoint[0].State.ActiveSince.Local().Truncate(time.Hour))

	err = MeteringPointsSetStatus(db, "TE000004", model.ACTIVE, []string{meter.MeteringPoint})
	require.NoError(t, err)

	p, err = GetParticipants(db, "TE000004")
	require.NoError(t, err)

	require.Equal(t, 1, len(p))
	require.Equal(t, 1, len(p[0].MeteringPoint))
	assert.Equal(t, model.ACTIVE, p[0].MeteringPoint[0].Status)
	assert.Equal(t, time.Date(2999, 12, 31, 23, 59, 59, 0, time.UTC), p[0].MeteringPoint[0].State.InactiveSince.Add(-1*time.Hour).UTC())
	assert.Equal(t, time.Now().Local().Truncate(time.Hour), p[0].MeteringPoint[0].State.ActiveSince.Add(-1*time.Hour).Local().Truncate(time.Hour))
}

func Test_RegistrationProcess(t *testing.T) {
	meter := &model.MeteringPoint{
		MeteringPoint: "AT0030000000000000000000030000401",
		Direction:     model.GENERATOR,
		Street:        null.StringFrom("Solargasse"),
		StreetNumber:  null.StringFrom("1"),
		City:          null.StringFrom("Solarcity"),
		Zip:           null.StringFrom("1111"),
	}

	newParticipant := &model.EegParticipant{
		BusinessRole:  "EEG_PRIVATE",
		Role:          "EEG_USER",
		FirstName:     "Registration",
		LastName:      "Test",
		MeteringPoint: []*model.MeteringPoint{meter},
	}

	findParticipantUnderTest := func(pp []model.EegParticipant) *model.EegParticipant {
		var pUt *model.EegParticipant
		for _, p := range pp {
			if p.FirstName == "Registration" && p.LastName == "Test" {
				pUt = &p
				break
			}
		}
		return pUt
	}

	db, err := openTestDb()
	require.NoError(t, err)
	defer db.Close()

	tx, err := db.Beginx()
	require.NoError(t, err)
	defer tx.Rollback()

	// Try to register new Participant with already registerd meteringpoint -> should fail
	err = RegisterParticipant(tx, "TE000004", "test", newParticipant)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	pp, err := GetParticipants(db, "TE000004")
	require.NoError(t, err)
	assert.Less(t, 0, len(pp))

	pUnderTest := findParticipantUnderTest(pp)
	require.NotNil(t, pUnderTest)

	err = ConfirmParticipant(db, "test", pUnderTest.Id.String())
	require.NoError(t, err)

	pp, err = GetParticipants(db, "TE000004")
	require.NoError(t, err)
	assert.Less(t, 0, len(pp))

	expectedRegistrationDate := time.Now()

	pUnderTest = findParticipantUnderTest(pp)
	require.NotNil(t, pUnderTest)
	require.Equal(t, model.ACTIVE, pUnderTest.Status)
	require.Equal(t, 1, len(pUnderTest.MeteringPoint))
	require.Equal(t, model.NEW, pUnderTest.MeteringPoint[0].Status)
	require.Equal(t, expectedRegistrationDate.Truncate(24*time.Hour).UTC(), pUnderTest.MeteringPoint[0].RegisteredSince.UTC())
	require.Equal(t, expectedRegistrationDate.Truncate(1*time.Hour).Add(1*time.Hour).UTC(), pUnderTest.MeteringPoint[0].State.ActiveSince.Truncate(1*time.Hour).UTC())

	err = MeteringPointsSetStatus(db, "TE000004", model.PENDING, []string{meter.MeteringPoint})
	require.NoError(t, err)

	m, err := FindMeteringById(db, meter.MeteringPoint)
	require.NoError(t, err)
	assert.Equal(t, model.PENDING, m.Status)

	err = MeteringPointsSetStatus(db, "TE000004", model.APPROVED, []string{meter.MeteringPoint})
	require.NoError(t, err)
	m, err = FindMeteringById(db, meter.MeteringPoint)
	require.NoError(t, err)
	assert.Equal(t, model.APPROVED, m.Status)

	err = MeteringPointsSetStatus(db, "TE000004", model.ACTIVE, []string{meter.MeteringPoint})
	require.NoError(t, err)
	m, err = FindMeteringById(db, meter.MeteringPoint)
	require.NoError(t, err)
	assert.Equal(t, model.ACTIVE, m.Status)
	assert.Equal(t, time.Now().Truncate(24*time.Hour), m.RegisteredSince.Local())

}

func Test_UpdateMeteringPoint(t *testing.T) {
	jsonObj := `{"meteringPoint":"AT0030000000000000000000030041724","transformer":null,"direction":"GENERATION","status":"ACTIVE","tariff_id":"f9b640dc-efe3-11ed-9f81-6ad19f4af00f","equipmentNumber":null,"equipmentName":"HARI PV","inverterid":null,"street":"Fellingerstraße","streetNumber":"9","city":"Waizenkirchen","zip":"4730","registeredSince":"2023-08-16T00:00:00Z","modifiedAt":"2023-08-16T16:36:09.076145Z","modifiedBy":null,"gridOperatorId":null,"gridOperatorName":null,"participantState":{"activeSince":"2022-01-01T00:00:00Z","inactiveSince":"2999-12-31T00:00:00Z"}}`

	m := model.MeteringPoint{}
	err := json.NewDecoder(strings.NewReader(jsonObj)).Decode(&m)
	require.NoError(t, err)

	db, err := openTestDb()
	require.NoError(t, err)
	defer db.Close()

	expectedRegistrationDate := time.Date(2023, 8, 16, 0, 0, 0, 0, time.UTC)
	expectedactiveDate := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	err = UpdateMeteringPoint(db, "TE000001", "test", "ea9942db-03da-11ee-b82b-5a985b4b033a", m.MeteringPoint, &m)
	require.NoError(t, err)

	mUnderTest, err := FindMeteringById(db, "AT0030000000000000000000030041724")
	require.NoError(t, err)

	require.Equal(t, expectedRegistrationDate.Truncate(24*time.Hour).UTC(), mUnderTest.RegisteredSince.UTC())
	require.Equal(t, expectedactiveDate.Truncate(1*time.Hour).UTC(), mUnderTest.State.ActiveSince.Truncate(1*time.Hour).UTC())
}

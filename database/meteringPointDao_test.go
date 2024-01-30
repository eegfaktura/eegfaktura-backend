package database

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/doug-martin/goqu/v9"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
	"strings"
	"testing"
	"time"
)

func TestRegisterMeteringPoint(t *testing.T) {
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

			if err != nil {
				t.Fatalf("An error occurred while creating mock: %s", err)
			}
			defer mock.Close()

			mock.Mock.ExpectBegin()
			mock.Mock.ExpectExec("INSERT (.+) \"base\".\"meteringpoint\"").WillReturnResult(sqlmock.NewResult(1, 1))
			mock.Mock.ExpectExec("INSERT INTO \"base\".\"participant_meter_state\" (.+)").WillReturnResult(sqlmock.NewResult(1, 1))

			assert.NoError(t, RegisterMeteringPoint(mock.OpenMockDb, tt.args.tenant, "userId", tt.args.participantId, tt.args.point))
		})
	}
}

func TestMeteringPointRevoke(t *testing.T) {
	consentEnd := time.Now().Truncate(24 * time.Hour).Local()
	db, err := openTestDb()
	require.NoError(t, err)
	err = MeteringPointRevoke(db, "TE000003", "AT0030000000000000000000030003010", "INACTIVE", consentEnd)
	assert.NoError(t, err)

	meters, err := FindInactiveMeteringById(openTestDb, "AT0030000000000000000000030003010")
	assert.NoError(t, err)
	require.NotNil(t, meters)
	require.Equal(t, 1, len(meters))

	meter := meters[0]

	assert.Equal(t, consentEnd, meter.State.InactiveSince.Local())
	assert.Equal(t, model.INACTIVE, meter.Status)
}

func TestAddMultipleMeteringPoints(t *testing.T) {

	meter := &model.MeteringPoint{
		MeteringPoint: "AT0030000000000000000000030000100",
		Direction:     model.GENERATOR,
		Street:        null.StringFrom("Solargasse"),
		StreetNumber:  null.StringFrom("1"),
		City:          null.StringFrom("Solarcity"),
		Zip:           null.StringFrom("1111"),
	}

	err := RegisterMeteringPoint(openTestDb, "TE000001", "test", "ea9942da-03da-11ee-b82b-5a985b4b033a", meter)
	require.NoError(t, err)

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

	err = RegisterParticipant(tx, "TE000001", "test", newParticipant)
	require.Error(t, err)
	require.NoError(t, tx.Rollback())
	//tx.Commit()

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

	newParticipant.FirstName = "Paula"
	tx, err = db.Beginx()
	require.NoError(t, err)
	err = RegisterParticipant(tx, "TE000001", "test", newParticipant)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	type metersQuery struct {
		meterId string `db:"metering_point"`
		//participantId string `db:"participant_id"`
		//active        int
		//inactiveSince time.Time `db:"inactivesince"`
	}
	//meters := []metersQuery{}
	var names []string
	stmt, _, err := pgDialect.From("base.meteringpoint").Select("metering_point_id").
		Where(goqu.C("metering_point_id").Eq(meter.MeteringPoint)).ToSQL()
	fmt.Printf("STMT: %v\n", stmt)
	err = db.Select(&names, stmt)
	require.NoError(t, err)

	require.Equal(t, 3, len(names))

	//findMeter := func(m []*model.MeteringPoint, mid string) *model.MeteringPoint {
	//	for i := range m {
	//		if m[i].MeteringPoint == mid {
	//			return m[i]
	//		}
	//	}
	//	return nil
	//}

	findP := func(p []model.EegParticipant, firstname, lastname string) *model.EegParticipant {
		for i := range p {
			if p[i].FirstName == firstname && p[i].LastName == lastname {
				return &p[i]
			}
		}
		return nil
	}

	p, err := GetParticipants(openTestDb, "TE000001")
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

func TestUpdateMeteringPoint(t *testing.T) {
	jsonObj := `{"meteringPoint":"AT0030000000000000000000030041724","transformer":null,"direction":"GENERATION","status":"ACTIVE","tariff_id":"f9b640dc-efe3-11ed-9f81-6ad19f4af00f","equipmentNumber":null,"equipmentName":"HARI PV","inverterid":null,"street":"Fellingerstraße","streetNumber":"9","city":"Waizenkirchen","zip":"4730","registeredSince":"2023-08-16T00:00:00Z","modifiedAt":"2023-08-16T16:36:09.076145Z","modifiedBy":null,"gridOperatorId":null,"gridOperatorName":null,"participantState":{"activeSince":"2022-01-01T00:00:00Z","inactiveSince":"2999-12-31T00:00:00Z"}}`

	m := model.MeteringPoint{}
	err := json.NewDecoder(strings.NewReader(jsonObj)).Decode(&m)
	require.NoError(t, err)

	err = UpdateMeteringPoint(openTestDb, "TE000001", "test", "ea9942db-03da-11ee-b82b-5a985b4b033a", m.MeteringPoint, &m)
	require.NoError(t, err)
}

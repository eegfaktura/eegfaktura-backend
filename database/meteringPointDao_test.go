package database

import (
	"at.ourproject/vfeeg-backend/model"
	"github.com/DATA-DOG/go-sqlmock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"gopkg.in/guregu/null.v4"
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

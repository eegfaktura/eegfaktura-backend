package eda

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/services"
	"bytes"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEdaRecorder_meteringPointPerformAnswerMsg(t *testing.T) {
	type args struct {
		ecId    string
		meterId []string
	}
	tests := []struct {
		name        string
		args        args
		prepareMock func() (sqlmock.Sqlmock, database.OpenDbXConnection)
		sendMail    services.SendMailFunc
		wantErr     assert.ErrorAssertionFunc
	}{
		{
			name: "Send Activation Mail",
			args: args{
				ecId:    "CC00000000000002221212121212",
				meterId: []string{"AT0030000000000000000000000410702"},
			},
			sendMail: func(tenant, to, subject string, cc *string, body *bytes.Buffer, inlineContent []*services.Attachment, attachment *services.Attachment) error {
				println(tenant, to, subject, cc, body, inlineContent, attachment)
				return nil
			},
			prepareMock: func() (sqlmock.Sqlmock, database.OpenDbXConnection) {
				mockDb, err := database.GetDatabaseMock()
				require.NoError(t, err)

				stmt := "SELECT (.+) FROM \"base\".\"eeg\" WHERE (.+)"
				rows := sqlmock.NewRows([]string{"tenant", "name", "description", "businessNr", "legal", "gridoperator_name", "communityId", "gridoperator_code", "rcNumber", "area", "allocationMode",
					"settlementInterval", "providerBusinessNr", "street", "streetNumber", "zip", "city", "phone", "email", "website", "iban", "owner", "bankName",
					"taxNumber", "vatNumber", "online", "contactPerson"}).
					AddRow("TE1000001", "test-eeg", "", "", "verein", "Netz-Test", "CC00000000000002221212121212", "EE000001", "RC100130",
						"LOCAL", "DYNAMIC", "MONTHLY", 0, "Solargasse", "1", "1111", "Solarcity", "", "obermueller.peter@gmail.com", "", "", "Max Mustermann", "Bankname", "", "", false, "Max Mustermann")
				mockDb.Mock.ExpectQuery(stmt).WillReturnRows(rows)

				mockDb.Mock.ExpectBegin()
				uuidParticipant := uuid.Parse("ea9942da-03da-11ee-b82b-5a985b4b033a")

				stmtParticipant := "SELECT (.+) FROM \"base\".\"participant\" (.+)"
				rowsParticipant := sqlmock.NewRows([]string{
					"accountInfo.bankName",
					"accountInfo.iban",
					"accountInfo.owner",
					"billingAddress.city",
					"billingAddress.street",
					"billingAddress.streetNumber",
					"billingAddress.type",
					"billingAddress.zip",
					"businessRole",
					"companyRegisterNumber",
					"contact.email",
					"contact.phone",
					"createdBy",
					"firstname",
					"id",
					"lastname",
					"participantNumber",
					"participantSince",
					"residentAddress.city",
					"residentAddress.street",
					"residentAddress.streetNumber",
					"residentAddress.type",
					"residentAddress.zip",
					"role",
					"status",
					"tariffId",
					"taxNumber",
					"titleAfter",
					"titleBefore",
					"vatNumber",
					"version"}).
					AddRow("accountInfo.bankName",
						"accountInfo.iban",
						"accountInfo.owner",
						"billingAddress.city",
						"billingAddress.street",
						"billingAddress.streetNumber",
						"BILLING",
						"billingAddress.zip",
						"businessRole",
						"companyRegisterNumber",
						"contact.email",
						"contact.phone",
						"createdBy",
						"firstname",
						uuidParticipant.String(),
						"lastname",
						"participantNumber",
						"2024-01-01",
						"Solarcity",
						"Solargasse",
						"11",
						"RESIDENCE",
						"1111",
						"role",
						"status",
						"tariffId",
						"taxNumber",
						"titleAfter",
						"titleBefore",
						"vatNumber",
						1)
				mockDb.Mock.ExpectQuery(stmtParticipant).WillReturnRows(rowsParticipant)

				stmtMeteringPoint := "SELECT (.+)"
				rowsMeteringPoint := sqlmock.NewRows([]string{
					"allocation_factor",
					"city",
					"consent_id",
					"direction",
					"equipmentName", "equipmentNumber",
					"grid_operator_id", "grid_operator_name",
					"inverterid",
					"metering_point_id",
					"modifiedAt",
					"modifiedBy",
					"partFact",
					"process_state",
					"registeredSince",
					"state.activesince", "state.flag", "state.inactivesince", "status", "statusCode", "street", "streetNumber", "tariff_id", "transformer", "zip"}).
					AddRow(
						100.0,
						"city",
						"consent_id",
						"direction",
						"equipmentName", "equipmentNumber",
						"grid_operator_id", "grid_operator_name",
						"inverterid",
						"AT0030000000000000000000000410702",
						"2024-01-01",
						"modifiedBy",
						100,
						"process_state",
						"2024-01-01",
						"2024-01-01", model.F_ASSIGNED, "2999-01-01", "status", 0, "street", "streetNumber", "", "transformer", "zip")
				mockDb.Mock.ExpectQuery(stmtMeteringPoint).WillReturnRows(rowsMeteringPoint)
				mockDb.Mock.ExpectCommit()
				//mockDb.Mock.ExpectClose()

				return mockDb.Mock, mockDb.OpenMockDb
			},
			wantErr: assert.NoError,
		},

		// TODO: Add test cases.

	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockdb, dbOpen := tt.prepareMock()
			r := &EdaRecorder{
				dbOpen: dbOpen,
			}
			tt.wantErr(t, r.meteringPointPerformAnswerMsg(tt.sendMail, tt.args.ecId, tt.args.meterId), fmt.Sprintf("meteringPointPerformAnswerMsg(%v, %v)", tt.args.ecId, tt.args.meterId))
			mockdb.MatchExpectationsInOrder(true)
		})
	}
}

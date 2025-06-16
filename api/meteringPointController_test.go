package api

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jjeffery/civil"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestMeteringPointValues(t *testing.T) {
	type args struct {
		tenant      string
		request     string
		mqttReqFunc func(eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error
	}

	tests := []struct {
		name  string
		args  args
		check func(t *testing.T, recorder *httptest.ResponseRecorder)
	}{
		{
			name: "Update EEG",
			args: args{
				tenant:  "TE100100",
				request: `{"meteringPoints": [{"meter": "AT000000000000000000001", "direction": "CONSUMPTION"}], "from": 1212001200120012, "to": 23423434243234234}`,
				mqttReqFunc: func(eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error {
					return nil
				},
			},
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				assert.Equal(t, http.StatusCreated, recorder.Code)
			},
		},
		{
			name: "Update EEG - sepp", // TODO: Add test cases.
			args: args{
				tenant:  "TE100100",
				request: fmt.Sprintf(`{"meteringPoints": [{"meter": "AT000000000000000000001", "direction": "CONSUMPTION"}], "from": %d, "to": 23423434243234234}`, time.Date(2023, time.Month(11), 1, 0, 0, 0, 0, time.Local).UnixMilli()),
				mqttReqFunc: func(eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error {
					fmt.Printf("FromDate %s\n", time.UnixMilli(fromDate).String())
					assert.Equal(t, civil.DateFor(2024, 1, 1).Unix()*1000, fromDate)
					return nil
				},
			},
			check: func(t *testing.T, recorder *httptest.ResponseRecorder) {
				fmt.Printf("recorder: %v\n", recorder)
				assert.Equal(t, http.StatusCreated, recorder.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mockDb, err = database.GetDatabaseMock()
			require.NoError(t, err)
			defer mockDb.Close()

			database.ConnectToDatabase = func() (*sqlx.DB, error) {
				return mockDb.OpenMockDb()
			}

			rows := sqlmock.NewRows([]string{"name", "description", "\"businessNr\"", "legal", "gridoperator_name", "\"communityId\"", "gridoperator_code", "\"rcNumber\"", "area", "\"allocationMode\"",
				"\"settlementInterval\"", "providerBusinessNr", "street", "\"streetNumber\"", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "\"bankName\"",
				"\"taxNumber\"", "\"vatNumber\"", "online", "\"contactPerson\""}).
				AddRow("TEST_EEG", "Test EEG", "", "verein", "Netz Test", "AT000000000000000001", "AT009999", "RC000001", "LOCAL", "DYNAMIC", "", 0,
					"Solargasse", "10", "1111", "Solarcity", "", "", "", "", "test", false, "", "", "", true, "")
			mockDb.Mock.ExpectQuery("^SELECT (.+)").WillReturnRows(rows)

			activated := time.Date(2024, time.Month(1), 1, 0, 0, 0, 0, time.Local)
			inactivated := time.Date(2999, time.Month(1), 1, 0, 0, 0, 0, time.Local)

			rows = sqlmock.NewRows([]string{"city", "direction", "equipmentName", "equipmentNumber", "grid_operator_id", "grid_operator_name", "inverterid",
				"metering_point_id", "modifiedAt", "modifiedBy", "registeredSince", "state.flag", "state.activesince", "state.inactivesince", "status", "process_state", "street", "streetNumber", "tariff_id",
				"transformer", "zip"}).
				AddRow("Solarcity", "CONSUMPTION", "", "", "AT009999", "Netz Test", "", "AT009999999999999999999999", time.Now(), "", time.Now(), 1,
					activated, inactivated, "ACTIVE", "ACTIVE", "Solargasse", "1", nil, nil, "1111",
				)
			mockDb.Mock.ExpectQuery("^SELECT (.+)").WillReturnRows(rows)

			req, _ := http.NewRequest("POST", "/meteringpoint/syncenergy", strings.NewReader(tt.args.request))
			w := httptest.NewRecorder()
			mqttclient.RequestingEnergyData = tt.args.mqttReqFunc
			requestMeteringPointValues()(w, req, nil, tt.args.tenant)
			tt.check(t, w)
		})
	}
}

func TestRequestChangePartitionFactor(t *testing.T) {
	var mockDb, err = database.GetDatabaseMock()
	require.NoError(t, err)
	defer mockDb.Close()

	database.ConnectToDatabase = func() (*sqlx.DB, error) {
		return mockDb.OpenMockDb()
	}

	rows := sqlmock.NewRows([]string{"name", "description", "\"businessNr\"", "legal", "gridoperator_name", "\"communityId\"", "gridoperator_code", "\"rcNumber\"", "area", "\"allocationMode\"",
		"\"settlementInterval\"", "providerBusinessNr", "street", "\"streetNumber\"", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "\"bankName\"",
		"\"taxNumber\"", "\"vatNumber\"", "online", "\"contactPerson\""}).
		AddRow("TEST_EEG", "Test EEG", "", "verein", "Netz Test", "AT000000000000000001", "AT009999", "RC000001", "LOCAL", "DYNAMIC", "", 0,
			"Solargasse", "10", "1111", "Solarcity", "", "", "", "", "test", false, "", "", "", true, "")
	mockDb.Mock.ExpectQuery("^SELECT (.+)").WillReturnRows(rows)

	data := `{"meteringPoints":[{"meter":"AT0020000000000000000000020901172","direction":"GENERATION","activation":"2022-01-01","partFact":1}]}`
	req, _ := http.NewRequest("POST", "/meteringpoint/changepartitionfactor", strings.NewReader(data))
	w := httptest.NewRecorder()
	mqttclient.ChangePartitionFactor = func(eeg *model.Eeg, meter []*model.ChangePartitionFactorRequest) error {
		assert.Equal(t, "TE100100", eeg.Id)
		return nil
	}

	requestChangePartitionFactor()(w, req, nil, "TE100100")

	assert.Equal(t, http.StatusCreated, w.Code)
}

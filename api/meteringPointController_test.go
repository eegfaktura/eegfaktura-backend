package api

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

//func executeRequest(req *http.Request) *httptest.ResponseRecorder {
//	rr := httptest.NewRecorder()
//	a.Router.ServeHTTP(rr, req)
//
//	return rr
//}

func TestRequestMeteringPointValues(t *testing.T) {
	var mockDb, err = database.GetDatabaseMock()
	require.NoError(t, err)
	defer mockDb.Close()

	database.ConnectToDatabase = func() (*sqlx.DB, error) {
		return mockDb.OpenMockDb()
	}

	request := `{"meteringPoints": [{"meter": "AT000000000000000000001", "direction": "CONSUMPTION"}], "from": 1212001200120012, "to": 23423434243234234}`

	rows := sqlmock.NewRows([]string{"name", "description", "\"businessNr\"", "legal", "gridoperator_name", "\"communityId\"", "gridoperator_code", "\"rcNumber\"", "area", "\"allocationMode\"",
		"\"settlementInterval\"", "providerBusinessNr", "street", "\"streetNumber\"", "zip", "city", "phone", "email", "website", "iban", "owner", "sepa", "\"bankName\"",
		"\"taxNumber\"", "\"vatNumber\"", "online", "\"contactPerson\""}).
		AddRow("TEST_EEG", "Test EEG", "", "verein", "Netz Test", "AT000000000000000001", "AT009999", "RC000001", "LOCAL", "DYNAMIC", "", 0,
			"Solargasse", "10", "1111", "Solarcity", "", "", "", "", "test", false, "", "", "", true, "")
	mockDb.Mock.ExpectQuery("^SELECT (.+)").WillReturnRows(rows)

	rows = sqlmock.NewRows([]string{"city", "direction", "equipmentName", "equipmentNumber", "grid_operator_id", "grid_operator_name", "inverterid",
		"metering_point_id", "modifiedAt", "modifiedBy", "registeredSince", "state.active", "state.activesince", "state.inactivesince", "status", "street", "streetNumber", "tariff_id",
		"transformer", "zip"}).AddRow("Solarcity", "CONSUMPTION", "", "", "AT009999", "Netz Test", "", "AT009999999999999999999999", time.Now(), "", time.Now(), 1,
		time.Date(2024, time.Month(1), 1, 0, 0, 0, 0, time.Local), time.Date(2999, time.Month(1), 1, 0, 0, 0, 0, time.Local),
		"ACTIVE", "Solargasse", "1", nil, nil, "1111",
	)
	mockDb.Mock.ExpectQuery("^SELECT (.+)").WillReturnRows(rows)

	//rows = sqlmock.NewRows([]string{"city", "direction", "equipmentName", "equipmentNumber", "grid_operator_id", "grid_operator_name", "inverterid",
	//	"metering_point_id", "modifiedAt", "modifiedBy", "registeredSince", "active", "activesince", "inactivesince", "status", "street", "streetNumber", "tariff_id",
	//	"transformer", "zip"}).AddRow("Solarcity", "CONSUMPTION", "", "", "AT009999", "Netz Test", "", "AT009999999999999999999999", time.Now(), "", time.Now(), 1,
	//	time.Date(2024, time.Month(1), 1, 0, 0, 0, 0, time.Local), time.Date(2999, time.Month(1), 1, 0, 0, 0, 0, time.Local),
	//	"ACTIVE", "Solargasse", "1", nil, nil, "1111",
	//)
	//mockDb.Mock.ExpectQuery("^SELECT (.+)").WillReturnRows(rows)

	req, _ := http.NewRequest("POST", "/meteringpoint/syncenergy", strings.NewReader(request))
	w := httptest.NewRecorder()

	mqttclient.RequestingEnergyData = func(tenant string, eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error {
		return nil
	}

	requestMeteringPointValues()(w, req, nil, "tenant")

	assert.Equal(t, http.StatusCreated, w.Code)
}

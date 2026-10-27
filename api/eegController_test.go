package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDB *database.TestDatabase

func openTestDb() (*sqlx.DB, error) {
	err := testDB.Open(context.Background())
	if err != nil {

	}
	return testDB.DbInstance, nil
}

func TestMain(m *testing.M) {
	//testDB = database.SetupTestDatabase()
	//defer testDB.TearDown()

	testDB = database.SetupTestDatabase()
	db, err := database.GetTestDB(context.Background(), testDB)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = db.CloseDB()
		testDB.TearDown()
	}()

	os.Exit(m.Run())
}

func TestMarschaling(t *testing.T) {
	//jsonStr := `{"id":"","name":"Mein Einspeise Traif","type":"EZP","useVat":false,"baseFee":"0","accountGrossAmount":0,"participantFee":0,"accountNetAmount":0,"billingPeriod":"monthly","businessNr":0,"centPerKWh":"0.12","discount":0,"freeKWH":0,"vatInPercent":0}`
	jsonStr := `{"id":"",
"name":"Mein Einspeise Traif",
"type":"EZP",
"useVat":false,
"baseFee":"0",
"accountGrossAmount":"0",
"participantFee":0,
"accountNetAmount":"0",
"billingPeriod":"monthly",
"businessNr":"0",
"centPerKWh":12.0,
"discount":"0",
"freeKWH":"0",
"vatSupplementaryText": "",
"vatInPercent":"0"}`

	var r model.Tariff
	err := json.Unmarshal([]byte(jsonStr), &r)
	require.NoError(t, err)

	fmt.Printf("R: %+v\n", r)
}

// A rejected tariff tells the client why: the validation message is in the error body (it was "{}" before,
// because a plain error marshals to an empty JSON object).
func TestAddTariff_validationMessageInResponse(t *testing.T) {
	db, err := database.GetDB(context.Background())
	require.NoError(t, err)
	h := NewEegHandler(db)

	body := `{"name":"ZVT an EEG","type":"EEG","useTimeTariff":true}`
	req := httptest.NewRequest(http.MethodPost, "/eeg/tariff", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.addTariff()(w, req, &middleware.PlatformClaims{Username: "test"}, "TE000002")

	require.Equal(t, http.StatusBadRequest, w.Code)
	var resp struct {
		Error HttpError `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, 4001, resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "zeitbasierter Tarif ist nur für Verbraucher- und Erzeuger-Tarife zulässig")
}

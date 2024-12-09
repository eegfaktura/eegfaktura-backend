package database

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
	"strings"
	"testing"
)

func TestGetEeg(t *testing.T) {
	db, err := openTestDb()
	require.NoError(t, err)

	eeg, err := GetEeg(db, "TE000001")
	assert.NoError(t, err)

	expectedEeg := &model.Eeg{
		Id:                 "TE000001",
		Name:               "MY-TEST",
		Description:        "Gemeinnütziger Verein",
		BusinessNr:         null.StringFrom("123456789"),
		Area:               "LOCAL",
		Legal:              "verein",
		OperatorName:       "Netz OOE",
		CommunityId:        "AT00300000000TC000001000000000001",
		GridOperator:       "AT003000",
		RcNumber:           "TE000001",
		AllocationMode:     "DYNAMIC",
		SettlementInterval: "MONTHLY",
		ProviderBusinessNr: null.Int{},
		TaxNumber:          null.StringFrom("11 123/4567"),
		VatNumber:          null.String{},
		ContactPerson:      null.StringFrom("Max Sonnenmann"),
		EegAddress: model.EegAddress{
			Street:       "Solarstraße",
			StreetNumber: "9",
			Zip:          "1111",
			City:         "Solarcity",
		},
		AccountInfo: model.AccountInfo{
			Iban:     null.StringFrom("AT011234000000321321"),
			Owner:    null.StringFrom("T-VIERE"),
			BankName: null.String{},
			Sepa:     false,
		},
		Contact: model.Contact{
			Phone: null.StringFrom("0043-664-1234567"),
			Email: null.StringFrom("test-eeg@gmx.at"),
		},
		Optionals: model.Optionals{Website: null.StringFrom("test-eeg.at")},
		//Periods:   nil,
		Online: false,
	}
	assert.Equal(t, expectedEeg, eeg)
}

func TestUpdateEeg(t *testing.T) {
	mDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	eegJson := `{
            "id": "TE100100",
            "name": "T-VIERE",
            "businessNr": "123456789",
            "area": "",
            "legal": "verein",
            "operatorName": "Netz OOE",
            "communityId": "AT00300000000TC100100000000000001",
            "gridOperator": "AT003000",
            "rcNumber": "TE100100",
            "allocationMode": "DYNAMIC",
            "settlementInterval": "MONTHLY",
            "providerBusinessNr": null,
            "taxNumber": "11 123/4567",
            "vatNumber": null,
            "contactPerson": "",
            "address": {
                "type": "",
                "street": "Solarstraße",
                "streetNumber": "9",
                "zip": "1111",
                "city": "Solarcity"
            },
            "accountInfo": {
                "iban": "AT011234000000321321",
                "owner": "T-VIERE",
                "sepa": false
            },
            "contact": {
                "phone": "0043-664-1234567",
                "email": "test-eeg@gmx.at"
            },
            "optionals": {
                "website": "test-eeg.at"
            },
            "periods": null,
            "online": false
        }`

	var eeg model.Eeg
	err = json.NewDecoder(strings.NewReader(eegJson)).Decode(&eeg)
	assert.NoError(t, err)

	mdb := sqlx.NewDb(mDB, "mock")

	type args struct {
		tenant string
		eeg    *model.Eeg
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
	}{
		{name: "Update EEG", // TODO: Add test cases.
			args:    args{tenant: "TE100100", eeg: &eeg},
			wantErr: assert.NoError}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.ExpectExec("INSERT INTO (.+)").WillReturnResult(sqlmock.NewResult(1, 1))
			tt.wantErr(t, InsertEeg(mdb, tt.args.tenant, tt.args.eeg), fmt.Sprintf("InsertEeg(%v, %+v)", tt.args.tenant, tt.args.eeg))
			assert.NoError(t, mock.ExpectationsWereMet())
			require.NoError(t, err)
		})
	}
}

func TestNotification(t *testing.T) {
	db, err := openTestDb()
	require.NoError(t, err)
	defer db.Close()

	err = SaveNotification(openTestDb, "TE000001", `{"msg":"hello world"}`, model.N_TYPE_NOTIFICATION, model.N_PROCESS_EDA_PROCESS, "ADMIN")
	assert.NoError(t, err)

	not, err := GetNotification(db, "TE000001", 0, true)
	assert.NoError(t, err)

	assert.NotEmpty(t, not)
}

func TestGetEegById(t *testing.T) {
	db, err := openTestDb()
	require.NoError(t, err)
	defer db.Close()

	eeg, err := GetEegById(db, "TE000001")
	assert.NoError(t, err)

	println(eeg)
}

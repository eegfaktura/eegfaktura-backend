package database

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/eegfaktura/eegfaktura-backend/model"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//func TestGetMeteringPoint(t *testing.T) {
//	eeg, err := GetEeg("RC100181")
//	assert.NoError(t, err)
//
//	assert.NotEmpty(t, eeg)
//	fmt.Printf("EEG: %+v\n", eeg)
//}

func TestUpdateEeg(t *testing.T) {
	mDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	eegJson := `{
            "id": "TE100100",
            "name": "T-VIERE",
            "businessNr": 123456789,
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
			tt.wantErr(t, UpdateEeg(mdb, tt.args.tenant, tt.args.eeg), fmt.Sprintf("UpdateEeg(%v, %+v)", tt.args.tenant, tt.args.eeg))
			assert.NoError(t, mock.ExpectationsWereMet())
			require.NoError(t, err)
		})
	}
}

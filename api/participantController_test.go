package api

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"fmt"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func init() {
	viper.Set("database.host", "localhost")
	viper.Set("database.port", 6432)
	viper.Set("database.user", "postgresuser")
	viper.Set("database.password", "postgresPW")
	viper.Set("database.dbname", "postgresdb")
}

func Test_registerParticipant(t *testing.T) {
	registerObject := `{
		"id": "",
		"participantNumber": "072",
		"participantSince": "2006-01-02T15:04:05Z",
		"firstname": "Helmut",
		"lastname": "Stieger",
		"status": "NEW",
		"titleBefore": "",
		"titleAfter": "",
		"residentAddress": {
		"street": "Lambacherstraße",
				"type": "RESIDENCE",
				"city": "Haag am Hausruck",
				"streetNumber": "39",
				"zip": "4680"
		},
		"billingAddress": {
		"street": "Lambacherstraße",
				"type": "BILLING",
				"city": "Haag am Hausruck",
				"streetNumber": "39",
				"zip": "4680"
	},
		"contact": {
			"email": "obermueller.peter@gmail.com",
			"phone": "06603611758"
	},
		"accountInfo": {
		"iban": "ATxxxxxxxxxxxxxxxxxx",
				"owner": "Helmut Stieger",
				"sepa": false
	},
		"businessRole": "EEG_PRIVATE",
			"role": "EEG_USER",
			"optionals": {
		"website": ""
	},
		"taxNumber": "",
			"tariffId": "",
			"meters": [
	{
	"direction": "CONSUMPTION",
	"status": "NEW",
	"meteringPoint": "AT0030000000000000000000000000011",
	"street": "Lambacherstraße",
	"streetNumber": "39",
	"zip": "4680",
	"city": "Haag am Hausruck"
	}
]
}`

	var p model.EegParticipant
	err := json.NewDecoder(strings.NewReader(registerObject)).Decode(&p)
	require.NoError(t, err)

	fmt.Printf("Part: %+v\n", p)
}

//func Test_ConfirmParticipant(t *testing.T) {
//	participantId := "ea9942da-03da-11ee-b82b-5a985b4b033a"
//	participant, err := database.QueryParticipant(participantId)
//
//	require.NoError(t, err)
//	fmt.Printf("P: %+v\n", participant)
//}

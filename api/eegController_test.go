package api

import (
	"at.ourproject/vfeeg-backend/model"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestMarschaling(t *testing.T) {
	//jsonStr := `{"id":"","name":"Mein Einspeise Traif","type":"EZP","useVat":false,"baseFee":"0","accountGrossAmount":0,"participantFee":0,"accountNetAmount":0,"billingPeriod":"monthly","businessNr":0,"centPerKWh":"0.12","discount":0,"freeKWH":0,"vatInPercent":0}`
	jsonStr := `{"id":"","name":"Mein Einspeise Traif","type":"EZP","useVat":false,"baseFee":"0","accountGrossAmount":"0","participantFee":"0","accountNetAmount":"0","billingPeriod":"monthly","businessNr":"0","centPerKWh":"12","discount":"0","freeKWH":"0","vatInPercent":"0"}`

	var r model.Tariff
	err := json.Unmarshal([]byte(jsonStr), &r)
	require.NoError(t, err)

	fmt.Printf("R: %+v\n", r)
}

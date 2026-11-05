package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platform#229: the six community types, nothing else (also no empty value).
func TestValidateArea(t *testing.T) {
	assert.Equal(t, []AreaType{"LOCAL", "REGIONAL", "BEG", "GEA", "P2P", "EVA"}, AreaTypes)
	for _, area := range AreaTypes {
		assert.NoError(t, ValidateArea(area), area)
	}
	for _, area := range []AreaType{"", "6", "local", "PP", "SC", "RC_L"} {
		assert.Error(t, ValidateArea(area), area)
	}
}

// MQTT register-eeg: the schema enum (registration.schema.json) knows exactly the types of AreaTypes.
func TestRegisterEegRequestArea(t *testing.T) {
	decode := func(area string) (RegisterEegRequest, error) {
		var r RegisterEegRequest
		err := json.Unmarshal([]byte(`{"tenant":"TE000044","rcNumber":"TE000044","communityId":"AT1","name":"n",
			"description":"d","iban":"","owner":"","sepa":false,"legal":"verein","settelmentInterval":"MONTHLY",
			"gridId":"AT003000","gridName":"g","area":"`+area+`","allocation":"DYNAMIC","eegOwner":"o","street":"s",
			"streetNumber":"1","city":"c","zip":"4020","email":"e@example.org","online":false}`), &r)
		return r, err
	}
	for _, area := range AreaTypes {
		r, err := decode(string(area))
		require.NoError(t, err, area)
		assert.Equal(t, string(area), string(r.Area))
	}
	_, err := decode("PP")
	assert.Error(t, err)
}

package mqttclient

import (
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"gopkg.in/guregu/null.v4"
)

// multiGrid are the types with several grid operators possible: BEG, and P2P and EVA, which run centrally over
// ebUtilities like the BEG (platform#229).
var multiGrid = map[model.AreaType]bool{model.BEG: true, model.P2P: true, model.EVA: true}

// The grid operator receiving a message: from the metering point for BEG, P2P and EVA; every other type uses the
// operator from the community settings.
func TestGetReceiverFrom(t *testing.T) {
	meter := &model.MeteringPoint{GridOperatorId: null.StringFrom("at008000")}
	for _, area := range model.AreaTypes() {
		eeg := &model.Eeg{Area: area, GridOperator: "at003000"}
		want := "AT003000"
		if multiGrid[area] {
			want = "AT008000"
		}
		assert.Equal(t, want, getReceiverFrom(eeg, meter), string(area))
		assert.Equal(t, "AT003000", getReceiverFrom(eeg, nil), string(area)+" without metering point")
	}
}

// The metering point list goes to the chosen grid operator for BEG, P2P and EVA, else to the community's one.
func TestZpListReceiver(t *testing.T) {
	for _, area := range model.AreaTypes() {
		got, err := zpListReceiver(&model.Eeg{Area: area, GridOperator: "AT003000"}, "AT008000")
		assert.NoError(t, err, area)
		want := "AT003000"
		if multiGrid[area] {
			want = "AT008000"
		}
		assert.Equal(t, want, got, area)

		_, err = zpListReceiver(&model.Eeg{Area: area}, "AT008000")
		assert.Equal(t, multiGrid[area], err == nil, string(area)+" without the community's grid operator")
	}
}

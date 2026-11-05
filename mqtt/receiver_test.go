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

// The grid operator receiving a message: from the metering point for BEG, P2P and EVA; every other type, and a
// metering point without one, uses the operator from the community settings; without either nothing is sent.
func TestGetReceiverFrom(t *testing.T) {
	receiver := func(eeg *model.Eeg, meter *model.MeteringPoint) string {
		r, err := getReceiverFrom(eeg, meter)
		assert.NoError(t, err, eeg.Area)
		return r
	}
	meter := &model.MeteringPoint{GridOperatorId: null.StringFrom("at008000")}
	noOperator := &model.MeteringPoint{}
	for _, area := range model.AreaTypes() {
		eeg := &model.Eeg{Area: area, GridOperator: "at003000"}
		want := "AT003000"
		if multiGrid[area] {
			want = "AT008000"
		}
		assert.Equal(t, want, receiver(eeg, meter), string(area))
		assert.Equal(t, "AT003000", receiver(eeg, nil), string(area)+" without metering point")
		assert.Equal(t, "AT003000", receiver(eeg, noOperator), string(area)+" metering point without grid operator")

		_, err := getReceiverFrom(&model.Eeg{Area: area}, noOperator)
		assert.Error(t, err, string(area)+" without any grid operator")
		r, err := getReceiverFrom(&model.Eeg{Area: area}, meter)
		assert.Equal(t, multiGrid[area], err == nil, string(area)+" only the metering point's grid operator")
		if err == nil {
			assert.Equal(t, "AT008000", r)
		}
	}
}

// Nothing is sent without a grid operator: P2P (and BEG) with a metering point that has none and no grid operator
// in the community settings.
func TestRegistrationForParticipationWithoutGridOperator(t *testing.T) {
	for _, area := range []model.AreaType{model.P2P, model.BEG} {
		eeg := &model.Eeg{Id: "TE000049", RcNumber: "TE000049", Area: area}
		err := RegistrationForParticipation(eeg, &model.MeteringPoint{MeteringPoint: "AT0030000000000000000000000049001"}, nil)
		assert.Error(t, err, area)
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

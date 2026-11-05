package mqttclient

import (
	"testing"

	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"gopkg.in/guregu/null.v4"
)

// The grid operator receiving a message: from the metering point only for BEG; every other type, P2P and EVA
// included (platform#229, phase 1: one grid area), uses the operator from the community settings.
func TestGetReceiverFrom(t *testing.T) {
	meter := &model.MeteringPoint{GridOperatorId: null.StringFrom("at008000")}
	for _, area := range model.AreaTypes {
		eeg := &model.Eeg{Area: area, GridOperator: "at003000"}
		want := "AT003000"
		if area == model.BEG {
			want = "AT008000"
		}
		assert.Equal(t, want, getReceiverFrom(eeg, meter), string(area))
		assert.Equal(t, "AT003000", getReceiverFrom(eeg, nil), string(area)+" without metering point")
	}
}

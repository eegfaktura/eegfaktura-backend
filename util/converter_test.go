package util

import (
	"at.ourproject/vfeeg-backend/graph/gmodel"
	"at.ourproject/vfeeg-backend/model"
	"fmt"
	"github.com/mitchellh/mapstructure"
	"testing"
)

func TestConvertStructToMap(t *testing.T) {

	eeg := model.Eeg{Id: "UUID-1-1-1", Name: "TEST EEG"}

	//rm := ConvertStructToMap(eeg)

	rm := make(map[string]interface{})
	mapstructure.Decode(eeg, &rm)

	fmt.Printf("result: %+v\n", rm)

	eeg_out := model.Eeg{}

	mapstructure.Decode(rm, &eeg_out)

	fmt.Printf("result struct: %+v\n", eeg_out)

	eegModel := &gmodel.EegModel{}
	sepp := "sepp"
	eegModel.SettlementInterval = &sepp

	rm1 := make(map[string]interface{})
	mapstructure.Decode(eegModel, &rm1)

	fmt.Printf("result map: %+v\n", 1)

}

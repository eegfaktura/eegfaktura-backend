package eda

import (
	"at.ourproject/vfeeg-backend/model"
	"errors"
)

func extractResponseCodeAndMeteringPoint(ebmsMessage *model.EbmsMessage) ([]int16, []string, error) {
	meters := []string{}
	codes := []int16{}
	for _, rd := range ebmsMessage.ResponseData {
		if len(rd.ResponseCode) > 0 {
			meters = append(meters, rd.MeteringPoint)
			codes = append(codes, rd.ResponseCode...)
		}
	}

	if len(codes) == 0 {
		return codes, meters, errors.New("wrong Response from EDA")
	}

	return codes, meters, nil
}

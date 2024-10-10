package eda

import (
	"at.ourproject/vfeeg-backend/model"
	"errors"
	"fmt"
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

func extractMeterList(ebmsMessage *model.EbmsMessage) []string {
	meters := []string{}
	for _, m := range ebmsMessage.MeterList {
		meters = append(meters, m.MeteringPoint)
	}
	return meters
}

func codesContains(expected, codes []int16) bool {
	var intersect []int16
	for _, element1 := range codes {
		for _, element2 := range expected {
			if element1 == element2 {
				intersect = append(intersect, element1)
			}
		}
	}
	return len(intersect) > 0
}

func convertCodes2Strings(codes []int16) []string {
	strCodes := []string{}
	for _, c := range codes {
		sc, ok := ECON_RESPONSE_CODES[c]
		if !ok {
			sc = fmt.Sprintf("%d", c)
		}
		strCodes = append(strCodes, sc)
	}
	return strCodes
}

package eda

import (
	"at.ourproject/vfeeg-backend/model"
	"errors"
)

func extractResponseCodeAndMeteringPoint(ebmsMessage *model.EbmsMessage) (int16, string, error) {
	for _, rd := range ebmsMessage.ResponseData {
		meter := rd.MeteringPoint
		if len(rd.ResponseCode) > 0 {
			return rd.ResponseCode[0], meter, nil
		} else {
			return -1, meter, nil
		}
	}

	return 0, "", errors.New("wrong Response from EDA")
}

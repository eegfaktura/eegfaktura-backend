package factory

import (
	"at.ourproject/vfeeg-backend/model"
	"gopkg.in/guregu/null.v4"
)

func GetEegFromRegisterEeg(request model.RegisterEegRequest) model.Eeg {

	getOptionalField := func(field *string) null.String {
		if field == nil {
			return null.String{}
		}
		return null.StringFrom(*field)
	}

	return model.Eeg{
		Id:                 request.Tenant,
		Name:               request.Name,
		Description:        request.Description,
		BusinessNr:         null.String{},
		Area:               model.AreaType(request.Area),
		Legal:              string(request.Legal),
		OperatorName:       request.GridName,
		CommunityId:        request.CommunityId,
		GridOperator:       request.GridId,
		RcNumber:           request.RcNumber,
		AllocationMode:     model.AllocationModeType(request.Allocation),
		SettlementInterval: string(request.SettelmentInterval),
		ProviderBusinessNr: null.Int{},
		EegAddress: model.EegAddress{
			Street:       request.Street,
			StreetNumber: request.StreetNumber,
			Zip:          request.Zip,
			City:         request.City,
		},
		AccountInfo: model.AccountInfo{
			Iban:  null.StringFrom(request.Iban),
			Owner: null.StringFrom(request.Owner),
			Sepa:  request.Sepa,
		},
		Contact: model.Contact{
			Phone: getOptionalField(request.Phone),
			Email: null.StringFrom(request.Email),
		},
		Optionals: model.Optionals{
			Website: getOptionalField(request.Web),
		},
		//Periods:       nil,
		Online:        request.Online,
		ContactPerson: null.StringFrom(request.EegOwner),
	}
}

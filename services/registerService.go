package services

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	protobuf "at.ourproject/vfeeg-backend/proto"
	"context"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/guregu/null.v4"
)

type RegisterService struct {
	protobuf.UnimplementedRegisterEegServiceServer
}

func (r *RegisterService) Register(ctx context.Context, eeg *protobuf.RegisterEegRequest) (*protobuf.RegisteredEegReply, error) {

	getOptionalField := func(field *string) null.String {
		if field == nil {
			return null.String{}
		}
		return null.StringFrom(*field)
	}

	newEeg := model.Eeg{
		Id:                 eeg.Tenant,
		Name:               eeg.Name,
		Description:        eeg.Description,
		BusinessNr:         null.String{},
		Area:               model.AreaType(eeg.Area.String()),
		Legal:              eeg.Legal.String(),
		OperatorName:       eeg.GridName,
		CommunityId:        eeg.CommunityId,
		GridOperator:       eeg.GridId,
		RcNumber:           eeg.RcNumber,
		AllocationMode:     model.AllocationModeType(eeg.Allocation.String()),
		SettlementInterval: eeg.SettelmentInterval.String(),
		ProviderBusinessNr: null.Int{},
		EegAddress: model.EegAddress{
			Street:       eeg.Street,
			StreetNumber: eeg.StreetNumber,
			Zip:          eeg.Zip,
			City:         eeg.City,
		},
		AccountInfo: model.AccountInfo{
			Iban:  null.StringFrom(eeg.Iban),
			Owner: null.StringFrom(eeg.Owner),
			Sepa:  eeg.Sepa,
		},
		Contact: model.Contact{
			Phone: getOptionalField(eeg.Phone),
			Email: null.StringFrom(eeg.Email),
		},
		Optionals: model.Optionals{
			Website: getOptionalField(eeg.Web),
		},
		//Periods:       nil,
		Online:        eeg.Online,
		ContactPerson: null.StringFrom(eeg.EegOwner),
	}

	log.Printf("Register EEG: %+v", newEeg)
	db, err := database.ConnectToDatabase()
	if err != nil {
		log.Errorf("Database Error: %v", err)
		return &protobuf.RegisteredEegReply{Status: 500}, err
	}
	defer func() { _ = db.Close() }()

	err = database.InsertEeg(db, eeg.RcNumber, &newEeg)
	if err != nil {
		log.Errorf("Could not create an EEG! %v", err.Error())
		return &protobuf.RegisteredEegReply{Status: 500},
			status.Errorf(codes.NotFound, "unknown service %v", err)
	}

	return &protobuf.RegisteredEegReply{Status: 201}, nil
}

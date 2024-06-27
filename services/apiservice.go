package services

import (
	protobuf "at.ourproject/vfeeg-backend/proto"
	"context"
)

type ApiService struct {
	protobuf.UnimplementedRegisterEegServiceServer
}

func (api *ApiService) MasterData_MeteringPoint(ctx context.Context, meterRequest *protobuf.MeteringRequest) (*protobuf.MeteringPointReply, error) {

	return nil, nil
}

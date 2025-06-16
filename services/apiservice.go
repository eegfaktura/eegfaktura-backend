package services

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	protobuf "at.ourproject/vfeeg-backend/proto"
	"context"
)

type ApiService struct {
	protobuf.UnimplementedApiServiceServer
}

func (api *ApiService) MasterData_MeteringPoint(ctx context.Context, meterRequest *protobuf.MeteringRequest) (*protobuf.MeteringPointReply, error) {
	db, err := database.ConnectToDatabase()
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	var meters []*model.MeteringPoint
	if meterRequest.From == nil || meterRequest.To == nil {
		meters, err = database.FindMeteringPointsForTenant(db, meterRequest.Tenant)
	} else {
		meters, err = database.FindMeteringPointsActivePeriod(db, meterRequest.Tenant, int64(*meterRequest.From), int64(*meterRequest.To))
	}
	if err != nil {
		return nil, err
	}

	result := []*protobuf.MeteringPoint{}
	for _, meter := range meters {
		activeSince := uint64(meter.State.ActiveSince.Date.Unix() * 1000)
		inactiveSince := uint64(meter.State.InactiveSince.Date.Unix() * 1000)
		registeredSince := uint64(meter.RegisteredSince.Unix() * 1000)
		result = append(result, &protobuf.MeteringPoint{
			MeteringPointId: meter.MeteringPoint,
			Direction:       string(meter.Direction),
			Status:          string(meter.Status),
			PartFact:        uint32(meter.PartFact),
			ActiveSince:     &activeSince,
			InactiveSince:   &inactiveSince,
			Transformer:     meter.Transformer.Ptr(),
			EquipmentNumber: meter.EquipmentNumber.Ptr(),
			EquipmentName:   meter.EquipmentName.Ptr(),
			InverterId:      meter.InverterId.Ptr(),
			Street:          meter.Street.Ptr(),
			StreetNumber:    meter.StreetNumber.Ptr(),
			City:            meter.City.Ptr(),
			Zip:             meter.Zip.Ptr(),
			RegisteredSince: &registeredSince,
		})
	}
	return &protobuf.MeteringPointReply{MeteringPoints: result}, nil
}

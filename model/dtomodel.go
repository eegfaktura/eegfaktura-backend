package model

import (
	"gopkg.in/guregu/null.v4"
	"time"
)

type MeteringPointDBModel struct {
	MeteringPoint    string        `json:"meteringPoint" db:"metering_point_id" goqu:"skipupdate"`
	ConsentId        null.String   `json:"consentId" db:"consent_id" goqu:"omitempty"`
	Transformer      null.String   `json:"transformer,omitempty" goqu:"omitempty"`
	Direction        DirectionType `json:"direction,omitempty" goqu:"omitnil"`
	Status           *StatusType   `json:"status,omitempty" goqu:"omitnil"`
	StatusCode       null.Int      `json:"statusCode,omitempty" db:"statusCode" goqu:"omitempty"`
	TariffId         null.String   `json:"tariff_id,omitempty" db:"tariff_id" goqu:"omitempty"`
	EquipmentNumber  null.String   `json:"equipmentNumber,omitempty" db:"equipmentNumber" goqu:"omitempty"`
	EquipmentName    null.String   `json:"equipmentName,omitempty" db:"equipmentName" goqu:"omitempty"`
	InverterId       null.String   `json:"inverterid,omitempty" db:"inverterid" goqu:"omitempty"`
	Street           null.String   `json:"street,omitempty" goqu:"omitempty"`
	StreetNumber     null.String   `json:"streetNumber,omitempty" db:"streetNumber" goqu:"omitempty"`
	City             null.String   `json:"city,omitempty" goqu:"omitempty"`
	Zip              null.String   `json:"zip,omitempty" goqu:"omitempty"`
	RegisteredSince  *time.Time    `json:"registeredSince" db:"registeredSince" goqu:"omitnil"`
	ModifiedAt       *time.Time    `json:"modifiedAt" db:"modifiedAt" goqu:"omitnil"`
	ModifiedBy       null.String   `json:"modifiedBy" db:"modifiedBy" goqu:"omitempty"`
	GridOperatorId   null.String   `json:"gridOperatorId,omitempty" db:"grid_operator_id" goqu:"omitempty"`
	GridOperatorName null.String   `json:"gridOperatorName,omitempty" db:"grid_operator_name" goqu:"omitempty"`
	ActiveSince      *time.Time    `json:"activesince" goqu:"omitnil"`
	InactiveSince    *time.Time    `json:"inactivesince" goqu:"omitnil"`
	Active           *int          `json:"active" goqu:"omitnil"`
	Flag             *int          `json:"flat" goqu:"omitnil"`
}

func ConvertFromMeterList(ml []Meter) []*MeteringPointDBModel {

	getConsentId := func(consentId string) null.String {
		if len(consentId) > 0 {
			return null.StringFrom(consentId)
		}
		return null.String{}
	}

	converted := make([]*MeteringPointDBModel, len(ml))
	for i := range ml {
		converted[i] = &MeteringPointDBModel{
			MeteringPoint: ml[i].MeteringPoint,
			ConsentId:     getConsentId(ml[i].ConsentID),
			//Transformer:      null.String{},
			Direction:        ml[i].Direction,
			Status:           nil,
			StatusCode:       null.Int{},
			TariffId:         null.String{},
			EquipmentNumber:  null.String{},
			EquipmentName:    null.String{},
			InverterId:       null.String{},
			Street:           null.String{},
			StreetNumber:     null.String{},
			City:             null.String{},
			Zip:              null.String{},
			RegisteredSince:  nil,
			ModifiedAt:       nil,
			ModifiedBy:       null.String{},
			GridOperatorId:   null.String{},
			GridOperatorName: null.String{},
			ActiveSince:      nil,
			InactiveSince:    nil,
			Active:           nil,
			Flag:             nil,
		}
	}
	return converted
}

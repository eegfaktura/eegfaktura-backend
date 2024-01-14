package model

import (
	"github.com/pborman/uuid"
	"gopkg.in/guregu/null.v4"
	"time"
)

//func (ts EegParticipantState) MarshalJSON() ([]byte, error) {
//	type Alias EegParticipantState
//	if ts.Since == nil {
//		return json.Marshal(&ts)
//	}
//
//	return json.Marshal(&struct {
//		*Alias
//		Since int64 `json:"since"`
//	}{
//		Alias: (*Alias)(&ts),
//		Since: (*ts.Since).UnixMilli(),
//	})
//}

type MeterState struct {
	ActiveSince   time.Time `json:"activeSince" goqu:"skipinsert"`
	InactiveSince time.Time `json:"inactiveSince" goqu:"skipinsert"`
}

type EegParticipant struct {
	Id                    uuid.UUID        `json:"id" goqu:"skipupdate"`
	ParticipantNumber     null.String      `json:"participantNumber" db:"participantNumber"`
	BusinessRole          string           `json:"businessRole" db:"businessRole"`
	Role                  string           `json:"role" db:"role"`
	FirstName             string           `json:"firstname"`
	LastName              string           `json:"lastname"`
	TitleBefore           string           `json:"titleBefore" db:"titleBefore"`
	TitleAfter            string           `json:"titleAfter" db:"titleAfter"`
	ParticipantSince      time.Time        `json:"participantSince" db:"participantSince" goqu:"defaultifempty"`
	VatNumber             string           `json:"vatNumber" db:"vatNumber"`
	TaxNumber             string           `json:"taxNumber" db:"taxNumber"`
	CompanyRegisterNumber string           `json:"companyRegisterNumber" db:"companyRegisterNumber"`
	Contact               ContactInfo      `json:"contact" db:"-" goqu:"skipinsert"`
	BillingAddress        Address          `json:"billingAddress" db:"-" goqu:"skipinsert"`
	ResidentAddress       Address          `json:"residentAddress" db:"-" goqu:"skipinsert"`
	BankAccount           BankInfo         `json:"accountInfo" db:"-" goqu:"skipinsert"`
	MeteringPoint         []*MeteringPoint `json:"meters" db:"-" goqu:"skipinsert"`
	TariffId              null.String      `json:"tariffId" db:"tariffId" goqu:"skipinsert"`
	Status                StatusType       `json:"status" goqu:"defaultifempty"`
	Version               int              `json:"version" goqu:"defaultifempty"`
	CreatedBy             string           `json:"createdBy,omitempty" db:"createdBy"`
}

type ContactInfo struct {
	Phone null.String `json:"phone" db:"phone"`
	Email null.String `json:"email" db:"email"`
}

type BankInfo struct {
	Iban     null.String `json:"iban"`
	Owner    null.String `json:"owner"`
	BankName null.String `json:"bankName" db:"bankName"`
}

type DirectionType string

const (
	CONSUMPTION DirectionType = "CONSUMPTION"
	GENERATOR   DirectionType = "GENERATION"
	UNKNOWN     DirectionType = "UNKNOWN"
)

type StatusType string

const (
	NEW      StatusType = "NEW"
	PENDING  StatusType = "PENDING"
	APPROVED StatusType = "APPROVED"
	ACTIVE   StatusType = "ACTIVE"
	INACTIVE StatusType = "INACTIVE"
	REJECTED StatusType = "REJECTED"
	REVOKED  StatusType = "REVOKED"
	INVALID  StatusType = "INVALID"
	ARCHIVED StatusType = "ARCHIVED"
)

type MeteringPoint struct {
	MeteringPoint    string        `json:"meteringPoint" db:"metering_point_id" goqu:"skipupdate"`
	Transformer      null.String   `json:"transformer,omitempty"`
	Direction        DirectionType `json:"direction,omitempty"`
	Status           StatusType    `json:"status,omitempty"`
	TariffId         null.String   `json:"tariff_id,omitempty" db:"tariff_id"`
	EquipmentNumber  null.String   `json:"equipmentNumber,omitempty" db:"equipmentNumber"`
	EquipmentName    null.String   `json:"equipmentName,omitempty" db:"equipmentName"`
	InverterId       null.String   `json:"inverterid,omitempty" db:"inverterid"`
	Street           null.String   `json:"street,omitempty"`
	StreetNumber     null.String   `json:"streetNumber,omitempty" db:"streetNumber"`
	City             null.String   `json:"city,omitempty"`
	Zip              null.String   `json:"zip,omitempty"`
	RegisteredSince  time.Time     `json:"registeredSince" db:"registeredSince"`
	ModifiedAt       time.Time     `json:"modifiedAt" db:"modifiedAt"`
	ModifiedBy       null.String   `json:"modifiedBy" db:"modifiedBy"`
	GridOperatorId   null.String   `json:"gridOperatorId,omitempty" db:"grid_operator_id"`
	GridOperatorName null.String   `json:"gridOperatorName,omitempty" db:"grid_operator_name"`
	State            *MeterState   `json:"participantState" db:"participant_meter_state" goqu:"skipupdate"`
}

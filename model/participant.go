package model

import (
	"github.com/pborman/uuid"
	"gopkg.in/guregu/null.v4"
	"time"
)

type EegParticipant struct {
	Id                    uuid.UUID        `json:"id" goqu:"skipupdate"`
	FirstName             string           `json:"firstname"`
	LastName              string           `json:"lastname"`
	TitleBefore           string           `json:"titleBefore,omitempty"`
	TitleAfter            string           `json:"titleAfter,omitempty"`
	ParticipantSince      time.Time        `json:"participantSince" goqu:"defaultifempty"`
	VatId                 string           `json:"vatId,omitempty"`
	TaxId                 string           `json:"taxId,omitempty"`
	CompanyRegisterNumber string           `json:"companyRegisterNumber,omitempty"`
	Contact               ContactInfo      `json:"contact" db:"-" goqu:"skipinsert"`
	BillingAddress        Address          `json:"billingAddress" db:"-" goqu:"skipinsert"`
	ResidentAddress       Address          `json:"residentAddress" db:"-" goqu:"skipinsert"`
	BankAccount           BankInfo         `json:"accountInfo" db:"-" goqu:"skipinsert"`
	MeteringPoint         []*MeteringPoint `json:"meters" db:"-" goqu:"skipinsert"`
	TariffId              null.String      `json:"tariffId,omitempty" db:"tariffid" goqu:"skipinsert"`
	Status                StatusType       `json:"status,omitempty" goqu:"defaultifempty"`
	Version               int              `json:"version,omitempty" goqu:"defaultifempty"`
}

type ContactInfo struct {
	Phone null.String `json:"phone" db:"phone"`
	Email null.String `json:"email" db:"email"`
}

type BankInfo struct {
	Iban  null.String `json:"iban"`
	Owner null.String `json:"owner"`
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
)

type MeteringPoint struct {
	MeteringPoint string        `json:"meteringPoint" db:"metering_point_id"`
	Transformer   null.String   `json:"transformer,omitempty"`
	Direction     DirectionType `json:"direction,omitempty"`
	Status        StatusType    `json:"status,omitempty"`
	TariffId      null.String   `json:"tariffId" db:"tariff_id"`
	EquipmentName null.String   `json:"equipmentName,omitempty" db:"equipmentname"`
	InverterId    null.String   `json:"inverterId,omitempty" db:"inverterid"`
	Street        null.String   `json:"street,omitempty"`
	StreetNumber  null.String   `json:"streetNumber,omitempty" db:"street_number"`
	City          null.String   `json:"city,omitempty"`
	Zip           null.String   `json:"zip,omitempty"`
}

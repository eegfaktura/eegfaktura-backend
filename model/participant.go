package model

import (
	"github.com/pborman/uuid"
	"gopkg.in/guregu/null.v4"
	"time"
)

type EegParticipant struct {
	Id                    uuid.UUID       `json:"id"`
	FirstName             string          `json:"firstname"`
	LastName              string          `json:"lastname"`
	TitleBefore           string          `json:"titleBefore,omitempty"`
	TitleAfter            string          `json:"titleAfter,omitempty"`
	ParticipantSince      time.Time       `json:"participantSince" goqu:"defaultifempty"`
	VatId                 string          `json:"vatId,omitempty"`
	TaxId                 string          `json:"taxId,omitempty"`
	CompanyRegisterNumber string          `json:"companyRegisterNumber,omitempty"`
	Contact               ContactInfo     `json:"contact" db:"-" goqu:"skipinsert"`
	BillingAddress        Address         `json:"billingAddress" db:"-" goqu:"skipinsert"`
	ResidentAddress       Address         `json:"residentAddress" db:"-" goqu:"skipinsert"`
	BankAccount           BankInfo        `json:"bankAccount" db:"-" goqu:"skipinsert"`
	MeteringPoint         []MeteringPoint `json:"meters" db:"-" goqu:"skipinsert"`
	TariffId              null.String     `json:"tariffId,omitempty" db:"tariffid" goqu:"skipinsert"`
	Status                StatusType      `json:"status,omitempty" goqu:"defaultifempty"`
	Version               int             `json:"version,omitempty" goqu:"defaultifempty"`
}

type ContactInfo struct {
	Phone string `json:"phone" db:"phone"`
	Email string `json:"email" db:"email"`
}

type BankInfo struct {
	Iban  string `json:"iban"`
	Owner string `json:"owner"`
}

type DirectionType string

const (
	CONSUMPTION = "CONSUMPTION"
	GENERATOR   = "GENERATOR"
)

type StatusType string

const (
	NEW      = "NEW"
	PENDING  = "PENDING"
	ACTIVE   = "ACTIVE"
	INACTIVE = "INACTIVE"
)

type MeteringPoint struct {
	MeteringPoint string        `json:"meteringPoint" db:"metering_point_id"`
	Transformer   string        `json:"transformer,omitempty"`
	Direction     DirectionType `json:"direction,omitempty"`
	Status        StatusType    `json:"status,omitempty"`
	TariffId      uuid.UUID     `json:"tariffId" db:"tariff_id"`
	EquipmentName string        `json:"equipmentName,omitempty" db:"equipmentName"`
	InverterId    string        `json:"inverterId,omitempty" db:"inverterId"`
	Street        string        `json:"street,omitempty"`
	StreetNumber  string        `json:"streetNumber,omitempty" db:"street_number"`
	City          string        `json:"city,omitempty"`
	Zip           string        `json:"zip,omitempty"`
}

package model

import (
	"github.com/pborman/uuid"
	"gopkg.in/guregu/null.v4"
)

type BillingPeriod string

const (
	ANNUAL     BillingPeriod = "annual"
	MONTHLY    BillingPeriod = "monthly"
	SEMIANNUAL BillingPeriod = "semiannual"
	QUARTERLY  BillingPeriod = "quarterly"
)

type TariffModelType string

const (
	EEG    TariffModelType = "EEG"
	VZP    TariffModelType = "VZP"
	EZP    TariffModelType = "EZP"
	AKONTO TariffModelType = "AKONTO"
)

type Tariff struct {
	Id                   uuid.UUID       `json:"id" goqu:"defaultifempty"`
	Version              int             `json:"version" db:"version"`
	Type                 TariffModelType `json:"type"`
	Name                 string          `json:"name"`
	BillingPeriod        string          `json:"billingPeriod" db:"billingPeriod"`
	UseVat               bool            `json:"useVat" db:"useVat"`
	VatSupplementaryText string          `json:"vatSupplementaryText" db:"vatSupplementaryText" goqu:"omitempty"`
	VatInPercent         int             `json:"vatInPercent,string" db:"vatInPercent"`
	AccountNetAmount     int             `json:"accountNetAmount,string" db:"accountNetAmount"`
	AccountGrossAmount   int             `json:"accountGrossAmount,string"  db:"accountGrossAmount"`
	ParticipantFee       float32         `json:"participantFee" db:"participantFee"`
	BaseFee              int             `json:"baseFee,string" db:"baseFee"`
	BusinessNr           int             `json:"businessNr,string" db:"businessNr"`
	CentPerKWh           float32         `json:"centPerKWh" db:"centPerKWh"`
	FreeKWh              int             `json:"freeKWh,string" db:"freeKWh"`
	Discount             int             `json:"discount,string"`
	UseMeteringFee       bool            `json:"useMeteringPointFee"  db:"useMeteringPointFee"`
	MeteringFee          null.Float      `json:"meteringPointFee,omitempty" db:"meteringPointFee"`
}

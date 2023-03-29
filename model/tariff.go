package model

import (
	"github.com/pborman/uuid"
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
	Id                 uuid.UUID       `json:"id"`
	Version            int             `json:"version"`
	Type               TariffModelType `json:"type"`
	Name               string          `json:"name"`
	BillingPeriod      BillingPeriod   `json:"billingPeriod"`
	UseVat             bool            `json:"useVat,omitempty"`
	VatInPercent       int             `json:"vatInPercent,omitempty,string"`
	AccountNetAmount   int             `json:"accountNetAmount,omitempty,string"`
	AccountGrossAmount int             `json:"accountGrossAmount,omitempty,string"`
	ParticipantFee     int             `json:"participantFee,omitempty,string"`
	BaseFee            int             `json:"baseFee,omitempty,string"`
	BusinessNr         int             `json:"businessNr,omitempty,string"`
	CentPerKWh         float64         `json:"centPerKWh,omitempty,string"`
	FreeKWH            int             `json:"freeKWH,omitempty,string" db:"freekwh"`
	Discount           int             `json:"discount,omitempty,string"`
}

//func (t Tariff) PrepareType() Tariff {
//	switch t.Type {
//	case "EEG":
//		t.AccountNetAmount = 0
//		t.AccountGrossAmount = 0
//		t.CentPerKWh = 0
//		t.FreeKWH = 0
//		break
//	case "VZP":
//		t.AccountNetAmount = 0
//		t.AccountGrossAmount = 0
//		t.ParticipantFee = 0
//		break
//	case "EZP":
//		t.AccountNetAmount = 0
//		t.AccountGrossAmount = 0
//		t.ParticipantFee = 0
//	}
//	return t
//}

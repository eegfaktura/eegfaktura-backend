package model

import (
	"github.com/jmoiron/sqlx/types"
	"gopkg.in/guregu/null.v4"
	"time"
)

type Eeg struct {
	Id                 string      `json:"id"`
	Name               string      `json:"name,omitempty"`
	Description        string      `json:"description,omitempty"`
	BusinessNr         null.Int    `json:"businessNr,omitempty"`
	Area               AreaType    `json:"area"` /* LOCAL | REGIONAL*/
	Legal              string      `json:"legal,omitempty"`
	OperatorName       string      `json:"operatorName,omitempty"`
	CommunityId        string      `json:"communityId,omitempty"`
	GridOperator       string      `json:"gridOperator,omitempty"`
	RcNumber           string      `json:"rcNumber"`
	AllocationMode     string      `json:"allocationMode,omitempty"`
	SettlementInterval string      `json:"settlementInterval,omitempty"`
	ProviderBusinessNr null.Int    `json:"providerBusinessNr,omitempty"`
	TaxNumber          null.String `json:"taxNumber,omitempty"`
	VatNumber          null.String `json:"vatNumber"`
	ContactPerson      string      `json:"contactPerson"`
	Address            `json:"address,omitempty"`
	AccountInfo        AccountInfo `json:"accountInfo,omitempty"`
	Contact            Contact     `json:"contact,omitempty"`
	Optionals          Optionals   `json:"optionals,omitempty"`
	Periods            []int16     `json:"periods"`
	Online             bool        `json:"online"`
}

type AreaType string

const (
	LOCAL    AreaType = "LOCAL"
	REGIONAL AreaType = "REGIONAL"
)

type AddressType string

const (
	BILLING   AddressType = "BILLING"
	RESIDENCE AddressType = "RESIDENCE"
)

type Address struct {
	Type         AddressType `json:"type" goqu:"skipupdate"`
	Street       string      `json:"street,omitempty"`
	StreetNumber string      `json:"streetNumber,omitempty" db:"streetNumber"`
	Zip          string      `json:"zip,omitempty"`
	City         string      `json:"city,omitempty"`
}

type Contact struct {
	Phone null.String `json:"phone,omitempty"`
	Email null.String `json:"email,omitempty"`
}

type AccountInfo struct {
	Iban  null.String `json:"iban"`
	Owner null.String `json:"owner"`
	Sepa  bool        `json:"sepa"`
}

type Optionals struct {
	Website null.String `json:"website,omitempty"`
}
type EegNotification struct {
	Id      int16          `json:"id"`
	MsgType string         `json:"type" db:"type"`
	Message types.JSONText `json:"message" db:"notification"`
	Date    time.Time      `json:"date"`
}

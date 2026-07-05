package model

import (
	"github.com/jjeffery/civil"
	"gopkg.in/guregu/null.v4"
)

type Eeg struct {
	Id                 string             `json:"id" db:"tenant" goqu:"skipupdate"`
	Name               string             `json:"name,omitempty" goqu:"omitempty"`
	Description        string             `json:"description" goqu:"omitempty"`
	BusinessNr         null.String        `json:"businessNr,omitempty" db:"businessNr" goqu:"omitempty"`
	Area               AreaType           `json:"area" goqu:"omitempty"` /* LOCAL | REGIONAL | BEG | GEA */
	Legal              string             `json:"legal,omitempty" goqu:"omitempty"`
	GridOperator       string             `json:"gridOperator,omitempty" db:"gridoperator_code" goqu:"omitempty"`
	OperatorName       string             `json:"operatorName,omitempty" db:"gridoperator_name" goqu:"omitempty"`
	CommunityId        string             `json:"communityId,omitempty" db:"communityId" goqu:"omitempty"`
	RcNumber           string             `json:"rcNumber" db:"rcNumber" goqu:"skipupdate"`
	AllocationMode     AllocationModeType `json:"allocationMode,omitempty" db:"allocationMode" goqu:"omitempty"`
	SettlementInterval string             `json:"settlementInterval,omitempty" db:"settlementInterval" goqu:"omitempty"`
	ProviderBusinessNr null.Int           `json:"providerBusinessNr,omitempty" db:"providerBusinessNr" goqu:"omitempty"`
	TaxNumber          null.String        `json:"taxNumber,omitempty" db:"taxNumber" goqu:"omitempty"`
	VatNumber          null.String        `json:"vatNumber" db:"vatNumber" goqu:"omitempty"`
	ContactPerson      null.String        `json:"contactPerson" db:"contactPerson" goqu:"omitempty"`
	Online             bool               `json:"online" goqu:"skipupdate"`
	CreatedAt          civil.NullDate     `json:"createdAt,omitempty" db:"createdat" goqu:"skipinsert,skipupdate"`
	EegAddress         `json:"address,omitempty" mapstructure:",squash" goqu:"omitempty"`
	AccountInfo        `json:"accountInfo,omitempty" mapstructure:",squash" goqu:"omitempty"`
	Contact            `json:"contact,omitempty" mapstructure:",squash" goqu:"omitempty"`
	Optionals          `json:"optionals,omitempty" mapstructure:",squash" goqu:"omitempty"`
	//Periods            []int16 `json:"periods" goqu:"skipinsert,defaultifempty"`
}

type AllocationModeType string

const (
	STATIC  AllocationModeType = "STATIC"
	DYNAMIC AllocationModeType = "DYNAMIC"
)

type AreaType string

const (
	LOCAL    AreaType = "LOCAL"
	REGIONAL AreaType = "REGIONAL"
	BEG      AreaType = "BEG"
	GEA      AreaType = "GEA"
)

type AddressType string

const (
	BILLING   AddressType = "BILLING"
	RESIDENCE AddressType = "RESIDENCE"
)

type Address struct {
	Type         AddressType `json:"type,omitempty" goqu:"skipupdate"`
	Street       null.String `json:"street,omitempty"  goqu:"omitempty,omitnil"`
	StreetNumber null.String `json:"streetNumber,omitempty" db:"streetNumber" goqu:"omitempty,omitnil"`
	Zip          null.String `json:"zip,omitempty" goqu:"omitempty,omitnil"`
	City         null.String `json:"city,omitempty" goqu:"omitempty,omitnil"`
}

type EegAddress struct {
	Street       string `json:"street,omitempty" goqu:"omitempty"`
	StreetNumber string `json:"streetNumber,omitempty" db:"streetNumber" goqu:"omitempty"`
	Zip          string `json:"zip,omitempty" goqu:"omitempty"`
	City         string `json:"city,omitempty" goqu:"omitempty"`
}

type Contact struct {
	Phone null.String `json:"phone,omitempty" goqu:"omitempty"`
	Email null.String `json:"email,omitempty" goqu:"omitempty"`
}

type AccountInfo struct {
	Iban        null.String `json:"iban" goqu:"omitempty"`
	Owner       null.String `json:"owner" goqu:"omitempty"`
	BankName    null.String `json:"bankName" db:"bankName" goqu:"omitempty"`
	CreditorId  null.String `json:"creditorId" db:"creditor_id" goqu:"omitempty"`
	Bic         null.String `json:"bic" db:"bic" goqu:"omitempty"`
	Sepa        bool        `json:"sepa" db:"sepa" goqu:"omitempty"`
	BankPurpose null.String `json:"bankPurpose" db:"bankPurpose" goqu:"omitempty"`
}

type Optionals struct {
	Website null.String `json:"website,omitempty" goqu:"omitempty"`
}

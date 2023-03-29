package model

type Eeg struct {
	Id                 string `json:"id"`
	Name               string `json:"name,omitempty"`
	Description        string `json:"description,omitempty"`
	BusinessNr         int64  `json:"businessNr,omitempty"`
	Area               string `json:"area"` /* LOCAL | REGIONAL*/
	Legal              string `json:"legal,omitempty"`
	OperatorName       string `json:"operatorName,omitempty"`
	CommunityId        string `json:"communityId,omitempty"`
	GridOperator       string `json:"gridOperator,omitempty"`
	RcNumber           string `json:"rcNumber,omitempty"`
	AllocationMode     string `json:"allocationMode,omitempty"`
	SettlementInterval string `json:"settlementInterval,omitempty"`
	ProviderBusinessNr int64  `json:"providerBusinessNr,omitempty"`
	Address            `json:"address,omitempty"`
	AccountInfo        AccountInfo `json:"accountInfo,omitempty"`
	Contact            Contact     `json:"contact,omitempty"`
	Optionals          Optionals   `json:"optionals,omitempty"`
	Periods            []int16     `json:"periods"`
}

type AddressType string

const (
	BILLING   = "BILLING"
	RESIDENCE = "RESIDENCE"
)

type Address struct {
	Type         AddressType `json:"type"`
	Street       string      `json:"street,omitempty"`
	StreetNumber int         `json:"streetNumber,omitempty" db:"street_number"`
	Zip          string      `json:"zip,omitempty"`
	City         string      `json:"city,omitempty"`
}

type Contact struct {
	Phone string `json:"phone,omitempty"`
	Email string `json:"email,omitempty"`
}

type AccountInfo struct {
	Iban  string `json:"iban"`
	Owner string `json:"owner"`
	Sepa  bool   `json:"sepa"`
}

type Optionals struct {
	Website string `json:"website,omitempty"`
}

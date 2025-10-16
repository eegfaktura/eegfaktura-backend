package model

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/jjeffery/civil"
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

// Workaround for int values which will be provided as string in the communication.
// Custom type that can handle string or int
type IntOrString int

func (i *IntOrString) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as int
	var intVal int
	if err := json.Unmarshal(data, &intVal); err == nil {
		*i = IntOrString(intVal)
		return nil
	}

	// Try to unmarshal as string
	var strVal string
	if err := json.Unmarshal(data, &strVal); err == nil {
		intVal, err := strconv.Atoi(strVal)
		if err != nil {
			return err
		}
		*i = IntOrString(intVal)
		return nil
	}

	return fmt.Errorf("IntOrString: invalid data %s", string(data))
}

type Tariff struct {
	Id                   uuid.UUID       `json:"id" goqu:"defaultifempty"`
	Version              int             `json:"version" db:"version"`
	Type                 TariffModelType `json:"type"`
	Name                 string          `json:"name"`
	BillingPeriod        string          `json:"billingPeriod,omitempty" db:"billingPeriod" goqu:"omitempty"`
	UseVat               bool            `json:"useVat" db:"useVat"`
	VatSupplementaryText string          `json:"vatSupplementaryText,omitempty" db:"vatSupplementaryText" goqu:"omitempty"`
	VatInPercent         IntOrString     `json:"vatInPercent" db:"vatInPercent"`
	AccountNetAmount     IntOrString     `json:"accountNetAmount" db:"accountNetAmount"`
	AccountGrossAmount   IntOrString     `json:"accountGrossAmount"  db:"accountGrossAmount"`
	ParticipantFee       float32         `json:"participantFee" db:"participantFee"`
	BaseFee              IntOrString     `json:"baseFee" db:"baseFee"`
	BusinessNr           null.Int        `json:"businessNr,string" db:"businessNr"`
	CentPerKWh           float32         `json:"centPerKWh" db:"centPerKWh"`
	FreeKWh              null.Int        `json:"freeKWh,omitempty,omitzero" db:"freeKWh"`
	Discount             null.Int        `json:"discount,omitempty,omitzero" db:"discount"`
	UseMeteringFee       bool            `json:"useMeteringPointFee"  db:"useMeteringPointFee"`
	MeteringFee          null.Float      `json:"meteringPointFee" db:"meteringPointFee"`
	MeteringVat          null.Int        `json:"meteringPointVat" db:"meteringPointVat"`
	CreatedAt            civil.NullDate  `json:"createdAt,omitempty" db:"createdDate" goqu:"omitempty,skipupdae,skipinsert"`
	InactiveSince        civil.NullDate  `json:"inactiveSince,omitempty" db:"inactiveSince" goqu:"omitempty,skipupdae,skipinsert"`
}

type TariffHistory struct {
}

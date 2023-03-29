package model

type EbMsMessageType string

const (
	EBMS_ENERGY_FILE_RESPONSE = "DATEN_CRMSG"
	EBMS_ONLINE_REG_ANSWER    = "ANTWORT_ECON"
	EBMS_ONLINE_REG_INIT      = "ANFORDERUNG_ECON"
	EBMS_ZP_LIST              = "ANFORDERUNG_ECP"
	EBMS_ZP_SYNC              = "ANFORDERUNG_PT"
	EBMS_ZP_LIST_RESPONSE     = "SENDEN_ECP"
	EBMS_EEG_BASE_DATA        = "ANFORDERUNG_GN"
	EBMS_ERROR_MESSAGE        = "ERROR_MESSAGE"
)

type Timeline struct {
	From int64 `json:"from"` // Date
	To   int64 `json:"to"`   // Date
}

type EnergyValue struct {
	From   int64  `json:"from"`
	To     int64  `json:"to,omitempty"`
	Method string `json:"method,omitempty"`
	Value  int64  `json:"value"`
}

type EnergyData struct {
	MeterCode string        `json:"meterCode"`
	Value     []EnergyValue `json:"value"`
}

type Energy struct {
	Start     int64        `json:"start"`
	End       int64        `json:"end"`
	Interval  string       `json:"interval"`
	NInterval int64        `json:"NInterval"`
	Data      []EnergyData `json:"data"`
}

type Meter struct {
	MeteringPoint string        `json:"meteringPoint"`
	Direction     DirectionType `json:"direction,omitempty"`
}

type ResponseData struct {
	MeteringPoint string  `json:"meteringPoint,omitempty"`
	ResponseCode  []int16 `json:"responseCode"`
}

type EbmsMessage struct {
	ConversationId string          `json:"conversationId"`
	MessageId      string          `json:"messageId,omitempty"`
	Sender         string          `json:"sender"`
	Receiver       string          `json:"receiver"`
	MessageCode    EbMsMessageType `json:"messageCode"`
	RequestId      string          `json:"requestId,omitempty"`
	Meter          *Meter          `json:"meter,omitempty"`
	EcId           string          `json:"ecId,omitempty"` // Community ID
	ResponseData   []ResponseData  `json:"responseData,omitempty"`
	Energy         *Energy         `json:"energy,omitempty"`
	Timeline       *Timeline       `json:"timeline,omitempty"`
	MeterList      []Meter         `json:"meterList,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
}

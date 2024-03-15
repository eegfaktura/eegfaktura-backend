package model

type EbMsMessageType string

var (
	ECON_RESPONSE_CODES = map[int16]string{
		99:  "Meldung erhalten",
		182: "Noch kein fernauslesbarer Zähler eingebaut",
		183: "Summe der gemeldeten Aufteilungsschlüssel übersteigt 100%",

		175: "Zustimmung erteilt",

		56:  "Zählpunkt nicht gefunden",
		184: "Kunde hat optiert",
		177: "Keine Datenfreigabe vorhanden",
		160: "Verteilmodell entspricht nicht der Vereinbarung",
		159: "Zu Prozessdatum ZP inaktiv bzw. noch kein Gerät eingebaut",
		158: "ZP ist nicht teilnahmeberechtigt",
		157: "ZP bereits einem Betreiber zugeordnet",
		156: "ZP bereits zugeordnet",
		86:  "konkurrierende Prozesse",
		181: "Gemeinschafts-ID nicht vorhanden",
		178: "Consent existiert bereits",
		174: "Angefragte Daten nicht lieferbar",
		173: "Kunde hat auf Datenfreigabe nicht reagiert (Timeout)",
		172: "Kunde hat Datenfreigabe abgelehnt",
		76:  "Ungültige Anforderungsdaten",
		57:  "Zählpunkt nicht versorgt",
		185: "Zählpunkt befindet sich nicht im Bereich der Energiegemeinschaft",
		37:  "Stornierung nicht möglich",

		55: "Zählpunkt nicht dem Lieferanten zugeordnet",
		70: "Änderung/Anforderung akzeptiert",
		82: "Prozessdatum falsch",
		90: "Kein Smart Meter",
		94: "Keine Daten im angeforderten Zeitraum vorhanden",
	}
	REJECTED_INVALID_CODES = []int16{56, 184, 177, 159, 158, 156, 86}
)

const (
	EBMS_ENERGY_FILE_RESPONSE  EbMsMessageType = "DATEN_CRMSG"
	EBMS_ONLINE_REG_INIT       EbMsMessageType = "ANFORDERUNG_ECON"
	EBMS_ONLINE_REG_ANSWER     EbMsMessageType = "ANTWORT_ECON"
	EBMS_ONLINE_REG_REJECTION  EbMsMessageType = "ABLEHNUNG_ECON"
	EBMS_ONLINE_REG_APPROVAL   EbMsMessageType = "ZUSTIMMUNG_ECON"
	EBMS_ONLINE_REG_COMPLETION EbMsMessageType = "ABSCHLUSS_ECON"
	EBMS_ZP_LIST               EbMsMessageType = "ANFORDERUNG_ECP"
	EBMS_ZP_SYNC               EbMsMessageType = "ANFORDERUNG_PT"
	EBMS_ZP_RES                EbMsMessageType = "ANTWORT_PT"
	EBMS_ZP_REJ                EbMsMessageType = "ABLEHNUNG_PT"
	EBMS_ZP_LIST_RESPONSE      EbMsMessageType = "SENDEN_ECP"
	EBMS_AUFHEBUNG_CCMI        EbMsMessageType = "AUFHEBUNG_CCMI"
	EBMS_AUFHEBUNG_CCMS        EbMsMessageType = "AUFHEBUNG_CCMS"
	EBMS_AUFHEBUNG_CCMC        EbMsMessageType = "AUFHEBUNG_CCMC"
	EBMS_ABLEHNUNG_CCMS        EbMsMessageType = "ABLEHNUNG_CCMS"
	EBMS_ANTWORT_CCMS          EbMsMessageType = "ANTWORT_CCMS"
	EBMS_EEG_BASE_DATA         EbMsMessageType = "ANFORDERUNG_GN"
	EBMS_ERROR_MESSAGE         EbMsMessageType = "ERROR_MESSAGE"
)

type EdaProtocol string

const (
	CR_MSG     EdaProtocol = "CR_MSG"
	CR_REQ_PT  EdaProtocol = "CR_REQ_PT"
	EC_PODLIST EdaProtocol = "EC_PODLIST"
	EC_REQ_ONL EdaProtocol = "EC_REQ_ONL"
	CM_REV_IMP EdaProtocol = "CM_REV_IMP"
	CM_REV_CUS EdaProtocol = "CM_REV_CUS"
	CM_REV_SP  EdaProtocol = "CM_REV_SP"
	ERROR      EdaProtocol = "ERROR"
)

type Timeline struct {
	From int64 `json:"from"` // Date
	To   int64 `json:"to"`   // Date
}

type EnergyValue struct {
	From   int64   `json:"from"`
	To     int64   `json:"to,omitempty"`
	Method string  `json:"method,omitempty"`
	Value  float64 `json:"value"`
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
	ConsentEnd    int64   `json:"consentEnd,omitempty"`
}

type EdaHistoryData struct {
	Meter        string   `json:"meter"`
	ResponseCode []string `json:"responseCode"`
	To           int64    `json:"to,omitempty"`
	From         int64    `json:"from,omitempty"`
	Method       string   `json:"method,omitempty"`
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
	ConsentEnd     int64           `json:"consentEnd,omitempty"`
	Reason         string          `json:"reason,omitempty"`
}

func (ebms EbmsMessage) Meters() []string {
	if ebms.Meter != nil {
		return []string{ebms.Meter.MeteringPoint}
	}
	if ebms.MeterList != nil && len(ebms.MeterList) > 0 {
		meters := []string{}
		for _, m := range ebms.MeterList {
			meters = append(meters, m.MeteringPoint)
		}
		return meters
	}
	return []string{}
}

func (ebms EbmsMessage) HistoryData() []EdaHistoryData {
	data := []EdaHistoryData{}
	for _, m := range ebms.Meters() {
		data = append(data, EdaHistoryData{
			Meter:        m,
			ResponseCode: ebms.ResponseCodes(),
			To:           0,
			From:         0,
			Method:       "",
		})
	}
	return data
}

func (ebms EbmsMessage) ResponseCodes() []string {
	codes := []string{}
	for _, r := range ebms.ResponseData {
		for _, c := range r.ResponseCode {
			codes = append(codes, ECON_RESPONSE_CODES[c])
		}
	}
	if len(codes) == 0 {
		return nil
	}
	return codes
}

// SubscribeMessage aggregates the result from subscribing.
type SubscribeMessage struct {
	// Reports the index of corresponding SubscribeTopic.
	MessageCode EbMsMessageType

	Protocol EdaProtocol

	// Determine the tenantId.
	Tenant string

	// Reports the payload content.
	Payload EbmsMessage
}

type SubscribeHandler func(msg SubscribeMessage)

type Subscriptions struct {
	Protocol EdaProtocol
	Handler  SubscribeHandler
}

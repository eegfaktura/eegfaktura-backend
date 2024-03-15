package eda

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"github.com/sirupsen/logrus"
	"time"
)

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

		55:  "Zählpunkt nicht dem Lieferanten zugeordnet",
		70:  "Änderung/Anforderung akzeptiert",
		82:  "Prozessdatum falsch",
		90:  "Kein Smart Meter",
		94:  "Keine Daten im angeforderten Zeitraum vorhanden",
		176: "Zustimmung erfolgreich entzogen",
	}
	REJECTED_INVALID_CODES = []int16{56, 57, 90, 158, 159, 177, 184, 185}
	REJECTED_VALID_CODES   = []int16{156, 86}
)

func InitEdaSubscription() {
	mqttclient.Subscribe(getSubsriptions()...)
}

func getSubsriptions() []model.Subscriptions {
	recorder := NewEdaRecorder()
	return []model.Subscriptions{
		{
			Protocol: model.CR_MSG,
			Handler: func(msg model.SubscribeMessage) {
				protocolCrMsgHandler(msg, recorder)
			},
		},
		{
			Protocol: model.CR_REQ_PT,
			Handler: func(msg model.SubscribeMessage) {
				protocolCrReqPtHandler(msg, recorder)
			},
		},
		{
			Protocol: model.EC_REQ_ONL,
			Handler: func(msg model.SubscribeMessage) {
				protocolEcReqOnlHandler(msg, recorder)
			},
		},
		{
			Protocol: model.CM_REV_IMP,
			Handler: func(msg model.SubscribeMessage) {
				protocolCmRevImpHandler(msg, recorder)
			},
		},
		{
			Protocol: model.CM_REV_CUS,
			Handler: func(msg model.SubscribeMessage) {
				protocolCmRevImpHandler(msg, recorder)
			},
		},
		{
			Protocol: model.CM_REV_SP,
			Handler: func(msg model.SubscribeMessage) {
				protocolCmRevImpHandler(msg, recorder)
			},
		},
	}
}

func protocolCrMsgHandler(msg model.SubscribeMessage, recorder EdaRecording) {
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v-%v", msg.Protocol, msg.MessageCode)

	if msg.Payload.Meter != nil && msg.Payload.Energy != nil {
		historyValue := map[string]interface{}{"meter": msg.Payload.Meter.MeteringPoint, "from": msg.Payload.Energy.Start, "to": msg.Payload.Energy.End}
		_ = recorder.saveHistory(msg.Tenant, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, historyValue)
	}
	return
}

func protocolCrReqPtHandler(msg model.SubscribeMessage, recorder EdaRecording) {
	//var err error
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v-%v", msg.Protocol, msg.MessageCode)

	codes := []int16{}

	switch msg.MessageCode {
	case model.EBMS_ZP_RES, model.EBMS_ZP_REJ, model.EBMS_ZP_SYNC:
		codes, _, _ = extractResponseCodeAndMeteringPoint(&msg.Payload)
	default:
		return
	}

	if err := recorder.saveNotification(map[string]interface{}{
		"type":           msg.MessageCode,
		"meteringPoints": msg.Payload.Meters(),
		"responseCodes":  convertCodes2Strings(codes),
	}, msg.Tenant, "EDA_PROCESS", "ADMIN"); err != nil {
		logrus.WithField("PROTOCOL", msg.Protocol).Error(err)
	}
	_ = recorder.saveHistory(msg.Tenant, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

func protocolEcReqOnlHandler(msg model.SubscribeMessage, recorder EdaRecording) {
	//var err error
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v-%v", msg.Protocol, msg.MessageCode)

	codes, meters, _ := extractResponseCodeAndMeteringPoint(&msg.Payload)
	var status model.StatusType

	switch msg.MessageCode {
	case model.EBMS_ONLINE_REG_COMPLETION:
		codes = []int16{}
		meters = extractMeterList(&msg.Payload)
		status = model.ACTIVE
	case model.EBMS_ONLINE_REG_REJECTION:
		if codesContains(REJECTED_INVALID_CODES, codes) {
			status = model.INVALID
		} else if codesContains(REJECTED_VALID_CODES, codes) {
			status = ""
		} else if codesContains([]int16{156}, codes) {
			status = model.ACTIVE
		} else {
			status = model.REJECTED
		}
	case model.EBMS_ONLINE_REG_APPROVAL:
		for _, c := range codes {
			if c == 175 {
				status = model.APPROVED
			}
		}
	case model.EBMS_ONLINE_REG_ANSWER:
		for _, c := range codes {
			if c == 99 {
				status = model.PENDING
				if err := recorder.meteringPointPerformAnswerMsg(msg.Tenant, meters); err != nil {
					logrus.WithField("error", err.Error()).Errorf("Perform Answer Message %+v", meters)
					return
				}
			}
		}
	case model.EBMS_ONLINE_REG_INIT:
		meters = msg.Payload.Meters()
		codes = []int16{}
	default:
		return
	}

	if len(meters) > 0 && len(status) > 0 {
		db, err := recorder.databaseConnection()
		if err != nil {
			logrus.WithField("tenant", msg.Tenant).Error(err)
			return
		}
		defer func() { _ = db.Close() }()

		if err := database.MeteringPointsSetStatus(db, msg.Tenant, status, meters); err != nil {
			logrus.WithField("error", err.Error()).Errorf("can not change metering point status %+v", meters)
			return
		}
	}

	if err := recorder.saveNotification(map[string]interface{}{
		"type":           msg.MessageCode,
		"meteringPoints": meters,
		"responseCodes":  convertCodes2Strings(codes),
	}, msg.Tenant, "EDA_PROCESS", "ADMIN"); err != nil {
		logrus.WithField("PROTOCOL", msg.Protocol).Error(err)
	}
	_ = recorder.saveHistory(msg.Tenant, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

func protocolCmRevImpHandler(msg model.SubscribeMessage, recorder EdaRecording) {
	//var err error
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v Code: %s", msg.Protocol, msg.MessageCode)

	meters, _ := extractResponseCodeAndMeteringPointV2(&msg.Payload)
	var status model.StatusType

	switch msg.MessageCode {
	case model.EBMS_AUFHEBUNG_CCMI, model.EBMS_AUFHEBUNG_CCMC:
		status = model.INACTIVE
	case model.EBMS_ANTWORT_CCMS:
		if len(meters) > 0 {
			if codesContains([]int16{176}, meters[0].codes) {
				meters[0].consentEnd = msg.Payload.ConsentEnd
				status = model.INACTIVE
			}
		}
	case model.EBMS_AUFHEBUNG_CCMS, model.EBMS_ABLEHNUNG_CCMS:
		status = ""
	default:
		return
	}

	if len(meters) > 0 && len(status) > 0 && meters[0].consentEnd > 0 {
		consentEnd := time.UnixMilli(meters[0].consentEnd).Local()
		db, err := recorder.databaseConnection()
		if err != nil {
			logrus.WithField("tenant", msg.Tenant).Error(err)
			return
		}
		defer func() { _ = db.Close() }()

		if err := database.MeteringPointRevoke(db, msg.Tenant, meters[0].meter, status, consentEnd); err != nil {
			logrus.WithField("tenant", msg.Tenant).Errorf("can not revoke metering point %+v - %+v", meters, err)
			return
		}
	}

	if len(meters) > 0 {
		if err := recorder.saveNotification(map[string]interface{}{
			"type":           msg.MessageCode,
			"meteringPoints": meters,
			"responseCodes":  convertCodes2Strings(meters[0].codes),
		}, msg.Tenant, "EDA_PROCESS", "ADMIN"); err != nil {
			logrus.WithField("PROTOCOL", msg.Protocol).Error(err)
		}
	}
	_ = recorder.saveHistory(msg.Tenant, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

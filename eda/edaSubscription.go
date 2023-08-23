package eda

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"fmt"
	"github.com/sirupsen/logrus"
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

		55: "Zählpunkt nicht dem Lieferanten zugeordnet",
		70: "Änderung/Anforderung akzeptiert",
		82: "Prozessdatum falsch",
		90: "Kein Smart Meter",
		94: "Keine Daten im angeforderten Zeitraum vorhanden",
	}
	REJECTED_INVALID_CODES = []int16{56, 184, 177, 159, 158, 156, 86}
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
				protcolCrMsgHandler(msg, recorder)
			},
		},
		{
			Protocol: model.CR_REQ_PT,
			Handler: func(msg model.SubscribeMessage) {
				protcolCrReqPtHandler(msg, recorder)
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
		//{
		//	MessageCode: model.EBMS_ONLINE_REG_ANSWER,
		//	Handler:     regAnswerHandler,
		//},
		//{
		//	MessageCode: model.EBMS_ONLINE_REG_REJECTION,
		//	Handler:     regAnswerHandler,
		//},
		//{
		//	MessageCode: model.EBMS_ONLINE_REG_APPROVAL,
		//	Handler:     regAnswerHandler,
		//},
		//{
		//	MessageCode: model.EBMS_ONLINE_REG_COMPLETION,
		//	Handler:     regCompletionHandler,
		//},
		//{
		//	MessageCode: model.EBMS_ZP_RES,
		//	Handler:     regAnswerHandler,
		//},
		//{
		//	MessageCode: model.EBMS_ZP_REJ,
		//	Handler:     regAnswerHandler,
		//},
		//{
		//	MessageCode: model.EBMS_AUFHEBUNG_CCMI,
		//	Handler:     regAnswerHandler,
		//},
		//{
		//	MessageCode: model.EBMS_AUFHEBUNG_CCMS,
		//	Handler:     regAnswerHandler,
		//},
		//{
		//	MessageCode: model.EBMS_ABLEHNUNG_CCMS,
		//	Handler:     regAnswerHandler,
		//},
		//{
		//	MessageCode: model.EBMS_ANTWORT_CCMS,
		//	Handler:     regAnswerHandler,
		//},
	}
}

func protcolCrMsgHandler(msg model.SubscribeMessage, recorder *EdaRecorder) {
	logrus.Printf("Handle Subscriptions: %+v", msg.Protocol)

	if msg.Payload.Meter != nil && msg.Payload.Energy != nil {
		historyValue := map[string]interface{}{"meter": msg.Payload.Meter.MeteringPoint, "from": msg.Payload.Energy.Start, "to": msg.Payload.Energy.End}
		_ = recorder.saveHistory(msg.Tenant, string(msg.MessageCode), msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, historyValue)
	}
	return
}

func protcolCrReqPtHandler(msg model.SubscribeMessage, recorder *EdaRecorder) {
	var err error
	logrus.Printf("Handle Subscriptions: %+v", msg.Protocol)

	codes := []int16{}

	switch msg.MessageCode {
	case model.EBMS_ZP_RES, model.EBMS_ZP_REJ, model.EBMS_ZP_SYNC:
		codes, _, _ = extractResponseCodeAndMeteringPoint(&msg.Payload)
	default:
		return
	}

	if err = recorder.saveNotification(map[string]interface{}{
		"type":           msg.MessageCode,
		"meteringPoints": msg.Payload.Meters(),
		"responseCodes":  convertCodes2Strings(codes),
	}, msg.Tenant, "NOTIFICATION", "ADMIN"); err != nil {
		logrus.WithField("PROTOCOL", msg.Protocol).Error(err)
	}
	_ = recorder.saveHistory(msg.Tenant, string(msg.MessageCode), msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

func protocolEcReqOnlHandler(msg model.SubscribeMessage, recorder *EdaRecorder) {
	var err error
	logrus.Printf("Handle Subscriptions: %+v", msg.Protocol)

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
		} else {
			status = model.REJECTED
		}
	case model.EBMS_ONLINE_REG_APPROVAL:
		for _, c := range codes {
			if c == 175 {
				status = model.APPROVED
			}
		}
	case model.EBMS_ONLINE_REG_INIT:
		codes = []int16{}
	default:
		return
	}

	if len(meters) > 0 && len(status) > 0 {
		if err := database.MeteringPointsSetStatus(msg.Tenant, status, meters); err != nil {
			logrus.WithField("error", err.Error()).Errorf("can not change metering point status %+v", meters)
			return
		}
	}

	if err = recorder.saveNotification(map[string]interface{}{
		"type":           msg.MessageCode,
		"meteringPoints": msg.Payload.Meters(),
		"responseCodes":  convertCodes2Strings(codes),
	}, msg.Tenant, "NOTIFICATION", "ADMIN"); err != nil {
		logrus.WithField("PROTOCOL", msg.Protocol).Error(err)
	}
	_ = recorder.saveHistory(msg.Tenant, string(msg.MessageCode), msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

func protocolCmRevImpHandler(msg model.SubscribeMessage, recorder *EdaRecorder) {
	var err error
	logrus.Printf("Handle Subscriptions: %+v", msg.Protocol)

	codes, meters, _ := extractResponseCodeAndMeteringPoint(&msg.Payload)
	var status model.StatusType

	switch msg.MessageCode {
	case model.EBMS_AUFHEBUNG_CCMI, model.EBMS_AUFHEBUNG_CCMS, model.EBMS_AUFHEBUNG_CCMC:
		status = model.REVOKED
	default:
		return
	}

	if len(meters) > 0 && len(status) > 0 {
		if err := database.MeteringPointsSetStatus(msg.Tenant, status, meters); err != nil {
			logrus.WithField("error", err.Error()).Errorf("can not change metering point status %+v", meters)
			return
		}
	}

	if err = recorder.saveNotification(map[string]interface{}{
		"type":           msg.MessageCode,
		"meteringPoints": msg.Payload.Meters(),
		"responseCodes":  convertCodes2Strings(codes),
	}, msg.Tenant, "NOTIFICATION", "ADMIN"); err != nil {
		logrus.WithField("PROTOCOL", msg.Protocol).Error(err)
	}
	_ = recorder.saveHistory(msg.Tenant, string(msg.MessageCode), msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

//func saveNotification(notificationValue map[string]interface{}, tenant, notifcationType, role string) error {
//	var msgBytes []byte
//	var err error
//	if msgBytes, err = json.Marshal(notificationValue); err == nil {
//		if err = database.SaveNotification(tenant, string(msgBytes), notifcationType, role); err != nil {
//			logrus.Error(err)
//			return err
//		}
//	}
//	return nil
//}
//
//func saveHistory(tenant, messageCode, conversationId, role, dir string, protocol model.EdaProtocol, msg interface{}) error {
//	db, err := database.GetDBXConnection()
//	if err != nil {
//		return err
//	}
//	defer db.Close()
//
//	var msgBytes []byte
//	if msgBytes, err = json.Marshal(msg); err == nil {
//		if err = database.SaveEdaHistory(db, &model.EdaProcessHistory{
//			Tenant:         tenant,
//			ConversationId: conversationId,
//			ProcessType:    messageCode,
//			Date:           time.Time{},
//			Protocol:       null.StringFrom(string(protocol)),
//			Issuer:         role,
//			MessageByte:    msgBytes,
//			MessageMap:     nil,
//			Direction:      dir,
//		}); err != nil {
//			logrus.Error(err)
//			return err
//		}
//	}
//	return nil
//}

func convertCodes2Strings(codes []int16) []string {
	strCodes := []string{}
	for _, c := range codes {
		sc, ok := ECON_RESPONSE_CODES[c]
		if !ok {
			sc = fmt.Sprintf("%d", c)
		}
		strCodes = append(strCodes, sc)
	}
	return strCodes
}

//func reqInitialHandler(msg model.SubscribeMessage) {
//	logrus.Printf("Handle Subscriptions: %+v", msg)
//	var msgBytes []byte
//	var err error
//	if msgBytes, err = json.Marshal(msg.Payload); err == nil {
//		if err = database.SaveEdaHistory(msg.Tenant, msg.Payload.ConversationId, "OUT", string(msgBytes), string(msg.MessageCode), "ADMIN"); err != nil {
//			logrus.Error(err)
//		}
//		return
//	}
//	logrus.Errorf("Parse object to json: %v", err)
//}
//func regAnswerHandler(msg model.SubscribeMessage) {
//	fmt.Printf("Handle EDA MESSAGE: %+v\n", msg)
//	responseCode, meter, err := extractResponseCodeAndMeteringPoint(&msg.Payload)
//	if err != nil {
//		logrus.Error(err)
//		return
//	}
//	resp, ok := ECON_RESPONSE_CODES[responseCode]
//	if !ok {
//		resp = fmt.Sprintf("%d", responseCode)
//	}
//	notificationValue := map[string]interface{}{
//		"type":          msg.MessageCode,
//		"meteringPoint": meter,
//		"responseCode":  resp}
//
//	var status model.StatusType = model.REJECTED
//	switch responseCode {
//	case 175:
//		status = model.APPROVED
//		break
//	case 99:
//		status = model.PENDING
//		break
//	}
//
//	if msg.MessageCode == model.EBMS_AUFHEBUNG_CCMI ||
//		msg.MessageCode == model.EBMS_AUFHEBUNG_CCMS {
//		status = model.REVOKED
//	}
//
//	if len(meter) > 0 {
//		if err := database.MeteringPointsSetStatus(msg.Tenant, status, []string{meter}); err != nil {
//			logrus.WithField("error", err.Error()).Errorf("can not change metering point status %+v", meter)
//			return
//		}
//	}
//
//	var msgBytes []byte
//	if msgBytes, err = json.Marshal(notificationValue); err == nil {
//		if err = database.SaveNotification(msg.Tenant, string(msgBytes), "NOTIFICATION", "ADMIN"); err != nil {
//			logrus.Error(err)
//		}
//	}
//
//	if msgBytes, err = json.Marshal(msg.Payload); err == nil {
//		if err = database.SaveEdaHistory(msg.Tenant, msg.Payload.ConversationId, "IN", string(msgBytes), string(msg.MessageCode), "ADMIN"); err != nil {
//			logrus.Error(err)
//		}
//		return
//	}
//	logrus.Errorf("Parse object to json: %v", err)
//}
//
//func regCompletionHandler(msg model.SubscribeMessage) {
//	meterIds := []string{}
//	for _, m := range msg.Payload.MeterList {
//		meterIds = append(meterIds, m.MeteringPoint)
//	}
//
//	if len(meterIds) > 0 {
//		if err := database.MeteringPointsSetStatus(msg.Tenant, model.ACTIVE, meterIds); err != nil {
//			logrus.WithField("error", err.Error()).Errorf("can not activate metering points %+v", meterIds)
//			return
//		}
//	}
//
//	notificationValue := map[string]interface{}{
//		"type":           msg.MessageCode,
//		"meteringPoints": meterIds}
//
//	var err error
//	var msgBytes []byte
//	if msgBytes, err = json.Marshal(notificationValue); err != nil {
//		logrus.Errorf("Parse object to json: %v", err)
//		return
//	}
//	if err = database.SaveNotification(msg.Tenant, string(msgBytes), "NOTIFICATION", "USER"); err != nil {
//		logrus.Error(err)
//	}
//	if msgBytes, err = json.Marshal(msg.Payload); err == nil {
//		if err = database.SaveEdaHistory(msg.Tenant, msg.Payload.ConversationId, "IN", string(msgBytes), string(msg.MessageCode), "ADMIN"); err != nil {
//			logrus.Error(err)
//		}
//		return
//	}
//	logrus.Errorf("Parse object to json: %v", err)
//}

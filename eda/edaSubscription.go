package eda

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"at.ourproject/vfeeg-backend/services"
	"bytes"
	"fmt"
	"github.com/jjeffery/civil"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
)

var (
	ECON_RESPONSE_CODES = map[int16]string{
		37:  "Stornierung nicht möglich",
		55:  "Zählpunkt nicht dem Lieferanten zugeordnet",
		56:  "Zählpunkt nicht gefunden",
		57:  "Zählpunkt nicht versorgt",
		70:  "Änderung/Anforderung akzeptiert",
		76:  "Ungültige Anforderungsdaten",
		82:  "Prozessdatum falsch",
		86:  "konkurrierende Prozesse",
		90:  "Kein Smart Meter",
		94:  "Keine Daten im angeforderten Zeitraum vorhanden",
		99:  "Meldung erhalten",
		104: "Falsche Energierichtung",
		156: "ZP bereits zugeordnet",
		157: "ZP bereits einem Betreiber zugeordnet",
		158: "ZP ist nicht teilnahmeberechtigt",
		159: "Zu Prozessdatum ZP inaktiv bzw. noch kein Gerät eingebaut",
		160: "Verteilmodell entspricht nicht der Vereinbarung",
		172: "Kunde hat Datenfreigabe abgelehnt",
		173: "Kunde hat auf Datenfreigabe nicht reagiert (Timeout)",
		174: "Angefragte Daten nicht lieferbar",
		175: "Zustimmung erteilt",
		176: "Zustimmung erfolgreich entzogen",
		177: "Keine Datenfreigabe vorhanden",
		178: "Consent existiert bereits",
		180: "ConsentID abgelaufen",
		181: "Gemeinschafts-ID nicht vorhanden",
		182: "Noch kein fernauslesbarer Zähler eingebaut",
		183: "Summe der gemeldeten Aufteilungsschlüssel übersteigt 100%",
		184: "Kunde hat optiert",
		185: "Zählpunkt befindet sich nicht im Bereich der Energiegemeinschaft",
		187: "ConsentID und Zählpunkt passen nicht zusammen",
		188: "Teilnahmefaktor von 100 % würde überschritten werden",
		189: "Zählpunkt ist der Gemeinschafts-ID nicht zugeordnet",
		196: "Teilnahme-Limit wird überschritten",
	}
	REJECTED_INVALID_CODES = []int16{56, 57, 76, 104, 157, 158, 159, 172, 173, 177, 181, 184, 185, 188, 196}
	REJECTED_VALID_CODES   = []int16{156}
	REJECTED_IGNORE_CODES  = []int16{86}
)

func InitEdaSubscription() {
	mqttclient.Subscribe(getSubsriptions()...)
}

func getSubsriptions() []model.Subscriptions {
	recorder := NewEdaRecorder()
	//if err := recorder.meteringPointPerformAnswerMsg("CC100392", []string{"AT0030000000000000000000000433950"}); err != nil {
	//	logrus.WithField("tenant", "CC100392").Errorf("E: %v", err)
	//}
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
			Protocol: model.EC_REQ_OFF,
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
		{
			Protocol: model.EC_PRTFACT_CHANGE,
			Handler: func(msg model.SubscribeMessage) {
				protocolEcPrtChangeHandler(msg, recorder)
			},
		},
		{
			Protocol: model.EC_PODLIST,
			Handler: func(msg model.SubscribeMessage) {
				protocolEcPodListHandler(msg, recorder)
			},
		},
	}
}

func protocolCrMsgHandler(msg model.SubscribeMessage, recorder EdaRecording) {
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v-%v", msg.Protocol, msg.MessageCode)

	db, err := recorder.databaseConnection()
	if err != nil {
		logrus.WithField("tenant", msg.Tenant).Error(err)
		return
	}
	defer func() { _ = db.Close() }()

	eeg, err := database.GetEegByEcId(db, msg.Payload.EcId)
	if err != nil {
		logrus.WithField("tenant", msg.Tenant).WithError(err).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
		return
	}

	if msg.Payload.Meter != nil && msg.Payload.Energy != nil {
		historyValue := map[string]interface{}{"meter": msg.Payload.Meter.MeteringPoint, "from": msg.Payload.Energy.Start, "to": msg.Payload.Energy.End}
		_ = recorder.saveHistory(eeg.Id, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, historyValue)
	}
	return
}

func protocolCrReqPtHandler(msg model.SubscribeMessage, recorder EdaRecording) {
	//var err error
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v-%v", msg.Protocol, msg.MessageCode)

	codes := []int16{}

	switch msg.MessageCode {
	case model.EBMS_ZP_RES, model.EBMS_ZP_REJ, model.EBMS_ZP_SYNC:
		codes, _, _, _ = extractResponseCodeAndMeteringPoint(&msg.Payload)
	default:
		return
	}

	db, err := recorder.databaseConnection()
	if err != nil {
		logrus.WithField("tenant", msg.Tenant).Error(err)
		return
	}
	defer func() { _ = db.Close() }()

	eeg, err := database.GetEegByEcId(db, msg.Payload.EcId)
	if err != nil {
		logrus.WithField("error", err.Error()).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
		return
	}

	if err := recorder.saveNotification(map[string]interface{}{
		"type":           msg.MessageCode,
		"meteringPoints": msg.Payload.Meters(),
		"responseCodes":  convertCodes2Strings(codes),
	}, eeg.Id, "EDA_PROCESS", "ADMIN"); err != nil {
		logrus.WithField("PROTOCOL", msg.Protocol).Error(err)
	}
	_ = recorder.saveHistory(eeg.Id, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

// protocolEcReqOnlHandler executing the EC_REQ_ONL eda process (Online Meteringpoint registration). This process exist of the following steps
// - ANFORDERUNG_ECON: Initial request to open the process
// - ANTWORT_ECON: Request. The process was successfully started
// - ZUSTIMMUNG_ECON: Participant accepts the request in the user portal of the grid operator
// - ABSCHLUSS_ECON: Metering point is part of the EEG
// - ABLEHNUNG_ECON: The process was aborted
//
// During the process the Meteringpoint is taged with different status flags.
// - NEW: Meteringpoint is assosiated to a participant. The process is not started.
// - PENDING: ANFORDERUNG_ECON already sent and confirmed by partner with ANSWER_ECON message
// - APPROVED: ZUSTIMMUNG_ECON. Participant accept it in the grid operator portal
// - ACTIVE: ABSCHLUSS_ECON. Metering is activ
// - INVALID: Meteringpoint is rejected accourding to wrong attributes
// - REJECTED: Meteringpoint is rejected
// - ARCHIVED: Meteringpoint was archived by the participant
func protocolEcReqOnlHandler(msg model.SubscribeMessage, recorder EdaRecording) {
	//var err error
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v-%v", msg.Protocol, msg.MessageCode)
	getConsentId := func(consentId *string) *string {
		if consentId == nil {
			return nil
		}
		cId := strings.TrimSpace(*consentId)
		if len(cId) == 0 {
			return nil
		}
		return &cId
	}

	codes, meters, consentIds, _ := extractResponseCodeAndMeteringPoint(&msg.Payload)
	var status model.StatusType
	var statusCode *int16
	var activeSince civil.NullDate
	var consentId *string

	switch msg.MessageCode {
	case model.EBMS_ONLINE_REG_COMPLETION, model.EBMS_OFFLINE_REG_COMPLETION:
		codes = []int16{}
		completeMeters := extractMeterList(&msg.Payload)
		if len(completeMeters) == 1 {
			meters = []string{completeMeters[0].meter}
			ax := civil.DateOf(completeMeters[0].activeSince)
			activeSince = civil.NullDateFrom(&ax)
			consentId = completeMeters[0].consentId
			status = model.ACTIVE
		} else {
			status = ""
			meters = []string{}
		}

	case model.EBMS_ONLINE_REG_REJECTION, model.EBMS_OFFLINE_REG_REJECTION:
		if codesContains(REJECTED_VALID_CODES, codes) {
			status = model.ACTIVE
			codes = []int16{0}
		} else if codesContains(REJECTED_INVALID_CODES, codes) {
			status = model.INVALID
			statusCode = &intersectCodes(REJECTED_INVALID_CODES, codes)[0]
		} else if codesContains(REJECTED_IGNORE_CODES, codes) {
			status = ""
			codes = []int16{0}
		} else {
			status = model.REJECTED
			if len(codes) > 0 {
				statusCode = &codes[0]
			}
		}
	case model.EBMS_ONLINE_REG_APPROVAL, model.EBMS_OFFLINE_REG_APPROVAL:
		for i, c := range codes {
			if c == 175 {
				status = model.APPROVED
				consentId = &consentIds[i]
			}
		}
	case model.EBMS_ONLINE_REG_ANSWER, model.EBMS_OFFLINE_REG_ANSWER:
		for _, c := range codes {
			if c == 99 {
				status = model.PENDING
				if err := recorder.meteringPointPerformAnswerMsg(msg.Payload.EcId, meters); err != nil {
					logrus.WithField("error", err.Error()).Errorf("Perform Answer Message %+v", meters)
					return
				}
			} else if c == 182 || c == 183 {
				status = model.INVALID
				statusCode = &c
			}
		}
	case model.EBMS_ONLINE_REG_INIT, model.EBMS_OFFLINE_REG_INIT:
		meters = msg.Payload.Meters()
		codes = []int16{0}
		status = model.INIT
	default:
		return
	}

	db, err := recorder.databaseConnection()
	if err != nil {
		logrus.WithField("tenant", msg.Tenant).Error(err)
		return
	}
	defer func() { _ = db.Close() }()

	eeg, err := database.GetEegByEcId(db, msg.Payload.EcId)
	if err != nil {
		logrus.WithField("error", err.Error()).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
		return
	}

	if len(meters) > 0 && len(status) > 0 {
		if err := database.MeteringPointsSetStatus(db, eeg.Id, status, statusCode, meters, activeSince.Ptr(), getConsentId(consentId)); err != nil {
			logrus.WithField("error", err.Error()).Errorf("can not change metering point status %+v", meters)
		}
	}

	if err := recorder.saveNotification(map[string]interface{}{
		"type":           msg.MessageCode,
		"meteringPoints": meters,
		"responseCodes":  convertCodes2Strings(codes),
	}, eeg.Id, "EDA_PROCESS", "ADMIN"); err != nil {
		logrus.WithField("PROTOCOL", msg.Protocol).Error(err)
	}
	_ = recorder.saveHistory(eeg.Id, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

func protocolCmRevImpHandler(msg model.SubscribeMessage, recorder EdaRecording) {
	//var err error
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v Code: %s", msg.Protocol, msg.MessageCode)

	meters, _ := extractResponseCodeAndMeteringPointV2(&msg.Payload)

	db, err := recorder.databaseConnection()
	if err != nil {
		logrus.WithField("tenant", msg.Tenant).Error(err)
		return
	}
	defer func() { _ = db.Close() }()

	var eeg *model.Eeg
	switch msg.MessageCode {
	case model.EBMS_AUFHEBUNG_CCMS, model.EBMS_ABLEHNUNG_CCMS:
		eeg, err = database.GetEegByEcId(db, msg.Payload.EcId)
		if err != nil {
			logrus.WithField("tenant", msg.Tenant).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
			return
		}
	case model.EBMS_AUFHEBUNG_CCMC, model.EBMS_AUFHEBUNG_CCMI:

		var tenant *string
		if tenant, err = database.MeteringPointRevokeByConsentId(db, meters[0].consentId, meters[0].meter, meters[0].consentEnd); err != nil {
			logrus.WithField("tenant", msg.Tenant).Errorf("can not revoke metering point %+v - %+v", meters, err)
			return
		}

		eeg, err = database.GetEegById(db, *tenant)
		if err != nil {
			logrus.WithField("tenant", *tenant).Errorf("can not fetch eeg by tenant %s (REVOKE metering point)", *tenant)
			return
		}

	case model.EBMS_ANTWORT_CCMS:
		if len(meters) > 0 {
			if codesContains([]int16{176}, meters[0].codes) {
				meters[0].consentEnd = civil.DateOf(time.UnixMilli(msg.Payload.ConsentEnd))
				eeg, err = database.GetEegByEcId(db, msg.Payload.EcId)
				if err != nil {
					logrus.WithField("tenant", msg.Tenant).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
					return
				}

				if err := database.MeteringPointRevoke(db, eeg.Id, meters[0].meter, meters[0].consentEnd); err != nil {
					logrus.WithField("tenant", eeg.Id).Errorf("can not revoke metering point %+v - %+v", meters, err)
					return
				}
			}
		}
	default:
		return
	}

	if eeg != nil {
		if len(meters) > 0 {
			if err := recorder.saveNotification(map[string]interface{}{
				"type":           msg.MessageCode,
				"meteringPoints": []string{meters[0].meter},
				"responseCodes":  convertCodes2Strings(meters[0].codes),
			}, eeg.Id, "EDA_PROCESS", "ADMIN"); err != nil {
				logrus.WithField("PROTOCOL", msg.Protocol).Error(err)
			}
		}
		_ = recorder.saveHistory(eeg.Id, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
	} else {
		logrus.WithField("tenant", msg.Tenant).Errorf("%+v", msg.Payload)
	}
}

func protocolEcPrtChangeHandler(msg model.SubscribeMessage, recorder EdaRecording) {
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v Code: %s", msg.Protocol, msg.MessageCode)

	var meters []model.Meter
	var errCode int16
	switch msg.MessageCode {
	case model.EBMS_REJ_CHANGE_PARTFACT:
		if len(msg.Payload.ResponseData) > 0 && len(msg.Payload.ResponseData[0].ResponseCode) > 0 {
			errCode = msg.Payload.ResponseData[0].ResponseCode[0]
		} else {
			errCode = 1000
		}
		break
	case model.EBMS_ANS_CHANGE_PARTFACT:
		meters = msg.Payload.MeterList
		errCode = 0
		break
	case model.EBMS_REQ_CHANGE_PARTFACT:
		meters = nil
		break
	default:
		logrus.WithField("tenant", msg.Tenant).Warnf("Unknown Messagecode: %v", msg)
		return
	}

	db, err := recorder.databaseConnection()
	if err != nil {
		logrus.Error(err)
		return
	}
	defer func() { _ = db.Close() }()

	eeg, err := database.GetEegByEcId(db, msg.Payload.EcId)
	if err != nil {
		logrus.Errorf("can not fetch eeg with message -> %+v", msg.Payload)
		return
	}

	if len(meters) > 0 && errCode == 0 {
		if err := database.MeteringPointChangePartFactor(db, eeg.Id, meters); err != nil {
			logrus.WithField("tenant", eeg.Id).Errorf("can not change partition factor. %v", err)
			return
		}
	}

	if errCode > 0 {
		if err := recorder.saveNotification(map[string]interface{}{
			"type":           msg.MessageCode,
			"meteringPoints": meters,
			"responseCodes":  convertCodes2Strings([]int16{errCode}),
		}, eeg.Id, "EDA_PROCESS", "ADMIN"); err != nil {
			logrus.WithField("PROTOCOL", msg.Protocol).Error(err)
		}
	}
	_ = recorder.saveHistory(eeg.Id, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

func protocolEcPodListHandler(msg model.SubscribeMessage, recorder EdaRecording) {
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v Code: %s", msg.Protocol, msg.MessageCode)

	switch msg.MessageCode {
	case model.EBMS_ZP_LIST:
	case model.EBMS_ZP_LIST_RESPONSE:
		buf, err := database.ExportZPListToExcel(&msg.Payload)
		if err != nil {
			return
		}

		db, err := recorder.databaseConnection()
		if err != nil {
			logrus.Error(err)
			return
		}
		defer func() { _ = db.Close() }()

		eeg, err := database.GetEegByEcId(db, msg.Payload.EcId)
		if err != nil {
			logrus.Errorf("can not fetch eeg with message %+v", msg.Payload)
			return
		}

		if eeg.Email.Valid {
			now := time.Now()
			attm := &services.Attachment{
				Type:        "DEFAULT",
				Filename:    fmt.Sprintf("%s_%.4d%.2d%.2d-%.2d%.2d_ZP_PODLIST.xlsx", eeg.RcNumber, now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute()),
				Filecontent: buf,
				MimeType:    "application/vnd.ms-excel",
				ContentId:   nil,
			}
			var b bytes.Buffer
			b.WriteString(fmt.Sprintf("Zählpunktlist für EEG - %s", eeg.RcNumber))
			err = services.SendMailWithAttachment(fmt.Sprintf("%s@eegfaktura.at", eeg.RcNumber),
				eeg.Email.String,
				fmt.Sprintf("%s Zählpunktliste %.4d%.2d%.2d-%.2d%.2d", eeg.RcNumber, now.Year(), int(now.Month()), now.Day(), now.Hour(), now.Minute()), nil, &b, attm)
			if err != nil {
				logrus.WithField("tenant", msg.Tenant).Error(err)
			}
		}
		services.SyncMeteringPoints(msg.Tenant, &msg.Payload)

	case model.EBMS_ZP_LIST_REJECTION:
	default:
		logrus.WithField("tenant", msg.Tenant).Warnf("Unknown Messagecode: %v", msg)
		return
	}
	_ = recorder.saveHistory(msg.Tenant, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

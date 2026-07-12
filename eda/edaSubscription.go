package eda

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"at.ourproject/vfeeg-backend/parser"
	"at.ourproject/vfeeg-backend/services"
	"github.com/jjeffery/civil"
	"github.com/sirupsen/logrus"
)

// publishEnergyForEnergystore ist als Variable definiert, damit Tests die
// MQTT-Bruecke ohne laufenden Broker beobachten koennen.
var publishEnergyForEnergystore = mqttclient.PublishRaw

const (
	Stornierung_nicht_moeglich                                        = 37
	Zaehlpunkt_nicht_dem_Lieferanten_zugeordnet                       = 55
	ZP_NOT_FOUND                                                      = 56
	ZP_NOT_SUPPLIED                                                   = 57
	Aenderung_Anforderung_akzeptiert                                  = 70
	INVALID_REQUEST_DATA                                              = 76
	Prozessdatum_falsch                                               = 82
	COMPETING_PROCESSES                                               = 86
	Kein_Smart_Meter                                                  = 90
	Keine_Daten_im_angeforderten_Zeitraum_vorhanden                   = 94
	MESSAGE_RECEIVED                                                  = 99
	WRONG_ENERGY_DIRECTION                                            = 104
	ZP_ALREADY_ASSIGNED                                               = 156
	ZP_ALREADY_ASSIGNED_TO_AN_OPERATOR                                = 157
	ZP_IS_NOT_ELIGIBLE                                                = 158
	Zu_Prozessdatum_ZP_inaktiv_bzw_noch_kein_Geraet_eingebaut         = 159
	Verteilmodell_entspricht_nicht_der_Vereinbarung                   = 160
	CUSTOMER_HAS_REFUSED_DATA_RELEASE                                 = 172
	CUSTOMER_DID_NOT_RESPOND_TO_DATA_RELEASE                          = 173
	Angefragte_Daten_nicht_lieferbar                                  = 174
	CONSENT_GRANTED                                                   = 175
	Zustimmung_erfolgreich_entzogen                                   = 176
	Keine_Datenfreigabe_vorhanden                                     = 177
	Consent_existiert_bereits                                         = 178
	ConsentID_abgelaufen                                              = 180
	GemeinschaftsID_nicht_vorhanden                                   = 181
	NO_REMOTELY_READABLE_METER_INSTALLED_YET                          = 182
	SUM_OF_REPORTED_ALLOCATION_KEYS_EXCEEDS_100                       = 183
	Kunde_hat_optiert                                                 = 184
	Zaehlpunkt_befindet_sich_nicht_im_Bereich_der_Energiegemeinschaft = 185
	ConsentID_und_Zaehlpunkt_passen_nicht_zusammen                    = 187
	PARTICIPATION_FACTOR_OF_100_WOULD_BE_EXCEEDED                     = 188
	Zaehlpunkt_ist_der_GemeinschaftsID_nicht_zugeordnet               = 189
	TeilnahmeLimit_wird_ueberschritten                                = 196
	CONSENT_WAS_WITHDRAWN                                             = 203
	NO_STABLE_COMMUNICATION_POSSIBLE                                  = 204
)

var (
	ECON_RESPONSE_CODES = map[int16]string{
		37:   "Stornierung nicht möglich",
		55:   "Zählpunkt nicht dem Lieferanten zugeordnet",
		56:   "Zählpunkt nicht gefunden",
		57:   "Zählpunkt nicht versorgt",
		70:   "Änderung/Anforderung akzeptiert",
		76:   "Ungültige Anforderungsdaten",
		82:   "Prozessdatum falsch",
		86:   "konkurrierende Prozesse",
		90:   "Kein Smart Meter",
		94:   "Keine Daten im angeforderten Zeitraum vorhanden",
		99:   "Meldung erhalten",
		104:  "Falsche Energierichtung",
		156:  "ZP bereits zugeordnet",
		157:  "ZP bereits einem Betreiber zugeordnet",
		158:  "ZP ist nicht teilnahmeberechtigt",
		159:  "Zu Prozessdatum ZP inaktiv bzw. noch kein Gerät eingebaut",
		160:  "Verteilmodell entspricht nicht der Vereinbarung",
		172:  "Kunde hat Datenfreigabe abgelehnt",
		173:  "Kunde hat auf Datenfreigabe nicht reagiert (Timeout)",
		174:  "Angefragte Daten nicht lieferbar",
		175:  "Zustimmung erteilt",
		176:  "Zustimmung erfolgreich entzogen",
		177:  "Keine Datenfreigabe vorhanden",
		178:  "Consent existiert bereits",
		180:  "ConsentID abgelaufen",
		181:  "Gemeinschafts-ID nicht vorhanden",
		182:  "Noch kein fernauslesbarer Zähler eingebaut",
		183:  "Summe der gemeldeten Aufteilungsschlüssel übersteigt 100%",
		184:  "Kunde hat optiert",
		185:  "Zählpunkt befindet sich nicht im Bereich der Energiegemeinschaft",
		187:  "ConsentID und Zählpunkt passen nicht zusammen",
		188:  "Teilnahmefaktor von 100 % würde überschritten werden",
		189:  "Zählpunkt ist der Gemeinschafts-ID nicht zugeordnet",
		196:  "Teilnahme-Limit wird überschritten",
		203:  "Zustimmung wurde entzogen",
		204:  "Für ZP ist derzeit keine ausreichend stabile Kommunikation möglich",
		1000: "Mail konnte nicht gesendet werden.",
	}
	REJECTED_INVALID_CODES = []int16{ZP_NOT_FOUND, ZP_NOT_SUPPLIED, INVALID_REQUEST_DATA, WRONG_ENERGY_DIRECTION,
		ZP_ALREADY_ASSIGNED_TO_AN_OPERATOR, ZP_IS_NOT_ELIGIBLE, 159, 172, 173, 177, 181, 184, 185, 188, 196, NO_STABLE_COMMUNICATION_POSSIBLE}
	REJECTED_VALID_CODES  = []int16{ZP_ALREADY_ASSIGNED}
	REJECTED_IGNORE_CODES = []int16{COMPETING_PROCESSES}
)

//func InitEdaSubscription() {
//	mqttclient.Subscribe(getSubsriptions()...)
//}

func InitEdaSubscription(ctx context.Context) {
	mqttclient.Broker().Subscribe(getSubsriptions(ctx)...)
}

func getSubsriptions(ctx context.Context) []model.Subscriptions {

	return []model.Subscriptions{
		{
			Protocol: model.CR_MSG,
			Handler: func(msg model.SubscribeMessage) {
				protocolCrMsgHandler(ctx, msg)
			},
		},
		{
			Protocol: model.CR_REQ_PT,
			Handler: func(msg model.SubscribeMessage) {
				protocolCrReqPtHandler(ctx, msg)
			},
		},
		{
			Protocol: model.EC_REQ_ONL,
			Handler: func(msg model.SubscribeMessage) {
				protocolEcReqOnlHandler(ctx, msg)
			},
		},
		{
			Protocol: model.EC_REQ_OFF,
			Handler: func(msg model.SubscribeMessage) {
				protocolEcReqOnlHandler(ctx, msg)
			},
		},
		{
			Protocol: model.CM_REV_IMP,
			Handler: func(msg model.SubscribeMessage) {
				protocolCmRevImpHandler(ctx, msg)
			},
		},
		{
			Protocol: model.CM_REV_CUS,
			Handler: func(msg model.SubscribeMessage) {
				protocolCmRevImpHandler(ctx, msg)
			},
		},
		{
			Protocol: model.CM_REV_SP,
			Handler: func(msg model.SubscribeMessage) {
				protocolCmRevImpHandler(ctx, msg)
			},
		},
		{
			Protocol: model.EC_PRTFACT_CHANGE,
			Handler: func(msg model.SubscribeMessage) {
				protocolEcPrtChangeHandler(ctx, msg)
			},
		},
		{
			Protocol: model.EC_PODLIST,
			Handler: func(msg model.SubscribeMessage) {
				protocolEcPodListHandler(ctx, msg)
			},
		},
	}
}

func protocolCrMsgHandler(ctx context.Context, msg model.SubscribeMessage) {
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v-%v", msg.Protocol, msg.MessageCode)

	db, err := database.GetDB(ctx)
	if err != nil {
		logrus.WithField("tenant", msg.Tenant).Error(err)
		return
	}

	eeg, err := db.GetEegByEcId(ctx, msg.Payload.EcId)
	if err != nil {
		logrus.WithField("tenant", msg.Tenant).WithError(err).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
		return
	}

	if msg.Payload.Meter != nil && msg.Payload.Energy != nil {
		for i := range msg.Payload.Energy {
			energy := msg.Payload.Energy[i]
			historyValue := map[string]interface{}{"meter": msg.Payload.Meter.MeteringPoint, "from": energy.Start, "to": energy.End}
			_ = db.SaveHistory(eeg.Id, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", "CR_MSG", historyValue)
		}

		// Bridge: AT003100 (und vermutlich andere Netzbetreiber) liefert die
		// ConsumptionRecord-Quartiers-Werte direkt im CR_MSG-Payload statt
		// als separate Mail auf dem Energy-Topic. Ohne diese Weiterleitung
		// landen die Werte nur als from/to-Range im processhistory und
		// gehen fuer die Auswertung verloren. Wir transformieren auf die
		// v1/MqttEnergyResponse-Wire-Shape die eegfaktura-energystore-v2
		// ohnehin schon konsumiert und veroeffentlichen einen Datensatz
		// pro Energy-Eintrag auf `eda/response/energy/<tenant-lowercase>`.
		forwardEnergyToEnergystore(msg)
	}
	return
}

// forwardEnergyToEnergystore wandelt jeden Eintrag in msg.Payload.Energy
// in das von energystore-v2 erwartete Format (MqttEnergyResponse) um und
// publisht das jeweils auf `eda/response/energy/<tenant-lowercase>`. Bei
// Marshal-/Publish-Fehlern wird geloggt und fortgefahren — der DLQ-Pfad
// in energystore-v2 (mqtt_dlq) faengt schlechte Payloads ab und der
// History-Eintrag in base.processhistory bleibt unangetastet.
func forwardEnergyToEnergystore(msg model.SubscribeMessage) {
	if msg.Payload.Meter == nil {
		return
	}
	topic := fmt.Sprintf("eda/response/energy/%s", strings.ToLower(msg.Tenant))
	meterId := msg.Payload.Meter.MeteringPoint
	direction := string(msg.Payload.Meter.Direction)

	for _, e := range msg.Payload.Energy {
		wire := energyWire{}
		wire.Message.Meter.MeteringPoint = meterId
		wire.Message.Meter.Direction = direction
		wire.Message.Energy.Start = e.Start
		wire.Message.Energy.End = e.End
		wire.Message.Energy.Data = e.Data
		wire.Message.EcID = msg.Payload.EcId

		payload, err := json.Marshal(wire)
		if err != nil {
			logrus.WithField("tenant", msg.Tenant).WithField("meter", meterId).
				WithError(err).Error("CR_MSG -> energy bridge: marshal failed")
			continue
		}
		publishEnergyForEnergystore(topic, payload)
		logrus.WithField("tenant", msg.Tenant).WithField("meter", meterId).
			WithField("start", e.Start).WithField("end", e.End).
			WithField("series", len(e.Data)).
			Info("CR_MSG -> energy bridge: published to energystore-v2 topic")
	}
}

// energyWire matched eegfaktura-energystore-v2's MqttEnergyResponse /
// MqttEnergyMessage struct. Wir halten die Definition lokal, damit dieses
// Modul keine cross-repo Go-Dependency braucht — Wire-Shape ist seit v1
// (BadgerDB) stabil.
type energyWire struct {
	Message struct {
		Meter struct {
			MeteringPoint string `json:"meteringPoint"`
			Direction     string `json:"direction,omitempty"`
		} `json:"meter"`
		Energy struct {
			Start int64              `json:"start"`
			End   int64              `json:"end"`
			Data  []model.EnergyData `json:"data"`
		} `json:"energy"`
		EcID string `json:"ecId"`
	} `json:"message"`
}

func protocolCrReqPtHandler(ctx context.Context, msg model.SubscribeMessage) {
	//var err error
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v-%v", msg.Protocol, msg.MessageCode)

	codes := []int16{}

	switch msg.MessageCode {
	case model.EBMS_ZP_RES, model.EBMS_ZP_REJ, model.EBMS_ZP_SYNC:
		codes, _, _, _ = extractResponseCodeAndMeteringPoint(&msg.Payload)
	default:
		return
	}

	db, err := database.GetDB(ctx)
	if err != nil {
		logrus.WithField("tenant", msg.Tenant).Error(err)
		return
	}

	eeg, err := db.GetEegByEcId(ctx, msg.Payload.EcId)
	if err != nil {
		logrus.WithField("error", err.Error()).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
		return
	}

	_ = db.SaveNotification(eeg.Id, msg.MessageCode, msg.Payload.Meters(), convertCodes2Strings(codes), msg.Protocol)
	_ = db.SaveHistory(eeg.Id, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

// protocolEcReqOnlHandler executing the EC_REQ_ONL eda process (Online Meteringpoint registration). This process exist of the following steps
// - ANFORDERUNG_ECON: Initial request to open the process
// - ANTWORT_ECON: Request. The process was successfully started
// - ZUSTIMMUNG_ECON: Participant accepts the request in the user portal of the grid operator
// - ABSCHLUSS_ECON: Metering point is part of the EEG
// - ABLEHNUNG_ECON: The process was aborted
// - ABBRUCH_ECON: The membership was aborted by customer or grid operator
//
// During the process the Meteringpoint is taged with different status flags.
// - NEW: Meteringpoint is assosiated to a participant. The process is not started.
// - PENDING: ANFORDERUNG_ECON already sent and confirmed by partner with ANSWER_ECON message
// - APPROVED: ZUSTIMMUNG_ECON. Participant accept it in the grid operator portal
// - ACTIVE: ABSCHLUSS_ECON. Metering is activ
// - INVALID: Meteringpoint is rejected accourding to wrong attributes
// - REJECTED: Meteringpoint is rejected
// - ARCHIVED: Meteringpoint was archived by the participant
func protocolEcReqOnlHandler(ctx context.Context, msg model.SubscribeMessage) {
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
	var status model.ProcessStatusType
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
			if err := meteringPointPerformAnswerMsg(ctx, services.SendMail, msg.Payload.EcId, meters, "Dein Zählpunkt ist aktiv", "zp-complete-mail-template.toml"); err != nil {
				logrus.WithField("tenant", msg.Tenant).Errorf("complete mail message for %+v return with error. %v", meters, err.Error())
			}
		} else {
			status = ""
			meters = []string{}
		}

	case model.EBMS_ONLINE_REG_REJECTION, model.EBMS_OFFLINE_REG_REJECTION:
		if codesContains(REJECTED_IGNORE_CODES, codes) {
			status = model.RESTORE
			codes = []int16{0}
		} else if codesContains(REJECTED_VALID_CODES, codes) {
			status = model.ACTIVE
			activeSince = civil.NullDate{civil.Today(), true}
			codes = []int16{0}
			statusCode = &codes[0]
		} else if codesContains(REJECTED_INVALID_CODES, codes) {
			status = model.INVALID
			statusCode = &intersectCodes(REJECTED_INVALID_CODES, codes)[0]
		} else {
			status = model.REJECTED
			if len(codes) > 0 {
				statusCode = &codes[0]
			}
		}
	case model.EBMS_ONLINE_REG_APPROVAL, model.EBMS_OFFLINE_REG_APPROVAL:
		for i, c := range codes {
			switch c {
			case CONSENT_GRANTED:
				status = model.APPROVED
				consentId = &consentIds[i]
				break
			case CUSTOMER_HAS_REFUSED_DATA_RELEASE:
			case CUSTOMER_DID_NOT_RESPOND_TO_DATA_RELEASE:
				status = model.INVALID
				statusCode = &c
				break
			}
		}
	case model.EBMS_ONLINE_REG_ANSWER, model.EBMS_OFFLINE_REG_ANSWER:
		for _, c := range codes {
			if c == MESSAGE_RECEIVED {
				status = model.PENDING
				if err := meteringPointPerformAnswerMsg(ctx, services.SendMail, msg.Payload.EcId, meters, "Aktivierung im Serviceportal", "activation-mail-template.toml"); err != nil {
					logrus.WithField("tenant", msg.Tenant).Errorf("Perform Answer Message %+v. %v", meters, err.Error())
				}
			} else if c == NO_REMOTELY_READABLE_METER_INSTALLED_YET || c == SUM_OF_REPORTED_ALLOCATION_KEYS_EXCEEDS_100 {
				status = model.INVALID
				statusCode = &c
			}
		}
	case model.EBMS_ONLINE_REG_INIT, model.EBMS_OFFLINE_REG_INIT:
		meters = msg.Payload.Meters()
		codes = []int16{0}
		status = model.INIT

	case model.EBMS_ONLINE_REG_ABORT, model.EBMS_OFFLINE_REG_ABORT:
		meters = msg.Payload.Meters()
		//status = model.ABORTED
		status = model.REJECTED
	default:
		return
	}

	db, err := database.GetDB(ctx)
	if err != nil {
		logrus.WithField("tenant", msg.Tenant).Error(err)
		return
	}

	eeg, err := db.GetEegByEcId(ctx, msg.Payload.EcId)
	if err != nil {
		logrus.WithError(err).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
		return
	}

	if len(meters) > 0 && len(status) > 0 {
		switch status {
		case model.RESTORE:
			if err = db.RestoreMeteringPointProcessState(ctx, eeg.Id, meters[0]); err != nil {
				logrus.WithError(err).Errorf("can not restore MeteringPointProcessState")
			}
			break
		default:
			if err = db.MeteringPointsSetStatus(ctx, eeg.Id, status, statusCode, meters, activeSince.Ptr(), getConsentId(consentId)); err != nil {
				logrus.WithError(err).Errorf("can not change metering point status of meters: %+v", meters)
			}
			if err = db.SaveNotification(eeg.Id, msg.MessageCode, meters, convertCodes2Strings(codes), msg.Protocol); err != nil {
				logrus.WithError(err).Error("can not save notification")
			}
		}
	}
	_ = db.SaveHistory(eeg.Id, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

func protocolCmRevImpHandler(ctx context.Context, msg model.SubscribeMessage) {
	//var err error
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v Code: %s", msg.Protocol, msg.MessageCode)

	meters, _ := extractResponseCodeAndMeteringPointV2(&msg.Payload)

	db, err := database.GetDB(ctx)
	if err != nil {
		logrus.WithField("tenant", msg.Tenant).Error(err)
		return
	}

	var eeg *model.Eeg
	switch msg.MessageCode {
	case model.EBMS_AUFHEBUNG_CCMS:
		eeg, err = db.GetEegByEcId(ctx, msg.Payload.EcId)
		if err != nil {
			logrus.WithField("tenant", msg.Tenant).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
			return
		}

		if len(meters) == 0 {
			logrus.WithField("tenant", msg.Tenant).Errorf("no metering point in %s message -> %+v", msg.MessageCode, msg.Payload)
			return
		}

		if err := db.MeteringPointRevoke(ctx, eeg.Id, meters[0].meter, meters[0].consentEnd); err != nil {
			logrus.WithField("tenant", eeg.Id).Errorf("can not revoke metering point %+v - %+v", meters, err)
			return
		}

	case model.EBMS_ABLEHNUNG_CCMS:
		// The grid operator rejected the termination of the data-release consent
		// (Customer Consent Management), so the data release stays active and the
		// metering point must NOT be revoked. Only fetch the EEG so the rejection
		// is persisted as a notification (see below) and stays visible to the user.
		eeg, err = db.GetEegByEcId(ctx, msg.Payload.EcId)
		if err != nil {
			logrus.WithField("tenant", msg.Tenant).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
			return
		}

	case model.EBMS_AUFHEBUNG_CCMC, model.EBMS_AUFHEBUNG_CCMI:

		if len(meters) == 0 {
			logrus.WithField("tenant", msg.Tenant).Errorf("no metering point in %s message -> %+v", msg.MessageCode, msg.Payload)
			return
		}

		var tenant *string
		if tenant, err = db.MeteringPointRevokeByConsentId(ctx, meters[0].consentId, meters[0].meter, meters[0].consentEnd); err != nil {
			logrus.WithField("tenant", msg.Tenant).Errorf("can not revoke metering point %+v - %+v", meters, err)
			return
		}

		eeg, err = db.GetEegById(ctx, *tenant)
		if err != nil {
			logrus.WithField("tenant", *tenant).Errorf("can not fetch eeg by tenant %s (REVOKE metering point)", *tenant)
			return
		}

	case model.EBMS_ANTWORT_CCMS:
		if len(meters) > 0 {
			if codesContains([]int16{176}, meters[0].codes) {
				meters[0].consentEnd = civil.DateOf(time.UnixMilli(msg.Payload.ConsentEnd))
				eeg, err = db.GetEegByEcId(ctx, msg.Payload.EcId)
				if err != nil {
					logrus.WithField("tenant", msg.Tenant).Errorf("can not fetch eeg with message -> %+v", msg.Payload)
					return
				}

				if err := db.MeteringPointRevoke(ctx, eeg.Id, meters[0].meter, meters[0].consentEnd); err != nil {
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
			err = db.SaveNotification(eeg.Id, msg.MessageCode, []string{meters[0].meter}, convertCodes2Strings(meters[0].codes), msg.Protocol)
			if err != nil {
				logrus.WithError(err).Error("can not save notification")
			}
		}
		_ = db.SaveHistory(eeg.Id, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
	} else {
		logrus.WithField("tenant", msg.Tenant).Errorf("%+v", msg.Payload)
	}
}

func protocolEcPrtChangeHandler(ctx context.Context, msg model.SubscribeMessage) {
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

	db, err := database.GetDB(ctx)
	if err != nil {
		logrus.Error(err)
		return
	}

	eeg, err := db.GetEegByEcId(ctx, msg.Payload.EcId)
	if err != nil {
		logrus.Errorf("can not fetch eeg with message -> %+v", msg.Payload)
		return
	}

	if len(meters) > 0 && errCode == 0 {
		if err := db.MeteringPointChangePartFactor(ctx, eeg.Id, meters); err != nil {
			logrus.WithField("tenant", eeg.Id).Errorf("can not change partition factor. %v", err)
			return
		}
	}

	if errCode > 0 {
		_ = db.SaveNotification(eeg.Id, msg.MessageCode, getMeterIdSlice(meters), convertCodes2Strings([]int16{errCode}), msg.Protocol)
	}
	_ = db.SaveHistory(eeg.Id, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

func protocolEcPodListHandler(ctx context.Context, msg model.SubscribeMessage) {
	logrus.WithField("tenant", msg.Tenant).Printf("Handle Subscriptions: %+v Code: %s", msg.Protocol, msg.MessageCode)

	db, err := database.GetDB(ctx)
	if err != nil {
		logrus.Error(err)
		return
	}

	switch msg.MessageCode {
	case model.EBMS_ZP_LIST:
	case model.EBMS_ZP_LIST_RESPONSE:
		buf, err := database.ExportZPListToExcel(&msg.Payload)
		if err != nil {
			return
		}

		eeg, err := db.GetEegByEcId(ctx, msg.Payload.EcId)
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
				// Surface the failed ZP list mail to the tenant admins via
				// the existing notification system instead of log-only.
				_ = db.SaveNotificationFromMap(database.CreateNotificationMessageFromLog(
					&model.Log{Operation: "Mail", Messages: []*model.LogMessage{model.NewLogMessageFromVfeegError(
						"Zählpunktliste",
						err,
					)}}),
					msg.Payload.EcId, model.N_TYPE_ERROR, model.N_PROCESS_EDA_PROCESS, "ADMIN")
			}
		}
		if err = services.SyncMeteringPoints(msg.Tenant, &msg.Payload); err != nil {
			logrus.WithField("tenant", msg.Tenant).Error(err)
		}

	case model.EBMS_ZP_LIST_REJECTION:
	default:
		logrus.WithField("tenant", msg.Tenant).Warnf("Unknown Messagecode: %v", msg)
		return
	}
	_ = db.SaveHistory(msg.Tenant, msg.MessageCode, msg.Payload.ConversationId, "ADMIN", "IN", msg.Protocol, msg.Payload)
}

func getMeterIdSlice(meters []model.Meter) []string {
	ms := make([]string, len(meters))
	for _, m := range meters {
		ms = append(ms, m.MeteringPoint)
	}
	return ms
}

func meteringPointPerformAnswerMsg(ctx context.Context, sendMail services.SendMailFunc, ecId string, meterId []string, subject, templateConfigName string) error {

	db, err := database.GetDB(context.Background())
	if err != nil {
		return err
	}

	eeg, err := db.GetEegByEcId(ctx, ecId)
	if err != nil {
		return err
	}

	meterFilter := func(meters []*model.MeteringPoint, f func(string) bool) []*model.MeteringPoint {
		filtered := make([]*model.MeteringPoint, 0)
		for _, m := range meters {
			if f(m.MeteringPoint) {
				filtered = append(filtered, m)
			}
		}
		return filtered
	}

	for _, mid := range meterId {
		participant, err := db.FindParticipantByMeteringPoint(ctx, eeg.Id, mid)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return err
			} else {
				logrus.WithField("tenant", eeg.Id).Warn(err)
			}
		}

		if participant != nil && participant.Contact.Email.Valid {

			participant.MeteringPoint = meterFilter(participant.MeteringPoint, func(s string) bool {
				return s == mid
			})

			if len(participant.MeteringPoint) > 0 {
				if err = parser.SendActivationMailFromTemplate(sendMail,
					eeg.Id, subject, eeg, participant, templateConfigName); err != nil {
					logrus.WithField("tenant", eeg.Id).WithError(err).Error("Error Sending Mail")
					_ = db.SaveNotificationFromMap(database.CreateNotificationMessageFromLog(
						&model.Log{Operation: "Mail", Messages: []*model.LogMessage{model.NewLogMessageFromVfeegError(
							participant.MeteringPoint[0].MeteringPoint,
							err,
						)}}),
						ecId, model.N_TYPE_ERROR, model.N_PROCESS_EDA_PROCESS, "ADMIN")
				}
			} else {
				logrus.WithField("tenant", eeg.Id).Warn("No MeteringPoint for activation mail")
			}
		}
	}
	return nil
}

package mqttclient

import (
	"at.ourproject/vfeeg-backend/model"
	"errors"
	log "github.com/sirupsen/logrus"
	"strings"
)

func RegistrationForParticipation(tenant string, eeg *model.Eeg, meter *model.MeteringPoint) error {

	ebmsMessage := model.EbmsMessage{
		Sender:      strings.ToUpper(tenant),
		Receiver:    strings.ToUpper(getReceiverFrom(eeg, meter)),
		MessageCode: model.EBMS_ONLINE_REG_INIT,
		EcId:        eeg.CommunityId,
		Meter:       &model.Meter{MeteringPoint: meter.MeteringPoint, Direction: meter.Direction},
	}

	log.WithField("tenant", tenant).Infof("Start Meteringpoint %s registration", meter.MeteringPoint)
	if err := SendEbmsMessage(ebmsMessage); err != nil {
		return model.ErrEdaCommunication(err)
	}
	return nil
}

var RequestingEnergyData = func(tenant string, eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error {
	ebmsMessage := model.EbmsMessage{
		Sender:      strings.ToUpper(tenant),
		Receiver:    strings.ToUpper(getReceiverFrom(eeg, meter)),
		MessageCode: model.EBMS_ZP_SYNC,
		Meter:       &model.Meter{MeteringPoint: meter.MeteringPoint},
		Timeline: &model.Timeline{
			From: fromDate,
			To:   toDate,
		},
	}

	log.WithField("tenant", tenant).Info("Start Metering sync")
	if err := SendEbmsMessage(ebmsMessage); err != nil {
		return err
	}
	return nil
}

func RevokeMeteringPoint(tenant string, eeg *model.Eeg, meter *model.MeteringPoint, consentEnd int64, reason *string) error {

	var reasonMsg string
	if reason != nil {
		reasonMsg = *reason
	}

	ebmsMessage := model.EbmsMessage{
		Sender:   strings.ToUpper(tenant),
		Receiver: strings.ToUpper(getReceiverFrom(eeg, meter)),
		//Sender:      "sepp.gaug",
		//Receiver:    "obermueller.peter",
		MessageCode: model.EBMS_AUFHEBUNG_CCMS,
		Meter:       &model.Meter{MeteringPoint: meter.MeteringPoint},
		ConsentEnd:  consentEnd,
		Reason:      reasonMsg,
	}

	log.WithField("tenant", tenant).Info("Revoke Meteringpoint")
	if err := SendEbmsMessage(ebmsMessage); err != nil {
		return err
	}
	return nil
}

func RequestingMeteringPointList(tenant string, eeg *model.Eeg, from, to int64) error {

	if eeg.Area == model.BEG {
		return errors.New("process for BEG not available")
	}

	if eeg.GridOperator == "" {
		return errors.New("no Grid Operator known")
	}

	ebmsMessage := model.EbmsMessage{
		Sender:      strings.ToUpper(tenant),
		Receiver:    strings.ToUpper(eeg.GridOperator),
		MessageCode: model.EBMS_ZP_LIST,
		Meter:       &model.Meter{MeteringPoint: eeg.CommunityId},
		Timeline: &model.Timeline{
			From: from,
			To:   to,
		},
	}

	log.WithField("tenant", tenant).Info("Request MeteringPointList")
	if err := SendEbmsMessage(ebmsMessage); err != nil {
		return err
	}
	return nil
}

func getReceiverFrom(eeg *model.Eeg, meter *model.MeteringPoint) string {
	if eeg.Area == model.BEG {
		return meter.GridOperatorId.String
	}
	return eeg.GridOperator
}

//func getReceiver(eeg model.Eeg, meterId string) string {
//	if eeg.Area == model.BEG {
//		gridOperatorId, err := database.FindGridOperatorId(database.GetDBXConnection, meterId)
//		if err != nil {
//			log.WithField("tenant", eeg.Id).Errorf("Cannot find Grid Operator in Metering Point: %s", err.Error())
//			return eeg.GridOperator
//		}
//		return gridOperatorId
//	}
//	return eeg.GridOperator
//}

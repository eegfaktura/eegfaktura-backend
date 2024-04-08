package mqttclient

import (
	"at.ourproject/vfeeg-backend/model"
	"errors"
	log "github.com/sirupsen/logrus"
	"strings"
)

func RegistrationForParticipation(eeg *model.Eeg, meter *model.MeteringPoint) error {
	//ebmsMessage := model.EbmsMessage{
	//	Sender:   strings.ToUpper(tenant),
	//	Receiver: strings.ToUpper(getReceiverFrom(eeg, meter)),
	//	//Sender:      "sepp.gaug",
	//	//Receiver:    "obermueller.peter",
	//	MessageCode: model.EBMS_ONLINE_REG_INIT,
	//	EcId:        eeg.CommunityId,
	//	Meter:       &model.Meter{MeteringPoint: meter.MeteringPoint, Direction: meter.Direction, PartFact: meter.PartFact},
	//}
	ebmsMessage := createEbmsMessage(eeg, meter, model.EBMS_ONLINE_REG_INIT)
	ebmsMessage.Meter = &model.Meter{MeteringPoint: meter.MeteringPoint, Direction: meter.Direction, PartFact: meter.PartFact}

	log.WithField("tenant", eeg.Id).Infof("Start Meteringpoint %s registration", meter.MeteringPoint)
	if err := SendEbmsMessage(ebmsMessage); err != nil {
		return model.ErrEdaCommunication(err)
	}
	return nil
}

var RequestingEnergyData = func(eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error {
	//ebmsMessage := model.EbmsMessage{
	//	Sender:   strings.ToUpper(tenant),
	//	Receiver: strings.ToUpper(getReceiverFrom(eeg, meter)),
	//	//Sender:      "sepp.gaug",
	//	//Receiver:    "obermueller.peter",
	//	MessageCode: model.EBMS_ZP_SYNC,
	//	EcId:        eeg.CommunityId,
	//	Meter:       &model.Meter{MeteringPoint: meter.MeteringPoint},
	//	Timeline: &model.Timeline{
	//		From: fromDate,
	//		To:   toDate,
	//	},
	//}
	ebmsMessage := createEbmsMessage(eeg, meter, model.EBMS_ZP_SYNC)
	ebmsMessage.Meter = &model.Meter{MeteringPoint: meter.MeteringPoint}
	ebmsMessage.Timeline = &model.Timeline{From: fromDate, To: toDate}

	log.WithField("tenant", eeg.Id).Info("Start Metering sync")
	if err := SendEbmsMessage(ebmsMessage); err != nil {
		return err
	}
	return nil
}

func RevokeMeteringPoint(eeg *model.Eeg, meter *model.MeteringPoint, consentEnd int64, reason *string) error {

	var reasonMsg string
	if reason != nil {
		reasonMsg = *reason
	}

	//ebmsMessage := model.EbmsMessage{
	//	Sender:   strings.ToUpper(eeg.RcNumber),
	//	Receiver: strings.ToUpper(getReceiverFrom(eeg, meter)),
	//	//Sender:      "sepp.gaug",
	//	//Receiver:    "obermueller.peter",
	//	MessageCode: model.EBMS_AUFHEBUNG_CCMS,
	//	EcId:        eeg.CommunityId,
	//	Meter:       &model.Meter{MeteringPoint: meter.MeteringPoint},
	//	ConsentEnd:  consentEnd,
	//	Reason:      reasonMsg,
	//}
	ebmsMessage := createEbmsMessage(eeg, meter, model.EBMS_AUFHEBUNG_CCMS)
	ebmsMessage.Meter = &model.Meter{MeteringPoint: meter.MeteringPoint}
	ebmsMessage.ConsentEnd = consentEnd
	ebmsMessage.Reason = reasonMsg

	log.WithField("tenant", eeg.Id).Info("Revoke Meteringpoint")
	if err := SendEbmsMessage(ebmsMessage); err != nil {
		return err
	}
	return nil
}

func RequestingMeteringPointList(eeg *model.Eeg, from, to int64) error {

	if eeg.Area == model.BEG {
		return model.ErrEdaCommunication(errors.New("process for BEG not available"))
	}

	if eeg.GridOperator == "" {
		return model.ErrEdaCommunication(errors.New("no Grid Operator known"))
	}

	//ebmsMessage := model.EbmsMessage{
	//	Sender:      strings.ToUpper(eeg.RcNumber),
	//	Receiver:    strings.ToUpper(eeg.GridOperator),
	//	MessageCode: model.EBMS_ZP_LIST,
	//	Meter:       &model.Meter{MeteringPoint: eeg.CommunityId},
	//	EcId:        eeg.CommunityId,
	//	Timeline: &model.Timeline{
	//		From: from,
	//		To:   to,
	//	},
	//}

	ebmsMessage := createEbmsMessage(eeg, nil, model.EBMS_ZP_LIST)
	ebmsMessage.Meter = &model.Meter{MeteringPoint: eeg.CommunityId}
	ebmsMessage.Timeline = &model.Timeline{From: from, To: to}

	log.WithField("tenant", eeg.Id).Info("Request MeteringPointList")
	if err := SendEbmsMessage(ebmsMessage); err != nil {
		return model.ErrEdaCommunication(err)
	}
	return nil
}

func ChangePartitionFactor(eeg *model.Eeg, meter []*model.ChangePartitionFactorRequest) error {
	meterList := []model.Meter{}
	for _, m := range meter {
		meterList = append(meterList,
			model.Meter{
				MeteringPoint: m.MeteringPoint,
				Direction:     m.Direction,
				Activation:    m.Activation.UnixMilli(),
				PartFact:      m.PartFact,
			})
	}

	//ebmsMessage := model.EbmsMessage{
	//	Sender:   strings.ToUpper(eeg.RcNumber),
	//	Receiver: strings.ToUpper(eeg.GridOperator),
	//	//Sender:      "sepp.gaug",
	//	//Receiver:    "obermueller.peter",
	//	MessageCode: model.EBMS_REQ_CHANGE_PARTFACT,
	//	EcId:        eeg.CommunityId,
	//	EcType:      eeg.Area,
	//	EcDisModel:  model.AllocationModeType(eeg.AllocationMode),
	//	MeterList:   meterList,
	//}

	ebmsMessage := createEbmsMessage(eeg, nil, model.EBMS_REQ_CHANGE_PARTFACT)
	ebmsMessage.EcType = eeg.Area
	ebmsMessage.EcDisModel = model.AllocationModeType(eeg.AllocationMode)
	ebmsMessage.MeterList = meterList

	log.WithField("tenant", eeg.Id).Infof("Change Partition Factor. %+v", meterList)
	if err := SendEbmsMessage(ebmsMessage); err != nil {
		return model.ErrEdaCommunication(err)
	}
	return nil
}

func getReceiverFrom(eeg *model.Eeg, meter *model.MeteringPoint) string {
	receiver := eeg.GridOperator
	if meter != nil && eeg.Area == model.BEG {
		receiver = meter.GridOperatorId.String
	}
	return strings.ToUpper(receiver)
}

func createEbmsMessage(eeg *model.Eeg, meter *model.MeteringPoint, code model.EbMsMessageType) model.EbmsMessage {
	//sender := strings.ToUpper(eeg.RcNumber)
	receiver := getReceiverFrom(eeg, meter)

	return model.EbmsMessage{
		Sender:   strings.ToUpper(eeg.RcNumber),
		Receiver: receiver, //getReceiverFrom(eeg, meter),
		//Sender:      "sepp.gaug",
		//Receiver:    "obermueller.peter",
		MessageCode: code,
		EcId:        eeg.CommunityId,
	}
}

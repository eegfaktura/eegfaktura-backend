package mqttclient

import (
	"errors"
	"strings"

	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/util"
	log "github.com/sirupsen/logrus"
)

var RegistrationForParticipation = func(eeg *model.Eeg, meter *model.MeteringPoint, from *int64) error {

	ebmsMessage := createEbmsMessage(eeg, meter, model.EBMS_ONLINE_REG_INIT)
	ebmsMessage.Meter = &model.Meter{MeteringPoint: meter.MeteringPoint, Direction: meter.Direction, PartFact: meter.PartFact}

	log.WithField("tenant", eeg.Id).Infof("Start Meteringpoint %s ONLINE registration", meter.MeteringPoint)
	return sendRegistrationForParticipation(eeg, meter, from, ebmsMessage)
}

var OfflineRegistrationForParticipation = func(eeg *model.Eeg, meter *model.MeteringPoint, from *int64) error {
	ebmsMessage := createEbmsMessage(eeg, meter, model.EBMS_OFFLINE_REG_INIT)
	ebmsMessage.Meter = &model.Meter{MeteringPoint: meter.MeteringPoint, Direction: meter.Direction, PartFact: meter.PartFact, ConsentID: meter.ActivationCode}

	log.WithField("tenant", eeg.Id).Infof("Start Meteringpoint %s OFFLINE registration", meter.MeteringPoint)
	return sendRegistrationForParticipation(eeg, meter, from, ebmsMessage)
}

func sendRegistrationForParticipation(eeg *model.Eeg, meter *model.MeteringPoint, from *int64, ebmsMessage model.EbmsMessage) error {
	if from != nil {
		ebmsMessage.Meter.From = *from
	}

	if eeg.AllocationMode == model.STATIC && meter.AllocationFactor.Valid {
		ebmsMessage.Meter.Share = util.ToFixed(meter.AllocationFactor.Float64/100.0, 4)
	}

	//if err := SendEbmsMessage(ebmsMessage); err != nil {
	//	return model.ErrEdaCommunication(err)
	//}
	Broker().SendMessage(ebmsMessage)
	return nil
}

var RequestingEnergyData = func(eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error {

	ebmsMessage := createEbmsMessage(eeg, meter, model.EBMS_ZP_SYNC)
	ebmsMessage.Meter = &model.Meter{MeteringPoint: meter.MeteringPoint}
	ebmsMessage.Timeline = &model.Timeline{From: fromDate, To: toDate}

	log.WithField("tenant", eeg.Id).Info("Start Metering sync")
	//if err := SendEbmsMessage(ebmsMessage); err != nil {
	//	return err
	//}

	Broker().SendMessage(ebmsMessage)
	return nil
}

func RevokeMeteringPoint(eeg *model.Eeg, meter *model.MeteringPoint, consentEnd int64, reason *string) error {

	var reasonMsg string
	if reason != nil {
		reasonMsg = *reason
	}

	ebmsMessage := createEbmsMessage(eeg, meter, model.EBMS_AUFHEBUNG_CCMS)
	ebmsMessage.Meter = &model.Meter{MeteringPoint: meter.MeteringPoint, ConsentID: meter.ConsentId.ValueOrZero()}
	ebmsMessage.ConsentEnd = consentEnd
	ebmsMessage.Reason = reasonMsg

	log.WithField("tenant", eeg.Id).Info("Revoke Meteringpoint")
	//if err := SendEbmsMessage(ebmsMessage); err != nil {
	//	return err
	//}
	Broker().SendMessage(ebmsMessage)
	return nil
}

func RequestingMeteringPointList(eeg *model.Eeg, receiver string, from, to int64) error {

	ebmsMessage := createEbmsMessage(eeg, nil, model.EBMS_ZP_LIST)
	ebmsMessage.Meter = &model.Meter{MeteringPoint: eeg.CommunityId}
	ebmsMessage.Timeline = &model.Timeline{From: from, To: to}
	ebmsMessage.Receiver = receiver

	if eeg.Area != model.BEG {
		if eeg.GridOperator == "" {
			return model.ErrEdaCommunication(errors.New("no Grid Operator specified"))
		}
		ebmsMessage.Receiver = eeg.GridOperator
	}

	log.WithField("tenant", eeg.Id).Info("Request MeteringPointList")
	//if err := SendEbmsMessage(ebmsMessage); err != nil {
	//	return model.ErrEdaCommunication(err)
	//}
	Broker().SendMessage(ebmsMessage)
	return nil
}

var ChangePartitionFactor = func(eeg *model.Eeg, meterReq []*model.ChangePartitionFactorRequest) error {
	operators := map[string][]model.Meter{}
	var gridId string
	//meterList := []model.Meter{}
	for _, m := range meterReq {
		gridId = eeg.GridOperator
		if m.GridOperatorId.Valid && len(m.GridOperatorId.ValueOrZero()) > 0 {
			gridId = m.GridOperatorId.String
		}

		if _, ok := operators[gridId]; !ok {
			operators[gridId] = []model.Meter{}
		}

		operators[gridId] = append(operators[gridId],
			model.Meter{
				MeteringPoint: m.MeteringPoint,
				Direction:     m.Direction,
				Activation:    m.Activation.Unix() * 1000,
				PartFact:      m.PartFact,
			})
	}

	for k, v := range operators {
		ebmsMessage := createEbmsMessage(eeg, nil, model.EBMS_REQ_CHANGE_PARTFACT)
		ebmsMessage.Receiver = k
		ebmsMessage.EcType = eeg.Area
		ebmsMessage.EcDisModel = model.AllocationModeType(eeg.AllocationMode)
		ebmsMessage.MeterList = v

		log.WithField("tenant", eeg.Id).Infof("Change Partition Factor. %+v", v)
		Broker().SendMessage(ebmsMessage)
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
	receiver := getReceiverFrom(eeg, meter)

	return model.EbmsMessage{
		Sender:   strings.ToUpper(eeg.RcNumber),
		Receiver: receiver,
		//Sender:      strings.ToUpper("sepp.gaug"),
		//Receiver:    "obermueller.peter",
		MessageCode: code,
		EcId:        eeg.CommunityId,
	}
}

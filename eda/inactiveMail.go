package eda

import (
	"context"
	"strings"

	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/parser"
	"at.ourproject/vfeeg-backend/services"
	"github.com/jjeffery/civil"
	"github.com/sirupsen/logrus"
)

// Who ended the data release; one sentence of the inactive mail (platform#116).
const (
	inactiveReasonGridOperator = "Dein Netzbetreiber hat die Datenfreigabe für diesen Zählpunkt beendet, zum Beispiel wegen eines Zählertauschs oder Umzugs."
	inactiveReasonMember       = "Du hast die Datenfreigabe für diesen Zählpunkt beendet, zum Beispiel im Serviceportal deines Netzbetreibers."
	inactiveReasonCommunity    = "Deine Energiegemeinschaft hat diesen Zählpunkt abgemeldet."
)

// sendInactiveMail is a variable so tests can capture the mails.
var sendInactiveMail services.SendMailFunc = services.SendMail

// inactiveReasonFor returns the reason sentence for a revocation message, "" when it sends no mail.
func inactiveReasonFor(code model.EbMsMessageType) string {
	switch code {
	case model.EBMS_AUFHEBUNG_CCMI:
		return inactiveReasonGridOperator
	case model.EBMS_AUFHEBUNG_CCMC:
		return inactiveReasonMember
	case model.EBMS_ANTWORT_CCMS:
		return inactiveReasonCommunity
	default:
		return ""
	}
}

// sendMeteringPointInactiveMails tells the members of the given tenants that the metering point is
// no longer part of their community. It runs after the revocation is committed and never undoes
// it: a failed mail becomes an error notification for the community. One mail per member address
// and metering point, so a GEA with several tenants per RC number sends it once.
func sendMeteringPointInactiveMails(ctx context.Context, db database.Database, tenants []string, meterId string, consentEnd civil.Date, reason string) {
	sent := map[string]bool{}
	for _, tenant := range tenants {
		eeg, err := db.GetEegById(ctx, tenant)
		if err != nil {
			logrus.WithField("tenant", tenant).WithError(err).Error("inactive mail: can not fetch eeg")
			continue
		}
		participant, err := db.FindParticipantByMeteringPoint(ctx, tenant, meterId)
		if err != nil || participant == nil {
			logrus.WithField("tenant", tenant).Warnf("inactive mail: no participant for %s: %v", meterId, err)
			continue
		}
		key := strings.ToLower(strings.TrimSpace(participant.Contact.Email.String)) + "|" + meterId
		if participant.Contact.Email.Valid && sent[key] {
			continue
		}

		// the direction is read from the assigned metering point row itself
		direction := ""
		if m, err := db.FindAssignedMeteringById(ctx, tenant, meterId); err == nil && m != nil {
			direction = parser.DirectionText(m.Direction)
		}
		err = parser.SendMeteringPointInactiveMail(sendInactiveMail, tenant, parser.InactiveMailData{
			Eeg:           eeg,
			Participant:   participant,
			MeteringPoint: meterId,
			Direction:     direction,
			EndDate:       parser.FormatDate(consentEnd),
			Reason:        reason,
		})
		if err != nil {
			logrus.WithField("tenant", tenant).WithError(err).Error("inactive mail: error sending mail")
			_ = db.SaveNotificationFromMap(database.CreateNotificationMessageFromLog(
				&model.Log{Operation: "Mail", Messages: []*model.LogMessage{model.NewLogMessageFromVfeegError(meterId, err)}}),
				tenant, model.N_TYPE_ERROR, model.N_PROCESS_EDA_PROCESS, "ADMIN")
			continue
		}
		if participant.Contact.Email.Valid {
			sent[key] = true
		}
	}
}

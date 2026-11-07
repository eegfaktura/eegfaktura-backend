package eda

import (
	"context"
	"time"

	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"github.com/jjeffery/civil"
	"github.com/sirupsen/logrus"
)

// A deregistration by the community (AUFHEBUNG_CCMS, CM_REV_SP) changes the metering point only with the grid
// operator's answer (platform#238): sending it leaves the status as it is.

// revokeAfterAnswer sets the metering point of an answer to the deregistration INACTIVE when the answer ends the
// participation; meters[0].consentEnd becomes the inactive-since date. Only an ACTIVE metering point changes: a
// repeated answer, an earlier revocation (CCMI/CCMC) or a new participation keep their row and date. ok is false when
// the update fails; mail tells whether the member gets the "no longer part of the community" mail, only when the
// metering point changed.
func revokeAfterAnswer(ctx context.Context, db database.Database, tenant string, msg model.SubscribeMessage, meters []responseCodesPerMeter) (mail, ok bool) {
	if len(meters) == 0 || !revokeAnswerEndsParticipation(msg.MessageCode, meters[0].codes) {
		return false, true
	}
	meters[0].consentEnd = revokeInactiveSince(ctx, db, tenant, msg, meters[0].meter)
	changed, err := db.MeteringPointRevokeActive(ctx, tenant, meters[0].meter, meters[0].consentEnd)
	if err != nil {
		logrus.WithContext(ctx).WithField("tenant", tenant).WithError(err).Errorf("%s: can not revoke the metering point", msg.MessageCode)
		return false, false
	}
	conversation := msg.Payload.ConversationId
	entry := logrus.WithContext(ctx).WithField("tenant", tenant)
	if !changed {
		entry.WithField("event.action", "eda.revoke.unchanged").
			Infof("%s %v: metering point not active (or active only after %s), left as it is (conversation %s)",
				msg.MessageCode, meters[0].codes, meters[0].consentEnd, conversation)
		return false, true
	}
	entry.WithField("event.action", "eda.revoke.inactive").
		Infof("%s %v: metering point inactive since %s (conversation %s)", msg.MessageCode, meters[0].codes,
			meters[0].consentEnd, conversation)
	return revokeAnswerMailsMember(msg.MessageCode, meters[0].codes), true
}

// revokeAnswerEndsParticipation tells whether an answer to the community's deregistration sets the metering point
// INACTIVE: the confirmation (ANTWORT_CCMS 176), and a rejection because there is no data release (177) or the consent
// expired (180): the grid operator no longer delivers data for it. Any other answer, also a rejection with 82, 187,
// another or no code, leaves the status.
func revokeAnswerEndsParticipation(code model.EbMsMessageType, codes []int16) bool {
	switch code {
	case model.EBMS_ANTWORT_CCMS:
		return codesContains([]int16{Zustimmung_erfolgreich_entzogen}, codes)
	case model.EBMS_ABLEHNUNG_CCMS:
		return codesContains([]int16{Keine_Datenfreigabe_vorhanden, ConsentID_abgelaufen}, codes)
	default:
		return false
	}
}

// revokeAnswerMailsMember tells whether the member gets the "no longer part of the community" mail (platform#116):
// on the confirmation (176) only. Open question (platform#238): whether 177/180 send it too.
func revokeAnswerMailsMember(code model.EbMsMessageType, codes []int16) bool {
	return code == model.EBMS_ANTWORT_CCMS && codesContains([]int16{Zustimmung_erfolgreich_entzogen}, codes)
}

// revokeInactiveSince is the "inactive since" date when an answer ends the participation. Decision (a) of
// platform#238 (agreed by Harald in the issue): the consent end the community chose in its deregistration request,
// which the history of the tenant holds under the conversation id of the answer (eda-xp's echo of the sent request).
// Without that request the date the answer is received (b).
func revokeInactiveSince(ctx context.Context, db database.Database, tenant string, msg model.SubscribeMessage, meter string) civil.Date {
	conversation, answerEnd := msg.Payload.ConversationId, msg.Payload.ConsentEnd
	entry := logrus.WithContext(ctx).WithField("tenant", tenant)
	request, err := db.FindHistoryMessage(ctx, tenant, conversation, model.EBMS_AUFHEBUNG_CCMS)
	if err != nil {
		entry.WithError(err).Errorf("can not read the deregistration request from the history (conversation %s)", conversation)
	}
	if request == nil || request.ConsentEnd == 0 || request.Meter == nil || request.Meter.MeteringPoint != meter {
		entry.WithField("event.action", "eda.revoke.request_not_found").
			Warnf("%s: no deregistration request in the history, inactive since the date of the answer (conversation %s)",
				msg.MessageCode, conversation)
		return civil.Today()
	}
	requested := civil.DateOf(time.UnixMilli(request.ConsentEnd))
	// eda-xp copies the consent end of the request it stored into the answer: a different one is only logged
	if answerEnd != 0 {
		if answered := civil.DateOf(time.UnixMilli(answerEnd)); answered != requested {
			entry.WithField("event.action", "eda.revoke.date_mismatch").
				Warnf("%s carries %s, the request %s: the requested date is used (conversation %s)",
					msg.MessageCode, answered, requested, conversation)
		}
	}
	return requested
}

package parser

import (
	"fmt"
	"strings"

	"at.ourproject/vfeeg-backend/config"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/services"
	"github.com/jjeffery/civil"
	log "github.com/sirupsen/logrus"
)

// InactiveMailTemplate is the template config of the mail that tells a member that a metering point
// is no longer part of the community (platform#116).
const InactiveMailTemplate = "zp-inactive-mail-template.toml"

// InactiveMailSubject is the subject of that mail.
const InactiveMailSubject = "Dein Zählpunkt ist nicht mehr Teil der Energiegemeinschaft"

// InactiveMailData is what the inactive mail template renders.
type InactiveMailData struct {
	Eeg           *model.Eeg
	Participant   *model.EegParticipant
	MeteringPoint string
	Direction     string // "Bezug" / "Einspeisung", empty when unknown
	EndDate       string // TT.MM.JJJJ
	Reason        string // one sentence: who ended the data release
}

// FormatDate renders a date as TT.MM.JJJJ; "" for a missing date (zero value or the Unix epoch,
// which is what a missing consentEnd in an EDA message turns into).
func FormatDate(d civil.Date) string {
	if d.Year() <= 1970 {
		return ""
	}
	return fmt.Sprintf("%02d.%02d.%04d", d.Day(), int(d.Month()), d.Year())
}

// DirectionText renders the direction of a metering point for members.
func DirectionText(d model.DirectionType) string {
	switch d {
	case model.CONSUMPTION:
		return "Bezug"
	case model.GENERATOR:
		return "Einspeisung"
	default:
		return ""
	}
}

// SendMeteringPointInactiveMail renders the inactive mail and sends it to the member, with the
// community in Cc. A member without e-mail address gets no mail.
func SendMeteringPointInactiveMail(sendMail services.SendMailFunc, tenant string, data InactiveMailData) error {
	if !data.Participant.Contact.Email.Valid || strings.TrimSpace(data.Participant.Contact.Email.String) == "" {
		log.WithField("tenant", tenant).Warnf("Participant without email contact: %s (%s)", data.Participant.LastName, data.Participant.Id)
		return nil
	}

	tmplFS, source := resolveTemplateSource(tenant, InactiveMailTemplate)
	templateConfig, err := config.ReadActivationMailTemplateConfig(tmplFS, InactiveMailTemplate)
	if err != nil {
		return err
	}
	log.Infof("Mail template %q for tenant %q resolved from %s", InactiveMailTemplate, tenant, source)

	buf, err := ParseTemplate(tmplFS, templateConfig.TemplateFile, data)
	if err != nil {
		return err
	}
	return sendMail(tenant, data.Participant.Contact.Email.String,
		InactiveMailSubject, data.Eeg.Email.Ptr(), buf,
		buildInlineContent(tmplFS, templateConfig.InlinePictures),
		buildAttachment(tmplFS, templateConfig.Attachment.Name, templateConfig.Attachment.Mime),
	)
}

package parser

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"at.ourproject/vfeeg-backend/config"
	"at.ourproject/vfeeg-backend/mailtemplate"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/public"
	"at.ourproject/vfeeg-backend/services"
	"github.com/gabriel-vasile/mimetype"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"gopkg.in/guregu/null.v4"
)

// ParseTemplate renders the HTML template named name from fsys with data.
func ParseTemplate(fsys fs.FS, name string, data interface{}) (*bytes.Buffer, error) {

	t, err := template.ParseFS(fsys, name)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	if err = t.Execute(buf, data); err != nil {
		return nil, err
	}
	return buf, nil
}

// resolveTemplateSource decides where a mail template and its inline assets are
// read from. Operator overrides on the data volume win — a per-tenant templates
// dir first, then the global templates dir — so a mail can still be customised
// by dropping files on the PVC. When neither holds the requested config file,
// the defaults embedded in the binary (public/templates) are used, so a fresh
// deployment renders the mail without any template being seeded onto the volume.
func resolveTemplateSource(tenant, templateConfigName string) (fs.FS, string) {
	base := viper.GetString("file-content.templates")
	for _, dir := range []string{
		filepath.Join(base, tenant, "templates"),
		filepath.Join(base, "templates"),
	} {
		if _, err := os.Stat(filepath.Join(dir, templateConfigName)); err == nil {
			return os.DirFS(dir), dir
		}
	}
	embedded, err := fs.Sub(public.Templates, "templates")
	if err != nil {
		// A fixed sub-path of an embed.FS never fails; fall back defensively.
		return public.Templates, "embedded"
	}
	return embedded, "embedded"
}

// SendActivationMailFromTemplate sends the consent-request or the zp-active mail (by template
// config name). store holds the community's customised mails; nil sends the legacy template.
// TenantTemplateActive reports whether a hand-made template of the community on
// the data volume applies to a mail (used while it has no saved version).
func TenantTemplateActive(tenant, templateConfigName string) bool {
	_, err := os.Stat(filepath.Join(viper.GetString("file-content.templates"), tenant, "templates", templateConfigName))
	return err == nil
}

// RenderLegacyPreview renders the template the legacy resolution would send
// (tenant dir, global dir, embedded) with the sample member, images as data: URIs.
func RenderLegacyPreview(tenant, templateConfigName string, eeg *model.Eeg, data *mailtemplate.Data) (string, error) {
	tmplFS, _ := resolveTemplateSource(tenant, templateConfigName)
	cfg, err := config.ReadActivationMailTemplateConfig(tmplFS, templateConfigName)
	if err != nil {
		return "", err
	}
	participant := &model.EegParticipant{EegParticipantBase: model.EegParticipantBase{
		FirstName: data.FirstName, LastName: data.LastName, ParticipantNumber: null.StringFrom(data.Number)}}
	templateData := struct {
		Eeg            *model.Eeg
		Participant    *model.EegParticipant
		Meteringpoints []string
		MeteringPoint  string
		Direction      string
		EndDate        string
		Reason         string
	}{eeg, participant, data.MeteringPoints, strings.Join(data.MeteringPoints, ", "), data.Direction, data.EndDate, data.Reason}
	buf, err := ParseTemplate(tmplFS, cfg.TemplateFile, templateData)
	if err != nil {
		return "", err
	}
	out := buf.String()
	for _, p := range cfg.InlinePictures {
		content, err := fs.ReadFile(tmplFS, p.Filepath)
		if err != nil {
			continue
		}
		out = strings.ReplaceAll(out, "cid:"+p.ContentId,
			"data:"+mimetype.Detect(content).String()+";base64,"+base64.StdEncoding.EncodeToString(content))
	}
	return out, nil
}

func SendActivationMailFromTemplate(sendMail services.SendMailFunc, store mailtemplate.Store,
	tenant, subject string, eeg *model.Eeg, participant *model.EegParticipant, templateConfigName string) error {

	meterIds := []string{}
	gridOperator := ""
	for i := range participant.MeteringPoint {
		meterIds = append(meterIds, participant.MeteringPoint[i].MeteringPoint)
		if gridOperator == "" {
			gridOperator = participant.MeteringPoint[i].GridOperatorName.String
		}
	}
	templateData := struct {
		Eeg            *model.Eeg
		Participant    *model.EegParticipant
		Meteringpoints []string
		MeteringPoint  string
	}{eeg, participant, meterIds, strings.Join(meterIds, ", ")}

	mailData := memberMailData(eeg, participant)
	mailData.MeteringPoints = meterIds
	mailData.GridOperator = gridOperator
	return sendTemplateMail(sendMail, store, tenant, subject, templateConfigName, eeg, participant, templateData, mailData)
}

// memberMailData returns the placeholder values of a member mail.
func memberMailData(eeg *model.Eeg, participant *model.EegParticipant) *mailtemplate.Data {
	return &mailtemplate.Data{
		FirstName: participant.FirstName,
		LastName:  participant.LastName,
		Number:    participant.ParticipantNumber.String,
		Eeg:       eeg,
	}
}

// sendTemplateMail sends a member mail to the participant with the community in Cc: the
// community's customised version when it has one (store), else the legacy template config
// (tenant dir, global dir, embedded default). A participant without an e-mail address is
// skipped. Shared by the activation and the inactive mail.
func sendTemplateMail(sendMail services.SendMailFunc, store mailtemplate.Store, tenant, subject, templateConfigName string,
	eeg *model.Eeg, participant *model.EegParticipant, data interface{}, mailData *mailtemplate.Data) error {

	if store != nil && participant.Contact.Email.Valid && strings.TrimSpace(participant.Contact.Email.String) != "" {
		handled, err := mailtemplate.SendStored(context.Background(), store, sendMail,
			mailtemplate.LookupByLegacyConfig(templateConfigName), tenant, participant.Contact.Email.String,
			eeg.Email.Ptr(), mailData, strings.Join(mailData.MeteringPoints, ", "))
		if handled {
			return err
		}
	}

	tmplFS, source := resolveTemplateSource(tenant, templateConfigName)
	templateConfig, err := config.ReadActivationMailTemplateConfig(tmplFS, templateConfigName)
	if err != nil {
		return err
	}
	log.Infof("Mail template %q for tenant %q resolved from %s", templateConfigName, tenant, source)

	if !participant.Contact.Email.Valid || strings.TrimSpace(participant.Contact.Email.String) == "" {
		log.Warnf("Participant without email contact: %s (%s)", participant.LastName, participant.Id)
		return nil
	}

	buf, err := ParseTemplate(tmplFS, templateConfig.TemplateFile, data)
	if err != nil {
		return err
	}

	return sendMail(tenant, participant.Contact.Email.String,
		subject, eeg.Email.Ptr(), buf,
		buildInlineContent(tmplFS, templateConfig.InlinePictures),
		buildAttachment(tmplFS, templateConfig.Attachment.Name, templateConfig.Attachment.Mime),
	)
}

func GetTemplateFor(templateType, tenant string) (string, error) {

	path := filepath.Join(viper.GetString("file-content.templates"), tenant, "templates")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join("../public/templates")
	}

	switch templateType {
	case "ACTIVATION":
		return filepath.Join(path, "AktivierungsEmail-templates.html"), nil
	}
	return "", errors.New("Template not found")
}

func buildInlineContent(tmplFS fs.FS, a []model.InlinePicture) []*services.Attachment {
	attachments := []*services.Attachment{}
	for i := range a {
		att := a[i]
		data, err := fs.ReadFile(tmplFS, att.Filepath)
		if err != nil {
			log.Errorf("Read Attachment. Reason: %+v", err)
			continue
		}
		mime := mimetype.Detect(data)
		attachments = append(attachments, &services.Attachment{
			Type:        "INLINE",
			Filename:    filepath.Base(att.Filepath),
			Filecontent: bytes.NewBuffer(data),
			MimeType:    mime.String(),
			ContentId:   &att.ContentId,
		})
	}
	return attachments
}

func buildAttachment(tmplFS fs.FS, fileName string, mime string) *services.Attachment {
	if len(fileName) == 0 {
		return nil
	}

	buff, err := fs.ReadFile(tmplFS, fileName) // read the content of file
	if err != nil {
		log.Error(err)
		return nil
	}

	return &services.Attachment{
		Type:        "DEFAULT",
		Filename:    fileName,
		Filecontent: bytes.NewBuffer(buff),
		MimeType:    mime,
		ContentId:   nil,
	}
}

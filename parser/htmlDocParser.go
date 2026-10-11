package parser

import (
	"bytes"
	"errors"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"at.ourproject/vfeeg-backend/config"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/public"
	"at.ourproject/vfeeg-backend/services"
	"github.com/gabriel-vasile/mimetype"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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

func SendActivationMailFromTemplate(sendMail services.SendMailFunc,
	tenant, subject string, eeg *model.Eeg, participant *model.EegParticipant, templateConfigName string) error {

	meterIds := []string{}
	for i := range participant.MeteringPoint {
		meterIds = append(meterIds, participant.MeteringPoint[i].MeteringPoint)
	}
	templateData := struct {
		Eeg            *model.Eeg
		Participant    *model.EegParticipant
		Meteringpoints []string
		MeteringPoint  string
	}{eeg, participant, meterIds, strings.Join(meterIds, ", ")}

	return sendTemplateMail(sendMail, tenant, subject, templateConfigName, eeg, participant, templateData)
}

// sendTemplateMail renders a member mail from a template config (tenant dir, global dir, embedded
// default) and sends it to the participant with the community in Cc. A participant without an
// e-mail address is skipped. Shared by the activation and the inactive mail.
func sendTemplateMail(sendMail services.SendMailFunc, tenant, subject, templateConfigName string,
	eeg *model.Eeg, participant *model.EegParticipant, data interface{}) error {

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

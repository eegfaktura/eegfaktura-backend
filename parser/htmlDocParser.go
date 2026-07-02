package parser

import (
	"bytes"
	"embed"
	"errors"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"at.ourproject/vfeeg-backend/config"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/services"
	"github.com/gabriel-vasile/mimetype"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

//go:embed templates
var templates embed.FS

func ParseTemplate(templateFileName string, data interface{}) (*bytes.Buffer, error) {

	t, err := template.ParseFiles(templateFileName)
	if err != nil {
		return nil, err
	}
	buf := new(bytes.Buffer)
	if err = t.Execute(buf, data); err != nil {
		return nil, err
	}
	return buf, nil
}

func SendActivationMailFromTemplate(sendMail services.SendMailFunc,
	tenant, subject string, eeg *model.Eeg, participant *model.EegParticipant, templateConfigName string) error {

	templateConfigDir := filepath.Join(viper.GetString("file-content.templates"), tenant, "templates")
	// Fall back to the global templates dir when the tenant has no template dir at all,
	// or is missing this specific template file (e.g. no per-tenant zp-complete-mail-template).
	if _, err := os.Stat(filepath.Join(templateConfigDir, templateConfigName)); errors.Is(err, os.ErrNotExist) {
		templateConfigDir = filepath.Join(viper.GetString("file-content.templates"), "templates")
	}

	//templateConfig, err := config.ReadActivationMailTemplateConfig(filepath.Join(templateConfigDir, "activation-mail-template.toml"))
	templateConfig, err := config.ReadActivationMailTemplateConfig(filepath.Join(templateConfigDir, templateConfigName))
	if err != nil {
		return err
	}

	return sendMailFromTemplate(sendMail, tenant, subject, templateConfigDir, templateConfig, eeg, participant)
}

func sendMailFromTemplate(sendMail services.SendMailFunc, tenant, subject, templatePath string, templateConfig *model.ActivationMailTemplate, eeg *model.Eeg, participant *model.EegParticipant) error {
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

	if !participant.Contact.Email.Valid {
		log.Warnf("Participant without email contact: %s (%s)", participant.LastName, participant.Id)
		return nil
	}

	tmpPath := filepath.Join(templatePath, templateConfig.TemplateFile)
	buf, err := ParseTemplate(tmpPath, templateData)
	if err != nil {
		return err
	}

	return sendMail(tenant, participant.Contact.Email.String,
		subject, eeg.Email.Ptr(), buf,
		buildInlineContent(templatePath, templateConfig.InlinePictures),
		buildAttachment(templatePath, templateConfig.Attachment.Name, templateConfig.Attachment.Mime),
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

func buildInlineContent(templatePath string, a []model.InlinePicture) []*services.Attachment {
	attachments := []*services.Attachment{}
	for i := range a {
		att := a[i]
		data, err := os.ReadFile(filepath.Join(templatePath, att.Filepath))
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

func buildAttachment(templatePath string, fileName string, mime string) *services.Attachment {
	var err error
	var buff []byte
	if len(fileName) == 0 {
		return nil
	}

	buff, err = os.ReadFile(filepath.Join(templatePath, fileName)) // read the content of file
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

package parser

import (
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/util"
	"bytes"
	"errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"html/template"
	"os"
	"path/filepath"
)

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

func SendActivationMailFromTemplate(sendMail util.SendMailFunc,
	tenant, templateFileName, subject string, eeg *model.Eeg, participant *model.EegParticipant) error {

	templateData := struct {
		Eeg         *model.Eeg
		Participant *model.EegParticipant
	}{eeg, participant}

	if !participant.Contact.Email.Valid {
		log.Warnf("Participant without email contact: %s (%s)", participant.LastName, participant.Id)
		return nil
	}

	buf, err := ParseTemplate(templateFileName, templateData)
	if err != nil {
		return err
	}
	//to := participant.Contact.Email
	return sendMail(tenant, participant.Contact.Email.String, subject, buf, nil, nil)
}

func GetTemplateFor(templateType, tenant string) (string, error) {

	path := filepath.Join(viper.GetString("file-content.templates"), tenant, "templates")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = filepath.Join("../public/templates")
	}

	switch templateType {
	case "ACTIVATION":
		return filepath.Join(path, "AktivierungsEmail-template.html"), nil
	}
	return "", errors.New("Template not found")
}

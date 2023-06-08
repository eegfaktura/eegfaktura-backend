package parser

import (
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/util"
	"bytes"
	log "github.com/sirupsen/logrus"
	"html/template"
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

func SendMailFromTemplate(tenant, templateFileName, subject string, participant *model.EegParticipant) error {

	if !participant.Contact.Email.Valid {
		log.Warnf("Participant without email contact: %s (%s)", participant.LastName, participant.Id)
		return nil
	}

	buf, err := ParseTemplate(templateFileName, participant)
	if err != nil {
		return err
	}
	//to := participant.Contact.Email
	return util.SendMail(tenant, participant.Contact.Email.String, subject, buf, nil, nil)
}

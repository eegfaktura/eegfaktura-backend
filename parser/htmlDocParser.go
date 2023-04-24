package parser

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/util"
	"bytes"
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

func SendMailFromTemplate(tenant, participantId, templateFileName, subject, to string) error {

	participant, err := database.QueryParticipant(participantId)
	if err != nil {
		return err
	}

	buf, err := ParseTemplate(templateFileName, participant)
	if err != nil {
		return err
	}
	//to := participant.Contact.Email
	return util.SendMail(tenant, to, subject, buf, nil, nil)
}

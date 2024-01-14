package parser

import (
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/services"
	"bytes"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"gopkg.in/guregu/null.v4"
	"reflect"
	"strings"
	"testing"
	"time"
)

func init() {
	viper.Set("services.mail-server", "localhost:9092")
	viper.Set("file-content.templates", "../public")
}

func trimString(s string) string {
	s = strings.Replace(s, " ", "", -1)
	s = strings.Replace(s, "\t", "", -1)
	s = strings.Replace(s, "\n", "", -1)
	return s
}

func TestGetTemplateFor(t *testing.T) {
	type args struct {
		templateType string
		tenant       string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"Hugo",
			args{"ACTIVATION", "RC100181"},
			"../public/RC100181/templates/AktivierungsEmail-templates.html",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetTemplateFor(tt.args.templateType, tt.args.tenant)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetTemplateFor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetTemplateFor() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTemplate(t *testing.T) {

	eeg := &model.Eeg{
		Id:                 "",
		Name:               "TE-EEG",
		Description:        "TEST EEG",
		BusinessNr:         null.String{},
		Area:               "",
		Legal:              "",
		OperatorName:       "",
		CommunityId:        "",
		GridOperator:       "",
		RcNumber:           "",
		AllocationMode:     "",
		SettlementInterval: "",
		ProviderBusinessNr: null.Int{},
		TaxNumber:          null.String{},
		VatNumber:          null.String{},
		ContactPerson:      null.StringFrom("Max Sonnenmann"),
		EegAddress:         model.EegAddress{},
		AccountInfo:        model.AccountInfo{},
		Contact: model.Contact{
			Phone: null.StringFrom("123456789"),
		},
		Optionals: model.Optionals{},
		Periods:   nil,
		Online:    false,
	}

	participant := &model.EegParticipant{
		Id:                    nil,
		ParticipantNumber:     null.String{},
		BusinessRole:          "",
		FirstName:             "Max",
		LastName:              "Mustermann",
		TitleBefore:           "",
		TitleAfter:            "",
		ParticipantSince:      time.Time{},
		VatNumber:             "",
		TaxNumber:             "",
		CompanyRegisterNumber: "",
		Contact: model.ContactInfo{
			Phone: null.String{},
			Email: null.StringFrom("my@mail.com"),
		},
		BillingAddress:  model.Address{},
		ResidentAddress: model.Address{},
		BankAccount:     model.BankInfo{},
		MeteringPoint:   nil,
		TariffId:        null.String{},
		Status:          "",
		Version:         0,
	}

	type args struct {
		templateFileName string
		data             interface{}
	}
	tests := []struct {
		name    string
		args    args
		want    *bytes.Buffer
		wantErr bool
	}{
		{
			"Parse ACTIVATION Template",
			args{"../public/templates/AktivierungsEmail-template.html", struct {
				Eeg            *model.Eeg
				Participant    *model.EegParticipant
				Meteringpoints []string
			}{eeg, participant, []string{"AT0010000000000000000000000111"}}},
			bytes.NewBufferString(`<!DOCTYPE html>
        <html lang="en">
        <head>
            <meta charset="UTF-8">
            <title>Aktivierung Zählpunkt</title>
        </head>
        <body>
        <p>Hallo Max,</p>
        <p>damit deine Registrierung abgeschlossen werden kann,
            benötigen wir die Freigabe deiner Zählpunkte
            <ul> <li>AT0010000000000000000000000111</li> </ul>.
            Auf der Webseite deines Netzbetreibers kann diese Freigabe online erteilt werden.</p>
        <br>
        
        <p>Mit besten Grüßen</p>
        <p>deine VFEEG Team. Im Auftrag von, </p>
        <p>
        
        <div>{{Max Sonnenmann true}}</div>
        
        
        <div>123456789</div>
        
        </p>
        <div>Erneuerbare Energie Gemeinschaft:</div>
        <div>TE-EEG</div>
        <div>TEST EEG</div>
        
        <p>Powered by eegFaktura.at</p>
        <img src="cid:eegfaktura-logo-1" style="max-height: 90px"/>
        </body>
        </html>`),
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTemplate(tt.args.templateFileName, tt.args.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTemplate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(bytes.NewBufferString(trimString(got.String())), bytes.NewBufferString(trimString(tt.want.String()))) {
				t.Errorf("ParseTemplate() got = %v, want %v", got.String(), tt.want)
			}
		})
	}
}

func TestParseTemplate2(t *testing.T) {
	eeg := &model.Eeg{
		Id:                 "",
		Name:               "TE-EEG",
		Description:        "TEST EEG",
		BusinessNr:         null.String{},
		Area:               "",
		Legal:              "",
		OperatorName:       "",
		CommunityId:        "",
		GridOperator:       "",
		RcNumber:           "",
		AllocationMode:     "",
		SettlementInterval: "",
		ProviderBusinessNr: null.Int{},
		TaxNumber:          null.String{},
		VatNumber:          null.String{},
		ContactPerson:      null.StringFrom("Max Sonnenmann"),
		EegAddress:         model.EegAddress{},
		AccountInfo:        model.AccountInfo{},
		Contact: model.Contact{
			Phone: null.StringFrom("123456789"),
		},
		Optionals: model.Optionals{},
		Periods:   nil,
		Online:    false,
	}

	participant := &model.EegParticipant{
		Id:                    nil,
		ParticipantNumber:     null.String{},
		BusinessRole:          "",
		FirstName:             "Max",
		LastName:              "Mustermann",
		TitleBefore:           "",
		TitleAfter:            "",
		ParticipantSince:      time.Time{},
		VatNumber:             "",
		TaxNumber:             "",
		CompanyRegisterNumber: "",
		Contact: model.ContactInfo{
			Phone: null.String{},
			Email: null.StringFrom("my@mail.com"),
		},
		BillingAddress:  model.Address{},
		ResidentAddress: model.Address{},
		BankAccount:     model.BankInfo{},
		MeteringPoint:   nil,
		TariffId:        null.String{},
		Status:          "",
		Version:         0,
	}

	sendMock := func(tenant, to, subject string, body *bytes.Buffer, attachments []*services.Attachment) error {
		println("SendMock")
		return nil
	}

	err := SendActivationMailFromTemplate(sendMock, "sepp", "test", eeg, participant)
	assert.NoError(t, err)

}

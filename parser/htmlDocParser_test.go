package parser

import (
	"at.ourproject/vfeeg-backend/model"
	"bytes"
	"gopkg.in/guregu/null.v4"
	"reflect"
	"testing"
	"time"
)

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
			"../public/templates/AktivierungsEmail-template.html",
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
		BusinessNr:         null.Int{},
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
		ContactPerson:      "Max Sonnenmann",
		Address:            model.Address{},
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
				Eeg         *model.Eeg
				Participant *model.EegParticipant
			}{eeg, participant}},
			bytes.NewBufferString(""),
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
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseTemplate() got = %v, want %v", got, tt.want)
			}
		})
	}
}

//
//func TestSendActivationMailFromTemplate(t *testing.T) {
//	type args struct {
//		tenant           string
//		templateFileName string
//		subject          string
//		eeg              *model.Eeg
//		participant      *model.EegParticipant
//	}
//	tests := []struct {
//		name    string
//		args    args
//		wantErr bool
//	}{
//		// TODO: Add test cases.
//	}
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			if err := SendActivationMailFromTemplate(tt.args.tenant, tt.args.templateFileName, tt.args.subject, tt.args.eeg, tt.args.participant); (err != nil) != tt.wantErr {
//				t.Errorf("SendActivationMailFromTemplate() error = %v, wantErr %v", err, tt.wantErr)
//			}
//		})
//	}
//}

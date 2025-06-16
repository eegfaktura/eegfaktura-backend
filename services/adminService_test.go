package services

import (
	protobuf "at.ourproject/vfeeg-backend/proto"
	"context"
	"testing"
)

//func init() {
//	viper.Set("services.mail-server", "localhost:9092")
//}
//
//func TestSendMail(t *testing.T) {
//	var b bytes.Buffer
//	b.WriteString("Hallo")
//	b.WriteString("Jürgen")
//
//	err := SendMail("tenant", "obermueller.peter@gmail.com", "Ihre Anmeldung", &b, nil, nil)
//	require.NoError(t, err)
//}

func TestSendMail(t *testing.T) {

}

func TestRegisterEeg(t *testing.T) {
	service := &RegisterService{}
	eeg := &protobuf.RegisterEegRequest{
		RcNumber:           "TE100111",
		CommunityId:        "RC345124312545124312341234",
		Name:               "",
		Description:        "",
		Iban:               "",
		Owner:              "",
		Sepa:               false,
		Legal:              0,
		BusinessNr:         "",
		TaxNumber:          "",
		VatNumber:          "",
		SettelmentInterval: 0,
		GridId:             "",
		GridName:           "",
		Area:               0,
		Allocation:         0,
		EegOwner:           "",
		Street:             "",
		StreetNumber:       "",
		City:               "",
		Zip:                "",
		Email:              "",
		Web:                nil,
		Phone:              nil,
		Online:             false,
	}

	service.Register(context.Background(), eeg)
}

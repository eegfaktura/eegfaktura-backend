package services

import (
	"at.ourproject/vfeeg-backend/database"
	protobuf "at.ourproject/vfeeg-backend/proto"
	"context"
	"errors"
	"fmt"
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

// The contract with admin-backend (RegisterService.scala): 409 without a gRPC error stops the registration (261015-ca1).
func TestRegisterReply(t *testing.T) {
	if r, err := registerReply(nil); err != nil || r.Status != 201 {
		t.Errorf("registered: got status %d, err %v", r.GetStatus(), err)
	}
	if r, err := registerReply(fmt.Errorf("insert: %w", database.ErrEegExists)); err != nil || r.Status != 409 {
		t.Errorf("existing tenant: got status %d, err %v", r.GetStatus(), err)
	}
	if r, err := registerReply(errors.New("db down")); err == nil || r.Status != 500 {
		t.Errorf("database error: got status %d, err %v", r.GetStatus(), err)
	}
}

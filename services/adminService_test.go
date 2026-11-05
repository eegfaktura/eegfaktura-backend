package services

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	protobuf "at.ourproject/vfeeg-backend/proto"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// The contract with admin-backend (register.proto GRID_AREA, platform#229): every enum value is a known community
// type under its own name (P2P = 4, EVA = 5); a value this backend does not know is refused before any write.
func TestRegisterEegArea(t *testing.T) {
	assert.Len(t, protobuf.RegisterEegRequest_GRID_AREA_name, len(model.AreaTypes))
	for _, area := range model.AreaTypes {
		v, ok := protobuf.RegisterEegRequest_GRID_AREA_value[string(area)]
		require.True(t, ok, area)
		assert.NoError(t, model.ValidateArea(model.AreaType(protobuf.RegisterEegRequest_GRID_AREA(v).String())), area)
	}
	assert.Equal(t, int32(4), protobuf.RegisterEegRequest_GRID_AREA_value["P2P"])
	assert.Equal(t, int32(5), protobuf.RegisterEegRequest_GRID_AREA_value["EVA"])

	r, err := (&RegisterService{}).Register(context.Background(),
		&protobuf.RegisterEegRequest{Tenant: "TE000045", RcNumber: "TE000045", Area: 6})
	assert.Equal(t, uint32(400), r.GetStatus())
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
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

package util

import (
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	protobuf "at.ourproject/vfeeg-backend/proto"
	"bytes"
	"context"
	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gopkg.in/guregu/null.v4"
	"net"
	"time"
)

func SendMail(tenant, to, subject string, body *bytes.Buffer, fileName *string, fileContent *bytes.Buffer) error {
	fmt.Printf("GRPC SERVER: %v\n", viper.GetString("services.mail-server"))
	conn, err := grpc.Dial(viper.GetString("services.mail-server"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	c := protobuf.NewExcelAdminServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	request := &protobuf.SendExcelRequest{
		Tenant:    tenant,
		Recipient: to,
		Subject:   subject,
	}
	if body != nil {
		request.Body = body.Bytes()
	}

	if fileName != nil && fileContent != nil {
		request.Content = fileContent.Bytes()
		request.Filename = fileName
	}

	r, err := c.SendExcel(ctx, request)
	log.Infof("Response from MAIL-SERVER: %v", r)
	if r == nil {
		return errors.New("error Send Mail")
	}
	return err
}

type RegisterService struct {
	protobuf.UnimplementedRegisterEegServiceServer
}

func (r *RegisterService) Register(ctx context.Context, eeg *protobuf.RegisterEegRequest) (*protobuf.RegisteredEegReply, error) {

	newEeg := model.Eeg{
		Name:               eeg.Name,
		Description:        eeg.Description,
		BusinessNr:         null.Int{},
		Area:               model.AreaType(eeg.Area.String()),
		Legal:              eeg.Legal.String(),
		OperatorName:       eeg.GridName,
		CommunityId:        eeg.CommunityId,
		GridOperator:       eeg.GridId,
		RcNumber:           eeg.RcNumber,
		AllocationMode:     eeg.Allocation.String(),
		SettlementInterval: eeg.SettelmentInterval.String(),
		ProviderBusinessNr: null.Int{},
		TaxNumber:          null.StringFrom(eeg.Taxid),
		VatNumber:          null.StringFrom(eeg.Vatid),
		Address: model.Address{
			Type:         model.BILLING,
			Street:       eeg.Street,
			StreetNumber: eeg.Street,
			Zip:          eeg.Street,
			City:         eeg.Street,
		},
		AccountInfo: model.AccountInfo{
			Iban:  null.StringFrom(eeg.Iban),
			Owner: null.StringFrom(eeg.Owner),
			Sepa:  eeg.Sepa,
		},
		Contact: model.Contact{
			Phone: null.StringFrom(eeg.Phone),
			Email: null.StringFrom(eeg.Email),
		},
		Optionals: model.Optionals{
			Website: null.StringFrom(eeg.Web),
		},
		Periods: nil,
		Online:  eeg.Online,
	}

	err := database.UpdateEeg(eeg.RcNumber, &newEeg)
	if err != nil {
		log.Errorf("Could not create an EEG! %v", err.Error())
		return &protobuf.RegisteredEegReply{Status: 500},
			status.Errorf(codes.NotFound, "unknown service %v", err)
	}

	return &protobuf.RegisteredEegReply{Status: 200}, nil
}

func StartGRPCServer() {
	port := viper.GetInt("grpc-provider.port")
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		panic(err)
	}
	defer func() {
		listener.Close()
		log.Info("gRPC Server stops")
	}()
	log.Infof("gRPC Server listen on %s", fmt.Sprintf(":%d", port))
	grpcServer := grpc.NewServer()
	protobuf.RegisterRegisterEegServiceServer(grpcServer, &RegisterService{})
	grpcServer.Serve(listener)
}

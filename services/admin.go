package services

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

type SendMailFunc func(tenant, to, subject string, cc *string, body *bytes.Buffer, inlineContent []*Attachment, attachment *Attachment) error

type Attachment struct {
	Type        string
	Filename    string
	Filecontent *bytes.Buffer
	MimeType    string
	ContentId   *string
}

func SendMail(tenant, to, subject string, cc *string, body *bytes.Buffer, inlineContent []*Attachment, attachment *Attachment) error {
	log.WithField("tenant", tenant).Infof("Send Mail: from:%s to:%s sub: %s, cc: %+v, body: %s, att: %v", tenant, to, subject, cc, body, inlineContent)
	return sendHtmlInlineAttachment(tenant, to, subject, cc, body, inlineContent, attachment)
}

func sendHtmlInlineAttachment(sender, recipient, subject string, cc *string, htmlBody *bytes.Buffer, iContent []*Attachment, attachment *Attachment) error {
	//conn, err := grpc.Dial(viper.GetString("services.mail-server"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient(viper.GetString("services.mail-server"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	c := protobuf.NewSendMailServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	inlineContent := []*protobuf.Attachment{}
	for i := range iContent {
		inlineContent = append(inlineContent, &protobuf.Attachment{
			MimeType:  iContent[i].MimeType,
			Filename:  iContent[i].Filename,
			Content:   iContent[i].Filecontent.Bytes(),
			ContentId: iContent[i].ContentId,
		})
	}
	request := &protobuf.SendMailWithInlineAttachmentsRequest{
		Sender:        sender,
		Recipient:     recipient,
		Subject:       subject,
		HtmlBody:      htmlBody.String(),
		InlineContent: inlineContent,
	}
	if attachment != nil {
		request.Attachment = &protobuf.Attachment{
			MimeType: attachment.MimeType,
			Filename: attachment.Filename,
			Content:  attachment.Filecontent.Bytes(),
		}
	}

	if cc != nil {
		request.Cc = cc
	}

	r, err := c.SendMailWithInlineAttachment(ctx, request)
	if err != nil {
		log.WithField("MAIL", "INLINE").Errorf("Send Mail With Inline Attachment Error: %v", err)
		return err
	}
	log.Infof("Response from MAIL-SERVER: %v", r)
	if r == nil {
		return errors.New("error Send Mail")
	}
	if r.Status != 200 {
		return errors.New(*r.Message)
	}
	return err
}

func SendMailWithAttachment(sender, recipient, subject string, cc *string, htmlBody *bytes.Buffer, attachment *Attachment) error {
	conn, err := grpc.Dial(viper.GetString("services.mail-server"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	c := protobuf.NewSendMailServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	request := &protobuf.SendMailRequest{
		Sender:    sender,
		Recipient: recipient,
		Subject:   subject,
		Body:      htmlBody.Bytes(),
		Attachment: &protobuf.Attachment{
			MimeType:  attachment.MimeType,
			Filename:  attachment.Filename,
			Content:   attachment.Filecontent.Bytes(),
			ContentId: attachment.ContentId,
		},
	}

	if cc != nil {
		request.Cc = cc
	}

	r, err := c.SendMail(ctx, request)
	if err != nil {
		log.WithField("MAIL", "ATTACHMENT").Errorf("Send Mail With Inline Attachment Error: %v", err)
		return err
	}
	log.Infof("Response from MAIL-SERVER: %v", r)
	if r == nil {
		return errors.New("error Send Mail")
	}
	if r.Status != 200 {
		return errors.New(*r.Message)
	}
	return err
}

type RegisterService struct {
	protobuf.UnimplementedRegisterEegServiceServer
}

func (r *RegisterService) Register(ctx context.Context, eeg *protobuf.RegisterEegRequest) (*protobuf.RegisteredEegReply, error) {

	getOptionalField := func(field *string) null.String {
		if field == nil {
			return null.String{}
		}
		return null.StringFrom(*field)
	}

	newEeg := model.Eeg{
		Id:                 eeg.Tenant,
		Name:               eeg.Name,
		Description:        eeg.Description,
		BusinessNr:         null.String{},
		Area:               model.AreaType(eeg.Area.String()),
		Legal:              eeg.Legal.String(),
		OperatorName:       eeg.GridName,
		CommunityId:        eeg.CommunityId,
		GridOperator:       eeg.GridId,
		RcNumber:           eeg.RcNumber,
		AllocationMode:     model.AllocationModeType(eeg.Allocation.String()),
		SettlementInterval: eeg.SettelmentInterval.String(),
		ProviderBusinessNr: null.Int{},
		TaxNumber:          null.StringFrom(eeg.TaxNumber),
		VatNumber:          null.StringFrom(eeg.VatNumber),
		EegAddress: model.EegAddress{
			Street:       eeg.Street,
			StreetNumber: eeg.StreetNumber,
			Zip:          eeg.Zip,
			City:         eeg.City,
		},
		AccountInfo: model.AccountInfo{
			Iban:     null.StringFrom(eeg.Iban),
			Owner:    null.StringFrom(eeg.Owner),
			Sepa:     eeg.Sepa,
			BankName: null.StringFrom(eeg.BankName),
		},
		Contact: model.Contact{
			Phone: getOptionalField(eeg.Phone),
			Email: null.StringFrom(eeg.Email),
		},
		Optionals: model.Optionals{
			Website: getOptionalField(eeg.Web),
		},
		//Periods:       nil,
		Online:        eeg.Online,
		ContactPerson: null.StringFrom(eeg.EegOwner),
	}

	log.Printf("Register EEG: %+v", newEeg)
	db, err := database.ConnectToDatabase()
	if err != nil {
		log.Errorf("Database Error: %v", err)
		return &protobuf.RegisteredEegReply{Status: 500}, err
	}
	defer func() { _ = db.Close() }()

	err = database.InsertEeg(db, eeg.RcNumber, &newEeg)
	if err != nil {
		log.Errorf("Could not create an EEG! %v", err.Error())
		return &protobuf.RegisteredEegReply{Status: 500},
			status.Errorf(codes.NotFound, "unknown service %v", err)
	}

	return &protobuf.RegisteredEegReply{Status: 201}, nil
}

func StartGRPCServer(quit chan bool) {
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
	protobuf.RegisterApiServiceServer(grpcServer, &ApiService{})

	go func() {
		<-quit
		grpcServer.GracefulStop()
	}()

	err = grpcServer.Serve(listener)
	if err != nil {
		log.Printf("gRPC: Serve() error: %s", err)
	}
	log.Println("gRPC listener stopped")
}

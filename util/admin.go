package util

import (
	protobuf "at.ourproject/vfeeg-backend/proto"
	"bytes"
	"context"
	"errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"time"
)

func SendMail(tenant, to, subject string, body *bytes.Buffer, fileName *string, fileContent *bytes.Buffer) error {
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

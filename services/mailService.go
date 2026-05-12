package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	protobuf "at.ourproject/vfeeg-backend/proto"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	if err := ensureMailAddress(to); err != nil {
		return err
	}
	if cc != nil {
		if err := ensureMailAddress(*cc); err != nil {
			return err
		}
	}
	return sendHtmlInlineAttachment(tenant, to, subject, cc, body, inlineContent, attachment)
}

func ensureMailAddress(to string) error {
	return verifyEmail(to)
}

func isValidEmail(email string) error {
	// Regular expression for validating an Email
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	v := re.MatchString(email)
	if !v {
		return errors.New(fmt.Sprintf("invalid email (%s)", email))
	}
	return nil
}

func verifyDomain(email string) error {
	domain := email[strings.Index(email, "@")+1:]
	_, err := net.LookupMX(domain)
	return err
}

func verifyEmail(email string) error {
	if err := isValidEmail(email); err != nil {
		return err
	}
	return nil //verifyDomain(email)
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

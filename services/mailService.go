package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"at.ourproject/vfeeg-backend/model"
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
	log.WithField("tenant", tenant).Infof("Send Mail: from:%s sub: %s, att: %v", tenant, subject, inlineContent != nil)
	to, cc, err := normalizedRecipients(to, cc)
	if err != nil {
		return err
	}
	return sendHtmlInlineAttachment(tenant, to, subject, cc, body, inlineContent, attachment)
}

// normalizedRecipients trims each ';'-separated part of to/cc
// (model.NormalizeEmailList) and validates against the shared address
// rule. The normalized values are what actually gets sent — validating
// alone would let a green check pass while the raw string still fails
// downstream. An empty to after normalization is an error (a mail
// needs a recipient); an empty cc becomes nil.
func normalizedRecipients(to string, cc *string) (string, *string, error) {
	to, err := model.ValidateEmailList(to)
	if err != nil {
		return "", nil, err
	}
	if to == "" {
		return "", nil, errors.New("no valid recipient address")
	}
	if cc != nil {
		normalizedCc, err := model.ValidateEmailList(*cc)
		if err != nil {
			return "", nil, err
		}
		if normalizedCc == "" {
			cc = nil
		} else {
			cc = &normalizedCc
		}
	}
	return to, cc, nil
}

// checkRejectedRecipients surfaces recipients the mail server refused
// (reported via the additive SendMailReply.rejectedRecipients field) so
// callers can raise an admin notification instead of losing them
// silently. IMPORTANT: delivery to the remaining recipients has already
// happened — the message must read as a partial delivery, not as a
// failed send, or admins will re-send and produce duplicates.
func checkRejectedRecipients(rejected []string) error {
	if len(rejected) > 0 {
		return fmt.Errorf("Mail an gültige Empfänger zugestellt, aber NICHT an (Adresse ungültig): %s — Adresse korrigieren, kein erneuter Versand an die übrigen Empfänger nötig", strings.Join(rejected, ";"))
	}
	return nil
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
	return checkRejectedRecipients(r.GetRejectedRecipients())
}

func SendMailWithAttachment(sender, recipient, subject string, cc *string, htmlBody *bytes.Buffer, attachment *Attachment) error {
	recipient, cc, err := normalizedRecipients(recipient, cc)
	if err != nil {
		return err
	}
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
	return checkRejectedRecipients(r.GetRejectedRecipients())
}

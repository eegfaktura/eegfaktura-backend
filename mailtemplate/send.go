package mailtemplate

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/services"
	log "github.com/sirupsen/logrus"
)

// Store is what sending a customised mail needs from the database.
type Store interface {
	GetMailTemplate(ctx context.Context, tenant, key string) (*model.MailTemplate, error)
	ListMailTemplateFiles(ctx context.Context, tenant, key string, withContent bool) ([]model.MailTemplateFile, error)
	SaveNotificationFromMap(notificationValue map[string]interface{}, tenant string, notificationType model.NotificationType, process model.NotificationProcess, role string) error
}

// SendStored sends the customised version of a member mail when the community
// has one. handled=false means the caller sends the legacy template: either
// there is no customised version, or it could not be used — then the community
// gets an error notification and the member still gets the default mail.
// A send error after a successful render is returned with handled=true.
func SendStored(ctx context.Context, store Store, sendMail services.SendMailFunc, def *Definition,
	tenant, to string, cc *string, data *Data, identifier string) (handled bool, err error) {

	if store == nil || def == nil {
		return false, nil
	}
	t, err := store.GetMailTemplate(ctx, tenant, def.Key)
	if err != nil {
		log.WithField("tenant", tenant).WithError(err).Errorf("mail template %s: can not load, using default", def.Key)
		return false, nil
	}
	if t == nil {
		return false, nil
	}
	if !strings.EqualFold(t.Tenant, tenant) {
		log.WithField("tenant", tenant).Errorf("mail template %s: tenant mismatch, using default", def.Key)
		return false, nil
	}
	files, err := store.ListMailTemplateFiles(ctx, tenant, def.Key, true)
	if err != nil {
		notifyFallback(store, def, tenant, identifier, err)
		return false, nil
	}
	attachmentId := ""
	if t.AttachmentFileId != nil {
		attachmentId = *t.AttachmentFileId
	}
	r, err := Render(def, t.Subject, t.BodyHtml, attachmentId, data, files, ImagesInline)
	if err != nil {
		notifyFallback(store, def, tenant, identifier, err)
		return false, nil
	}
	log.WithField("tenant", tenant).Infof("Mail %s for tenant %q rendered from the customised version %d", def.Key, tenant, t.Version)
	return true, sendMail(tenant, to, r.Subject, cc, bytes.NewBufferString(r.Html), r.Inline, r.Attachment)
}

// notifyFallback tells the community that its customised mail was not used.
func notifyFallback(store Store, def *Definition, tenant, identifier string, cause error) {
	log.WithField("tenant", tenant).WithError(cause).Errorf("mail template %s: can not render, sending default", def.Key)
	msg := fmt.Sprintf("Ihre angepasste Mail „%s“ konnte nicht verwendet werden; es wurde der Standardtext verschickt. Grund: %s", def.Name, cause.Error())
	_ = store.SaveNotificationFromMap(database.CreateNotificationMessageFromLog(&model.Log{
		Operation: "Mail",
		Messages:  []*model.LogMessage{model.NewLogMessage("ERROR", identifier, "E_MAIL_TEMPLATE", msg)},
	}), tenant, model.N_TYPE_ERROR, model.N_PROCESS_EDA_PROCESS, "ADMIN")
}

// NoticeStore is what the default-change notice needs from the database.
type NoticeStore interface {
	ClaimMailTemplateDefaultNotices(ctx context.Context, key string, defaultVersion int) ([]string, error)
	SaveNotificationFromMap(notificationValue map[string]interface{}, tenant string, notificationType model.NotificationType, process model.NotificationProcess, role string) error
}

// NotifyDefaultChanges tells each community once that the default of a mail it
// customised has changed (concept G.2). Claiming the rows first makes a second
// replica starting at the same time skip them.
func NotifyDefaultChanges(ctx context.Context, store NoticeStore) {
	for _, def := range Catalog() {
		tenants, err := store.ClaimMailTemplateDefaultNotices(ctx, def.Key, def.DefaultVersion)
		if err != nil {
			log.WithError(err).Errorf("mail template %s: default change notice failed", def.Key)
			continue
		}
		for _, tenant := range tenants {
			msg := fmt.Sprintf("Der Standardtext der Mail „%s“ wurde geändert. Ihre angepasste Fassung wird weiter verwendet; den neuen Standard sehen Sie unter Einstellungen → E-Mail-Texte.", def.Name)
			_ = store.SaveNotificationFromMap(database.CreateNotificationMessageFromLog(&model.Log{
				Operation: "Mail",
				Messages:  []*model.LogMessage{model.NewLogMessage("INFO", def.Key, "I_MAIL_TEMPLATE_DEFAULT", msg)},
			}), tenant, model.N_TYPE_NOTIFICATION, model.N_PROCESS_EDA_PROCESS, "ADMIN")
		}
	}
}

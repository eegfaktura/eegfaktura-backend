package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/mailtemplate"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/parser"
	"at.ourproject/vfeeg-backend/public"
	"at.ourproject/vfeeg-backend/services"
	"github.com/gabriel-vasile/mimetype"
	"github.com/gorilla/mux"
	"github.com/pborman/uuid"
	log "github.com/sirupsen/logrus"
)

// MailTemplateHandler serves /eeg/mailtemplates: the member mails a community
// customises (konzept-mail-texte-editierbar.md, section H).
type MailTemplateHandler struct {
	db       database.Database
	sendMail services.SendMailFunc
}

const (
	maxMailTemplateJSONBytes = 1 << 20
	// multipart overhead on top of the largest file
	maxMailFileUploadOverhead = 1 << 20
	testSubjectPrefix         = "[Test] "
)

func initMailTemplateRoutes(s *mux.Router, db database.Database) {
	registerMailTemplateRoutes(s, &MailTemplateHandler{db: db, sendMail: services.SendMail})
}

func registerMailTemplateRoutes(s *mux.Router, h *MailTemplateHandler) {
	s.HandleFunc("/mailtemplates", middleware.Protect(h.list())).Methods("GET")
	s.HandleFunc("/mailtemplates/{key}", middleware.Protect(h.get())).Methods("GET")
	s.HandleFunc("/mailtemplates/{key}", middleware.Protect(h.save())).Methods("PUT")
	s.HandleFunc("/mailtemplates/{key}", middleware.Protect(h.reset())).Methods("DELETE")
	s.HandleFunc("/mailtemplates/{key}/files", middleware.Protect(h.uploadFile())).Methods("POST")
	s.HandleFunc("/mailtemplates/{key}/files/{id}", middleware.Protect(h.getFile())).Methods("GET")
	s.HandleFunc("/mailtemplates/{key}/files/{id}", middleware.Protect(h.deleteFile())).Methods("DELETE")
	s.HandleFunc("/mailtemplates/{key}/preview", middleware.Protect(h.preview())).Methods("POST")
	s.HandleFunc("/mailtemplates/{key}/test", middleware.Protect(h.test())).Methods("POST")
}

type mailTemplateSummary struct {
	Key            string     `json:"key"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Customized     bool       `json:"customized"`
	UpdatedAt      *time.Time `json:"updatedAt"`
	UpdatedBy      *string    `json:"updatedBy"`
	DefaultChanged bool       `json:"defaultChanged"`
}

type mailTemplateDTO struct {
	Key             string     `json:"key"`
	Name            string     `json:"name"`
	Description     string     `json:"description"`
	Subject         string     `json:"subject"`
	BodyHtml        string     `json:"bodyHtml"`
	Customized      bool       `json:"customized"`
	Version         int        `json:"version"`
	DefaultVersion  int        `json:"defaultVersion"`
	DefaultChanged  bool       `json:"defaultChanged"`
	DefaultSubject  string     `json:"defaultSubject"`
	DefaultBodyHtml string     `json:"defaultBodyHtml"`
	UpdatedAt       *time.Time `json:"updatedAt"`
	UpdatedBy       *string    `json:"updatedBy"`
	AttachmentFile  *string    `json:"attachmentFileId"`
	// LegacyTemplateActive: no saved version and a hand-made template on the volume applies
	LegacyTemplateActive bool                       `json:"legacyTemplateActive"`
	Placeholders         []mailtemplate.Placeholder `json:"placeholders"`
	Files                []model.MailTemplateFile   `json:"files"`
	Limits               mailtemplate.Limits        `json:"limits"`
}

type mailTemplateRequest struct {
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	BodyHtml  string `json:"bodyHtml"`
	Version   int    `json:"version"`
	// AttachmentFileId: the PDF; null = none. Preview and test mail without the field use the saved version's PDF.
	AttachmentFileId json.RawMessage `json:"attachmentFileId"`
}

// attachment returns the PDF id of a request; present=false when the field is missing.
func (req *mailTemplateRequest) attachment() (id *string, present bool, err *mailtemplate.ValidationError) {
	raw := strings.TrimSpace(string(req.AttachmentFileId))
	if raw == "" {
		return nil, false, nil
	}
	if raw == "null" {
		return nil, true, nil
	}
	var v string
	if json.Unmarshal(req.AttachmentFileId, &v) != nil || uuid.Parse(v) == nil {
		return nil, true, &mailtemplate.ValidationError{Field: "attachmentFileId", Message: "Ungültige Datei-ID."}
	}
	v = strings.ToLower(v)
	return &v, true, nil
}

func summaryOf(def *mailtemplate.Definition, t *model.MailTemplate) mailTemplateSummary {
	s := mailTemplateSummary{Key: def.Key, Name: def.Name, Description: def.Description}
	if t != nil {
		s.Customized = true
		s.UpdatedAt = &t.UpdatedAt
		s.UpdatedBy = &t.UpdatedBy
		s.DefaultChanged = t.DefaultVersion < def.DefaultVersion
	}
	return s
}

func dtoOf(def *mailtemplate.Definition, t *model.MailTemplate, files []model.MailTemplateFile) mailTemplateDTO {
	s := summaryOf(def, t)
	dto := mailTemplateDTO{
		Key: def.Key, Name: def.Name, Description: def.Description,
		Subject: def.DefaultSubject, BodyHtml: def.DefaultBodyHtml(),
		Customized: s.Customized, DefaultVersion: def.DefaultVersion, DefaultChanged: s.DefaultChanged,
		DefaultSubject: def.DefaultSubject, DefaultBodyHtml: def.DefaultBodyHtml(),
		UpdatedAt: s.UpdatedAt, UpdatedBy: s.UpdatedBy,
		Placeholders: def.Placeholders, Files: files, Limits: def.Limits,
	}
	if t != nil {
		dto.Subject, dto.BodyHtml, dto.Version, dto.AttachmentFile = t.Subject, t.BodyHtml, t.Version, t.AttachmentFileId
	}
	return dto
}

// definition resolves the {key} path parameter; false after a 404 answer.
func definition(w http.ResponseWriter, r *http.Request) (*mailtemplate.Definition, bool) {
	def := mailtemplate.Lookup(mux.Vars(r)["key"])
	if def == nil {
		respondWithJSON(w, http.StatusNotFound, map[string]string{"message": "Unbekannte Mail"})
		return nil, false
	}
	return def, true
}

func respondValidation(w http.ResponseWriter, errs ...*mailtemplate.ValidationError) {
	respondWithJSON(w, http.StatusBadRequest, map[string]interface{}{"errors": errs})
}

func respondServerError(w http.ResponseWriter, tenant string, err error) {
	log.WithField("tenant", tenant).WithError(err).Error("mail template")
	respondWithJSON(w, http.StatusInternalServerError, map[string]string{"message": "Interner Fehler"})
}

// loadTemplate loads the customised mail and asserts it belongs to the request tenant.
func (h *MailTemplateHandler) loadTemplate(w http.ResponseWriter, r *http.Request, tenant string, def *mailtemplate.Definition) (*model.MailTemplate, bool) {
	t, err := h.db.GetMailTemplate(r.Context(), tenant, def.Key)
	if err != nil {
		respondServerError(w, tenant, err)
		return nil, false
	}
	if t != nil && !strings.EqualFold(t.Tenant, tenant) {
		w.WriteHeader(http.StatusForbidden)
		return nil, false
	}
	return t, true
}

// loadFiles loads the files of a mail and asserts they belong to the request tenant.
func (h *MailTemplateHandler) loadFiles(w http.ResponseWriter, r *http.Request, tenant string, def *mailtemplate.Definition, withContent bool) ([]model.MailTemplateFile, bool) {
	files, err := h.db.ListMailTemplateFiles(r.Context(), tenant, def.Key, withContent)
	if err != nil {
		respondServerError(w, tenant, err)
		return nil, false
	}
	for _, f := range files {
		if !strings.EqualFold(f.Tenant, tenant) {
			w.WriteHeader(http.StatusForbidden)
			return nil, false
		}
	}
	return files, true
}

func (h *MailTemplateHandler) list() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		stored, err := h.db.ListMailTemplates(r.Context(), tenant)
		if err != nil {
			respondServerError(w, tenant, err)
			return
		}
		byKey := map[string]*model.MailTemplate{}
		for i := range stored {
			if !strings.EqualFold(stored[i].Tenant, tenant) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			byKey[stored[i].MailKey] = &stored[i]
		}
		out := []mailTemplateSummary{}
		for _, def := range mailtemplate.Catalog() {
			def := def
			out = append(out, summaryOf(&def, byKey[def.Key]))
		}
		respondWithJSON(w, http.StatusOK, out)
	}
}

func (h *MailTemplateHandler) get() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		def, ok := definition(w, r)
		if !ok {
			return
		}
		t, ok := h.loadTemplate(w, r, tenant, def)
		if !ok {
			return
		}
		files, ok := h.loadFiles(w, r, tenant, def, false)
		if !ok {
			return
		}
		dto := dtoOf(def, t, files)
		dto.LegacyTemplateActive = t == nil && parser.TenantTemplateActive(tenant, def.LegacyConfig)
		respondWithJSON(w, http.StatusOK, dto)
	}
}

// decodeRequest reads the JSON body; false after a 400 answer.
func decodeRequest(w http.ResponseWriter, r *http.Request) (*mailTemplateRequest, bool) {
	var req mailTemplateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxMailTemplateJSONBytes)).Decode(&req); err != nil {
		respondValidation(w, &mailtemplate.ValidationError{Field: "body", Message: "Ungültige Anfrage."})
		return nil, false
	}
	return &req, true
}

// sanitize checks subject and body of a request; false after a 400 answer.
func sanitize(w http.ResponseWriter, def *mailtemplate.Definition, req *mailTemplateRequest, fileOK mailtemplate.FileCheck) (string, string, bool) {
	var errs []*mailtemplate.ValidationError
	var verr *mailtemplate.ValidationError
	subject, err := mailtemplate.SanitizeSubject(def, req.Subject)
	if errors.As(err, &verr) {
		errs = append(errs, verr)
	}
	body, err := mailtemplate.SanitizeBody(def, req.BodyHtml, fileOK)
	if errors.As(err, &verr) {
		errs = append(errs, verr)
	}
	if len(errs) > 0 {
		respondValidation(w, errs...)
		return "", "", false
	}
	return subject, body, true
}

func (h *MailTemplateHandler) save() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		def, ok := definition(w, r)
		if !ok {
			return
		}
		req, ok := decodeRequest(w, r)
		if !ok {
			return
		}
		if req.Version < 0 {
			respondValidation(w, &mailtemplate.ValidationError{Field: "version", Message: "Ungültige Version."})
			return
		}
		// every well-formed image id stays in the text; whether the file exists (also as
		// an older draft) is decided under the lock of the save, so a vanished image is
		// a 400 and never silently dropped
		subject, body, ok := sanitize(w, def, req, func(id string) bool { return uuid.Parse(id) != nil })
		if !ok {
			return
		}
		attachmentId, _, aerr := req.attachment()
		if aerr != nil {
			respondValidation(w, aerr)
			return
		}
		images := mailtemplate.ReferencedImages(body)
		saved, err := h.db.SaveMailTemplate(r.Context(), &model.MailTemplate{
			Tenant: tenant, MailKey: def.Key, Subject: subject, BodyHtml: body,
			DefaultVersion: def.DefaultVersion, UpdatedBy: claims.Username, AttachmentFileId: attachmentId,
		}, req.Version, images, func(all []model.MailTemplateFile) error {
			return checkVersionFiles(def, tenant, images, attachmentId, all)
		})
		var verr *mailtemplate.ValidationError
		if errors.As(err, &verr) {
			respondValidation(w, verr)
			return
		}
		var conflict *database.MailTemplateVersionError
		if errors.As(err, &conflict) {
			respondWithJSON(w, http.StatusConflict, map[string]interface{}{
				"message":        "Die Mail wurde inzwischen geändert.",
				"currentVersion": conflict.Current,
			})
			return
		}
		if err != nil {
			respondServerError(w, tenant, err)
			return
		}
		files, ok := h.loadFiles(w, r, tenant, def, false)
		if !ok {
			return
		}
		log.WithField("tenant", tenant).Infof("mail template %s saved, version %d", def.Key, saved.Version)
		respondWithJSON(w, http.StatusOK, dtoOf(def, saved, files))
	}
}

// checkVersionFiles checks the files a version will use: they exist for this
// mail and tenant, have the right kind and stay within the limits.
func checkVersionFiles(def *mailtemplate.Definition, tenant string, images []string, attachmentId *string, all []model.MailTemplateFile) error {
	byId := map[string]*model.MailTemplateFile{}
	for i := range all {
		if !strings.EqualFold(all[i].Tenant, tenant) {
			return errors.New("tenant mismatch")
		}
		byId[all[i].Id] = &all[i]
	}
	var total int64
	seen := map[string]bool{}
	for _, id := range images {
		f := byId[id]
		if f == nil || f.Kind != model.MailFileKindImage {
			return &mailtemplate.ValidationError{Field: "bodyHtml", Message: "Ein Bild im Text gibt es nicht (mehr); bitte erneut einfügen."}
		}
		if !seen[id] {
			seen[id] = true
			total += f.Size
		}
	}
	if len(seen) > def.Limits.MaxImages {
		return &mailtemplate.ValidationError{Field: "bodyHtml", Message: fmt.Sprintf("Höchstens %d Bilder je Mail.", def.Limits.MaxImages)}
	}
	if attachmentId != nil {
		f := byId[*attachmentId]
		if f == nil || f.Kind != model.MailFileKindAttachment {
			return &mailtemplate.ValidationError{Field: "attachmentFileId", Message: "Den PDF-Anhang gibt es nicht (mehr)."}
		}
		total += f.Size
	}
	if total > def.Limits.MaxTotalBytes {
		return &mailtemplate.ValidationError{Field: "files", Message: fmt.Sprintf("Alle Dateien einer Mail zusammen dürfen höchstens %s groß sein.", formatBytes(def.Limits.MaxTotalBytes))}
	}
	return nil
}

func (h *MailTemplateHandler) reset() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		def, ok := definition(w, r)
		if !ok {
			return
		}
		if _, ok = h.loadTemplate(w, r, tenant, def); !ok {
			return
		}
		if err := h.db.DeleteMailTemplate(r.Context(), tenant, def.Key); err != nil {
			respondServerError(w, tenant, err)
			return
		}
		log.WithField("tenant", tenant).Infof("mail template %s reset to default", def.Key)
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *MailTemplateHandler) uploadFile() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		def, ok := definition(w, r)
		if !ok {
			return
		}
		limits := def.Limits
		r.Body = http.MaxBytesReader(w, r.Body, limits.MaxAttachmentBytes+maxMailFileUploadOverhead)
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			respondValidation(w, &mailtemplate.ValidationError{Field: "file", Message: "Die Datei ist zu groß oder die Anfrage ist ungültig."})
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()

		kind := r.FormValue("kind")
		var maxBytes int64
		switch kind {
		case model.MailFileKindImage:
			maxBytes = limits.MaxImageBytes
		case model.MailFileKindAttachment:
			maxBytes = limits.MaxAttachmentBytes
		default:
			respondValidation(w, &mailtemplate.ValidationError{Field: "kind", Message: "kind muss image oder attachment sein."})
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			respondValidation(w, &mailtemplate.ValidationError{Field: "file", Message: "Keine Datei angegeben."})
			return
		}
		defer func() { _ = file.Close() }()
		content, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
		if err != nil {
			respondValidation(w, &mailtemplate.ValidationError{Field: "file", Message: "Die Datei konnte nicht gelesen werden."})
			return
		}
		if int64(len(content)) > maxBytes {
			respondValidation(w, &mailtemplate.ValidationError{Field: "file", Message: fmt.Sprintf("Die Datei ist zu groß (höchstens %s).", formatBytes(maxBytes))})
			return
		}
		if len(content) == 0 {
			respondValidation(w, &mailtemplate.ValidationError{Field: "file", Message: "Die Datei ist leer."})
			return
		}
		// the type is checked on the content, never on name or Content-Type of the client
		detected := mimetype.Detect(content)
		var mimeType string
		switch {
		case kind == model.MailFileKindImage && detected.Is("image/png"):
			mimeType = "image/png"
		case kind == model.MailFileKindImage && detected.Is("image/jpeg"):
			mimeType = "image/jpeg"
		case kind == model.MailFileKindAttachment && detected.Is("application/pdf"):
			mimeType = "application/pdf"
		default:
			msg := "Bilder müssen PNG oder JPEG sein."
			if kind == model.MailFileKindAttachment {
				msg = "Der Anhang muss ein PDF sein."
			}
			respondValidation(w, &mailtemplate.ValidationError{Field: "file", Message: msg})
			return
		}

		f := &model.MailTemplateFile{
			Id: uuid.New(), Tenant: tenant, MailKey: def.Key, Kind: kind,
			Filename: cleanFilename(header.Filename, mimeType), MimeType: mimeType,
			Size: int64(len(content)), Content: content,
		}
		// uploads are drafts: the limits apply to the drafts here and to the saved version on PUT
		err = h.db.InsertMailTemplateFile(r.Context(), f, func(existing []model.MailTemplateFile) error {
			images, attachments, total := 0, 0, f.Size
			for _, e := range existing {
				if !strings.EqualFold(e.Tenant, tenant) {
					return errors.New("tenant mismatch")
				}
				if e.Committed {
					continue
				}
				total += e.Size
				if e.Kind == model.MailFileKindImage {
					images++
				} else {
					attachments++
				}
			}
			switch {
			case kind == model.MailFileKindImage && images >= limits.MaxImages:
				return &mailtemplate.ValidationError{Field: "file", Message: fmt.Sprintf("Höchstens %d neue Bilder je Mail vor dem Speichern.", limits.MaxImages)}
			case kind == model.MailFileKindAttachment && attachments >= limits.MaxAttachments:
				return &mailtemplate.ValidationError{Field: "file", Message: fmt.Sprintf("Höchstens %d neuer PDF-Anhang je Mail vor dem Speichern; bitte den anderen zuerst löschen.", limits.MaxAttachments)}
			case total > limits.MaxTotalBytes:
				return &mailtemplate.ValidationError{Field: "file", Message: fmt.Sprintf("Neue Dateien einer Mail dürfen zusammen höchstens %s groß sein.", formatBytes(limits.MaxTotalBytes))}
			}
			return nil
		})
		var verr *mailtemplate.ValidationError
		if errors.As(err, &verr) {
			respondValidation(w, verr)
			return
		}
		if err != nil {
			respondServerError(w, tenant, err)
			return
		}
		log.WithField("tenant", tenant).Infof("mail template %s: %s uploaded (%d bytes)", def.Key, kind, f.Size)
		respondWithJSON(w, http.StatusCreated, f)
	}
}

// loadFile resolves {id} and asserts the file belongs to the tenant and mail; false after an answer.
func (h *MailTemplateHandler) loadFile(w http.ResponseWriter, r *http.Request, tenant string, def *mailtemplate.Definition) (*model.MailTemplateFile, bool) {
	id := mux.Vars(r)["id"]
	if uuid.Parse(id) == nil {
		respondValidation(w, &mailtemplate.ValidationError{Field: "id", Message: "Ungültige Datei-ID."})
		return nil, false
	}
	f, err := h.db.GetMailTemplateFile(r.Context(), id)
	if err != nil {
		respondServerError(w, tenant, err)
		return nil, false
	}
	if f == nil {
		respondWithJSON(w, http.StatusNotFound, map[string]string{"message": "Datei nicht gefunden"})
		return nil, false
	}
	if !strings.EqualFold(f.Tenant, tenant) {
		log.WithField("tenant", tenant).Warn("mail template file of another tenant requested")
		w.WriteHeader(http.StatusForbidden)
		return nil, false
	}
	if f.MailKey != def.Key {
		respondWithJSON(w, http.StatusNotFound, map[string]string{"message": "Datei nicht gefunden"})
		return nil, false
	}
	return f, true
}

func (h *MailTemplateHandler) getFile() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		def, ok := definition(w, r)
		if !ok {
			return
		}
		var f *model.MailTemplateFile
		if mux.Vars(r)["id"] == mailtemplate.BuiltinLogoId {
			logo, err := public.Templates.ReadFile("templates/eegfaktura-logo.png")
			if err != nil {
				respondServerError(w, tenant, err)
				return
			}
			f = &model.MailTemplateFile{Filename: "eegfaktura-logo.png", MimeType: "image/png", Content: logo}
		} else if f, ok = h.loadFile(w, r, tenant, def); !ok {
			return
		}
		disposition := "inline"
		if f.Kind == model.MailFileKindAttachment {
			disposition = "attachment"
		}
		w.Header().Set("Content-Type", f.MimeType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.Filename}))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(f.Content)
	}
}

func (h *MailTemplateHandler) deleteFile() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		def, ok := definition(w, r)
		if !ok {
			return
		}
		f, ok := h.loadFile(w, r, tenant, def)
		if !ok {
			return
		}
		if f.Committed {
			t, ok := h.loadTemplate(w, r, tenant, def)
			if !ok {
				return
			}
			if t != nil && usesFile(t, f.Id) {
				respondWithJSON(w, http.StatusConflict, map[string]string{
					"message": "Die gespeicherte Fassung verwendet diese Datei; bitte zuerst ohne sie speichern."})
				return
			}
		}
		if _, err := h.db.DeleteMailTemplateFile(r.Context(), tenant, def.Key, f.Id); err != nil {
			respondServerError(w, tenant, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// imageCheck accepts the images among files.
func imageCheck(files []model.MailTemplateFile) mailtemplate.FileCheck {
	images := map[string]bool{}
	for _, f := range files {
		if f.Kind == model.MailFileKindImage {
			images[f.Id] = true
		}
	}
	return func(id string) bool { return images[id] }
}

// usesFile reports whether the saved version uses a file.
func usesFile(t *model.MailTemplate, id string) bool {
	if t.AttachmentFileId != nil && strings.EqualFold(*t.AttachmentFileId, id) {
		return true
	}
	for _, ref := range mailtemplate.ReferencedImages(t.BodyHtml) {
		if strings.EqualFold(ref, id) {
			return true
		}
	}
	return false
}

// render renders an unsaved version with the sample member and the real community; false after an answer.
func (h *MailTemplateHandler) render(w http.ResponseWriter, r *http.Request, tenant string, def *mailtemplate.Definition,
	req *mailTemplateRequest, mode mailtemplate.ImageMode) (*mailtemplate.Rendered, *model.Eeg, bool) {

	// an unsaved text may use the saved version's files and the drafts of this mail
	files, ok := h.loadFiles(w, r, tenant, def, true)
	if !ok {
		return nil, nil, false
	}
	subject, body, ok := sanitize(w, def, req, imageCheck(files))
	if !ok {
		return nil, nil, false
	}
	attachmentId, present, aerr := req.attachment()
	if aerr != nil {
		respondValidation(w, aerr)
		return nil, nil, false
	}
	if !present {
		t, ok := h.loadTemplate(w, r, tenant, def)
		if !ok {
			return nil, nil, false
		}
		if t != nil {
			attachmentId = t.AttachmentFileId
		}
	}
	attachment := ""
	if attachmentId != nil {
		attachment = *attachmentId
	}
	eeg, err := h.db.GetEegById(r.Context(), tenant)
	if err != nil || eeg == nil || !strings.EqualFold(eeg.Id, tenant) {
		respondWithJSON(w, http.StatusNotFound, map[string]string{"message": "EEG nicht gefunden"})
		return nil, nil, false
	}
	rendered, err := mailtemplate.Render(def, subject, body, attachment, mailtemplate.SampleData(eeg), files, mode)
	if errors.Is(err, mailtemplate.ErrMissingFile) {
		respondValidation(w, &mailtemplate.ValidationError{Field: "attachmentFileId", Message: "Den PDF-Anhang gibt es nicht (mehr)."})
		return nil, nil, false
	}
	if errors.Is(err, mailtemplate.ErrTooLarge) {
		respondValidation(w, &mailtemplate.ValidationError{Field: "files", Message: fmt.Sprintf("Die Dateien der Mail sind zusammen größer als %s.", formatBytes(def.Limits.MaxTotalBytes))})
		return nil, nil, false
	}
	var verr *mailtemplate.ValidationError
	if errors.As(err, &verr) {
		respondValidation(w, verr)
		return nil, nil, false
	}
	if err != nil {
		respondServerError(w, tenant, err)
		return nil, nil, false
	}
	return rendered, eeg, true
}

func (h *MailTemplateHandler) preview() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		def, ok := definition(w, r)
		if !ok {
			return
		}
		req, ok := decodeRequest(w, r)
		if !ok {
			return
		}
		if html, ok := h.legacyPreview(r, tenant, def, req); ok {
			respondWithJSON(w, http.StatusOK, map[string]string{"subject": def.DefaultSubject, "html": html})
			return
		}
		rendered, _, ok := h.render(w, r, tenant, def, req, mailtemplate.ImagesDataURI)
		if !ok {
			return
		}
		respondWithJSON(w, http.StatusOK, map[string]string{"subject": rendered.Subject, "html": rendered.Html})
	}
}

// legacyPreview: while a hand-made volume template applies (no saved version),
// the unchanged default in the editor previews that template, which is what
// members get today (section H, legacyTemplateActive).
func (h *MailTemplateHandler) legacyPreview(r *http.Request, tenant string, def *mailtemplate.Definition, req *mailTemplateRequest) (string, bool) {
	if !parser.TenantTemplateActive(tenant, def.LegacyConfig) {
		return "", false
	}
	subject, err := mailtemplate.SanitizeSubject(def, req.Subject)
	if err != nil || subject != def.DefaultSubject {
		return "", false
	}
	body, err := mailtemplate.SanitizeBody(def, req.BodyHtml, nil)
	if err != nil || body != def.DefaultBodyHtml() {
		return "", false
	}
	if t, err := h.db.GetMailTemplate(r.Context(), tenant, def.Key); err != nil || t != nil {
		return "", false
	}
	eeg, err := h.db.GetEegById(r.Context(), tenant)
	if err != nil || eeg == nil || !strings.EqualFold(eeg.Id, tenant) {
		return "", false
	}
	html, err := parser.RenderLegacyPreview(tenant, def.LegacyConfig, eeg, mailtemplate.SampleData(eeg))
	if err != nil {
		log.WithField("tenant", tenant).WithError(err).Warn("mail template: legacy preview failed")
		return "", false
	}
	return html, true
}

func (h *MailTemplateHandler) test() middleware.JWTHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, claims *middleware.PlatformClaims, tenant string) {
		def, ok := definition(w, r)
		if !ok {
			return
		}
		req, ok := decodeRequest(w, r)
		if !ok {
			return
		}
		if req.Recipient != "me" && req.Recipient != "eeg" {
			respondValidation(w, &mailtemplate.ValidationError{Field: "recipient", Message: "recipient muss me oder eeg sein."})
			return
		}
		rendered, eeg, ok := h.render(w, r, tenant, def, req, mailtemplate.ImagesInline)
		if !ok {
			return
		}
		// the recipient is chosen by the service, never given by the browser
		to := strings.TrimSpace(claims.Email)
		if req.Recipient == "eeg" {
			to = strings.TrimSpace(eeg.Email.String)
		}
		if to == "" {
			msg := "Für deinen Login ist keine E-Mail-Adresse hinterlegt."
			if req.Recipient == "eeg" {
				msg = "Für die Gemeinschaft ist keine E-Mail-Adresse hinterlegt."
			}
			respondValidation(w, &mailtemplate.ValidationError{Field: "recipient", Message: msg})
			return
		}
		if err := h.sendMail(tenant, to, testSubjectPrefix+rendered.Subject, nil,
			bytes.NewBufferString(rendered.Html), rendered.Inline, rendered.Attachment); err != nil {
			log.WithField("tenant", tenant).WithError(err).Error("mail template: test mail failed")
			respondWithJSON(w, http.StatusBadGateway, map[string]string{"message": "Die Testmail konnte nicht verschickt werden."})
			return
		}
		respondWithJSON(w, http.StatusOK, map[string]string{"sentTo": to})
	}
}

// cleanFilename keeps the base name of an uploaded file without control characters.
func cleanFilename(name, mimeType string) string {
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if runes := []rune(name); len(runes) > 200 {
		name = string(runes[len(runes)-200:])
	}
	if name == "" || name == "." || name == "/" {
		ext := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "application/pdf": ".pdf"}[mimeType]
		name = "datei" + ext
	}
	return name
}

func formatBytes(n int64) string {
	if n >= 1024*1024 && n%(1024*1024) == 0 {
		return fmt.Sprintf("%d MB", n/(1024*1024))
	}
	return fmt.Sprintf("%d KB", n/1024)
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/api/middleware"
	"at.ourproject/vfeeg-backend/database"
	"at.ourproject/vfeeg-backend/public"
	"at.ourproject/vfeeg-backend/services"
	"github.com/gorilla/mux"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mtTenant = "TE000300"
const mtOtherTenant = "TE000301"

type mtSent struct {
	tenant, to, subject string
	cc                  *string
	body                string
	inline              int
	attachment          bool
}

type mtClient struct {
	t      *testing.T
	router *mux.Router
	sent   []mtSent
}

func newMailTemplateClient(t *testing.T) *mtClient {
	ctx := context.Background()
	db, err := database.GetDB(ctx)
	require.NoError(t, err)
	for _, tenant := range []string{mtTenant, mtOtherTenant} {
		require.NoError(t, db.DeleteMailTemplate(ctx, tenant, "consent-request"))
		require.NoError(t, db.DeleteMailTemplate(ctx, tenant, "zp-active"))
	}
	_, err = execTestSQL(ctx, db, `INSERT INTO base.eeg
		(tenant, name, description, "rcNumber", area, gridoperator_code, gridoperator_name, "communityId",
		 street, "streetNumber", city, zip, email, phone)
		VALUES ($1, 'MAIL-TEST', 'Mailtest Gemeinschaft', $2, 'LOCAL', 'AT009999', 'Test Netz', 'AT00999900000TC000300000000000001',
		 'Solarweg', '1', 'Linz', '4020', 'eeg-mail-test@example.org; office@example.org', '+43 1 300')
		ON CONFLICT (tenant) DO NOTHING`, mtTenant, mtTenant)
	require.NoError(t, err)

	orig := middleware.VerifyTokenClaims
	t.Cleanup(func() { middleware.VerifyTokenClaims = orig })
	middleware.VerifyTokenClaims = func(_ context.Context, token string) (*middleware.PlatformClaims, error) {
		c := &middleware.PlatformClaims{Username: token, Email: token + "@example.org", AccessGroups: middleware.AccessGroups{"/EEG_ADMIN"}}
		switch token {
		case "admin":
			c.Tenants = []string{strings.ToLower(mtTenant)} // compared case-insensitively
		case "admin-noemail":
			c.Tenants, c.Email = []string{mtTenant}, ""
		case "user":
			c.Tenants, c.AccessGroups = []string{mtTenant}, middleware.AccessGroups{"/EEG_USER"}
		case "other":
			c.Tenants = []string{mtOtherTenant}
		}
		return c, nil
	}

	c := &mtClient{t: t, router: mux.NewRouter()}
	s := c.router.PathPrefix("/eeg").Subrouter()
	registerMailTemplateRoutes(s, &MailTemplateHandler{db: db, sendMail: func(tenant, to, subject string, cc *string,
		body *bytes.Buffer, inline []*services.Attachment, att *services.Attachment) error {
		c.sent = append(c.sent, mtSent{tenant, to, subject, cc, body.String(), len(inline), att != nil})
		return nil
	}})
	return c
}

// execTestSQL runs a fixture statement through the Database interface, which has no Exec.
func execTestSQL(ctx context.Context, db database.Database, stmt string, args ...interface{}) (int, error) {
	var rows []int
	err := db.Select(ctx, &rows, stmt+" RETURNING 1", args...)
	return len(rows), err
}

func (c *mtClient) do(token, tenant, method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("tenant", tenant)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	c.router.ServeHTTP(w, req)
	return w
}

func (c *mtClient) json(token, tenant, method, path string, payload interface{}) *httptest.ResponseRecorder {
	// a save names the PDF explicitly, as web always does (null = none); a missing field is a 400
	// (TestMailTemplates_SaveWithoutAttachmentField)
	if m, ok := payload.(map[string]interface{}); ok && method == "PUT" && !strings.Contains(path, "/files") {
		if _, set := m["attachmentFileId"]; !set {
			m["attachmentFileId"] = nil
		}
	}
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		require.NoError(c.t, err)
		body = bytes.NewReader(b)
	}
	return c.do(token, tenant, method, path, body, "application/json")
}

func (c *mtClient) upload(token, tenant, key, kind, filename string, content []byte) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(c.t, mw.WriteField("kind", kind))
	fw, err := mw.CreateFormFile("file", filename)
	require.NoError(c.t, err)
	_, _ = fw.Write(content)
	require.NoError(c.t, mw.Close())
	return c.do(token, tenant, "POST", "/eeg/mailtemplates/"+key+"/files", &buf, mw.FormDataContentType())
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v), w.Body.String())
	return v
}

type mtDTO struct {
	Key                  string  `json:"key"`
	Subject              string  `json:"subject"`
	BodyHtml             string  `json:"bodyHtml"`
	Customized           bool    `json:"customized"`
	Version              int     `json:"version"`
	DefaultVersion       int     `json:"defaultVersion"`
	DefaultChanged       bool    `json:"defaultChanged"`
	DefaultSubject       string  `json:"defaultSubject"`
	DefaultBodyHtml      string  `json:"defaultBodyHtml"`
	LegacyTemplateActive bool    `json:"legacyTemplateActive"`
	AttachmentFileId     *string `json:"attachmentFileId"`
	UpdatedBy            *string
	Placeholders         []struct {
		Key   string `json:"key"`
		Block bool   `json:"block"`
	} `json:"placeholders"`
	Files []struct {
		Id        string `json:"id"`
		Kind      string `json:"kind"`
		Filename  string `json:"filename"`
		MimeType  string `json:"mimeType"`
		Size      int64  `json:"size"`
		Committed bool   `json:"committed"`
	} `json:"files"`
	Limits map[string]int64 `json:"limits"`
}

type mtErrors struct {
	Errors []struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	} `json:"errors"`
}

func logoPNG(t *testing.T) []byte {
	png, err := public.Templates.ReadFile("templates/eegfaktura-logo.png")
	require.NoError(t, err)
	return png
}

var minimalPDF = []byte("%PDF-1.4\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF\n")

func TestMailTemplates_HappyPath(t *testing.T) {
	c := newMailTemplateClient(t)

	w := c.json("admin", mtTenant, "GET", "/eeg/mailtemplates", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	list := decode[[]map[string]interface{}](t, w)
	require.Len(t, list, 3)
	assert.Equal(t, "consent-request", list[0]["key"])
	assert.Equal(t, false, list[0]["customized"])
	assert.Nil(t, list[0]["updatedAt"])

	w = c.json("admin", mtTenant, "GET", "/eeg/mailtemplates/consent-request", nil)
	require.Equal(t, http.StatusOK, w.Code)
	def := decode[mtDTO](t, w)
	assert.False(t, def.Customized)
	assert.Equal(t, 0, def.Version)
	assert.Equal(t, 1, def.DefaultVersion)
	assert.Equal(t, "Aktivierung im Serviceportal", def.Subject)
	assert.Equal(t, def.DefaultBodyHtml, def.BodyHtml)
	assert.Contains(t, def.BodyHtml, `data-ph="eeg.contactBlock"`)
	assert.Empty(t, def.Files)
	assert.Equal(t, int64(10), def.Limits["maxImages"])
	assert.Equal(t, int64(6*1024*1024), def.Limits["maxTotalBytes"])

	// the builtin logo of the default text can be shown by the editor
	w = c.do("admin", mtTenant, "GET", "/eeg/mailtemplates/consent-request/files/eegfaktura-logo", nil, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))

	w = c.upload("admin", mtTenant, "consent-request", "image", `..\..\Logo "neu".png`, logoPNG(t))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	img := decode[map[string]interface{}](t, w)
	imgId := img["id"].(string)
	assert.Equal(t, "image/png", img["mimeType"])
	assert.Equal(t, "Logo neu.png", img["filename"])

	assert.Equal(t, false, img["committed"], "an upload is a draft")

	w = c.upload("admin", mtTenant, "consent-request", "attachment", "anleitung.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	pdfId := decode[map[string]interface{}](t, w)["id"].(string)

	body := `<p>Hallo <span data-ph="member.firstName">Vorname</span>!</p><img src="mailfile:` + imgId + `" alt="Logo"><div data-ph="eeg.contactBlock">x</div>`
	w = c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/consent-request", map[string]interface{}{
		"subject": `Hallo {member.firstName}`, "bodyHtml": body, "version": 0, "attachmentFileId": pdfId})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	saved := decode[mtDTO](t, w)
	assert.True(t, saved.Customized)
	assert.Equal(t, 1, saved.Version)
	require.NotNil(t, saved.AttachmentFileId)
	assert.Equal(t, pdfId, *saved.AttachmentFileId)
	require.Len(t, saved.Files, 2)
	for _, f := range saved.Files {
		assert.True(t, f.Committed, f.Filename)
	}
	require.NotNil(t, saved.UpdatedBy)
	assert.Equal(t, "admin", *saved.UpdatedBy)

	w = c.json("admin", mtTenant, "GET", "/eeg/mailtemplates", nil)
	list = decode[[]map[string]interface{}](t, w)
	assert.Equal(t, true, list[0]["customized"])
	assert.NotNil(t, list[0]["updatedAt"])
	assert.Equal(t, false, list[0]["defaultChanged"])

	w = c.do("admin", mtTenant, "GET", "/eeg/mailtemplates/consent-request/files/"+imgId, nil, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, logoPNG(t), w.Body.Bytes())

	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/preview", map[string]interface{}{
		"subject": "Vorschau {eeg.name}", "bodyHtml": body})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	preview := decode[map[string]string](t, w)
	assert.Equal(t, "Vorschau Mailtest Gemeinschaft", preview["subject"]) // {eeg.name} = designation (25.10.2026)
	assert.Contains(t, preview["html"], "<p>Hallo Max!</p>")
	assert.Contains(t, preview["html"], `src="data:image/png;base64,`)
	assert.Contains(t, preview["html"], "<div>Mailtest Gemeinschaft</div>")
	assert.Contains(t, preview["html"], "<div>T: +43 1 300</div>")

	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/test", map[string]interface{}{
		"recipient": "me", "subject": "Test {member.lastName}", "bodyHtml": body})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "admin@example.org", decode[map[string]string](t, w)["sentTo"])
	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/test", map[string]interface{}{
		"recipient": "eeg", "subject": "Test", "bodyHtml": body})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "eeg-mail-test@example.org; office@example.org", decode[map[string]string](t, w)["sentTo"])

	require.Len(t, c.sent, 2)
	assert.Equal(t, "admin@example.org", c.sent[0].to)
	assert.Equal(t, "[Test] Test Mustermann", c.sent[0].subject)
	assert.Nil(t, c.sent[0].cc, "a test mail has no copy to the community")
	assert.Equal(t, mtTenant, c.sent[0].tenant)
	assert.Contains(t, c.sent[0].body, `src="cid:`+imgId+`"`)
	assert.Equal(t, 1, c.sent[0].inline)
	assert.True(t, c.sent[0].attachment)
	assert.Equal(t, "eeg-mail-test@example.org; office@example.org", c.sent[1].to)

	w = c.json("admin", mtTenant, "DELETE", "/eeg/mailtemplates/consent-request", nil)
	require.Equal(t, http.StatusNoContent, w.Code)
	w = c.json("admin", mtTenant, "GET", "/eeg/mailtemplates/consent-request", nil)
	reset := decode[mtDTO](t, w)
	assert.False(t, reset.Customized)
	assert.Empty(t, reset.Files, "reset deletes the files")
}

// While a hand-made volume template applies, GET says so and the unchanged default previews that template.
func TestMailTemplates_LegacyTemplateActive(t *testing.T) {
	c := newMailTemplateClient(t)
	vol := t.TempDir()
	dir := filepath.Join(vol, mtTenant, "templates")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "activation-mail-template.toml"), []byte(`TemplateFile="t.html"
InlinePictures = [ { Filepath="l.png", contentId="logo-1" } ]`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "t.html"), []byte(`<html><body><p>Servus {{.Participant.FirstName}} von {{.Eeg.Name}}</p><img src="cid:logo-1"></body></html>`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "l.png"), logoPNG(t), 0o644))
	orig := viper.GetString("file-content.templates")
	viper.Set("file-content.templates", vol)
	t.Cleanup(func() { viper.Set("file-content.templates", orig) })

	w := c.json("admin", mtTenant, "GET", "/eeg/mailtemplates/consent-request", nil)
	def := decode[mtDTO](t, w)
	assert.True(t, def.LegacyTemplateActive)
	w = c.json("admin", mtTenant, "GET", "/eeg/mailtemplates/zp-active", nil)
	assert.False(t, decode[mtDTO](t, w).LegacyTemplateActive, "no hand-made template for this mail")

	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/preview", map[string]interface{}{
		"subject": def.DefaultSubject, "bodyHtml": def.DefaultBodyHtml})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	preview := decode[map[string]string](t, w)
	assert.Contains(t, preview["html"], "<p>Servus Max von MAIL-TEST</p>")
	assert.Contains(t, preview["html"], `src="data:image/png;base64,`)
	assert.Equal(t, "Aktivierung im Serviceportal", preview["subject"])

	// the editor's own serialisation of the default (no line breaks) still counts as unchanged
	compact := strings.ReplaceAll(def.DefaultBodyHtml, "\n", "")
	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/preview", map[string]interface{}{
		"subject": " " + def.DefaultSubject + " ", "bodyHtml": compact})
	assert.Contains(t, decode[map[string]string](t, w)["html"], "<p>Servus Max von MAIL-TEST</p>")
	// useDefault asks for it explicitly
	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/preview", map[string]interface{}{
		"subject": "x", "bodyHtml": "<p>x</p>", "useDefault": true})
	assert.Contains(t, decode[map[string]string](t, w)["html"], "<p>Servus Max von MAIL-TEST</p>")

	// the test mail sends the same hand-made template, with its inline image
	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/test", map[string]interface{}{
		"recipient": "me", "subject": def.DefaultSubject, "bodyHtml": compact})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, c.sent, 1)
	assert.Equal(t, "[Test] Aktivierung im Serviceportal", c.sent[0].subject)
	assert.Contains(t, c.sent[0].body, "<p>Servus Max von MAIL-TEST</p>")
	assert.Contains(t, c.sent[0].body, `src="cid:logo-1"`)
	assert.Equal(t, 1, c.sent[0].inline)

	// a changed text previews the new format
	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/preview", map[string]interface{}{
		"subject": def.DefaultSubject, "bodyHtml": "<p>Neu</p>"})
	assert.Contains(t, decode[map[string]string](t, w)["html"], "<p>Neu</p>")

	// the default text with an own PDF or an own image is a change: the preview shows it, not the hand-made template
	w = c.upload("admin", mtTenant, "consent-request", "attachment", "anleitung.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	pdfId := decode[map[string]interface{}](t, w)["id"].(string)
	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/preview", map[string]interface{}{
		"subject": def.DefaultSubject, "bodyHtml": compact, "attachmentFileId": pdfId})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, decode[map[string]string](t, w)["html"], "Servus Max", "own PDF: not the hand-made template")
	w = c.upload("admin", mtTenant, "consent-request", "image", "bild.png", logoPNG(t))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	imgId := decode[map[string]interface{}](t, w)["id"].(string)
	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/preview", map[string]interface{}{
		"subject": def.DefaultSubject, "bodyHtml": compact + `<p><img src="mailfile:` + imgId + `"></p>`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, decode[map[string]string](t, w)["html"], "Servus Max", "own image: not the hand-made template")

	// a saved version replaces the hand-made template
	w = c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/consent-request", map[string]interface{}{
		"subject": "S", "bodyHtml": "<p>x</p>", "version": 0})
	require.Equal(t, http.StatusOK, w.Code)
	w = c.json("admin", mtTenant, "GET", "/eeg/mailtemplates/consent-request", nil)
	assert.False(t, decode[mtDTO](t, w).LegacyTemplateActive)
}

// A global hand-made template on the volume applies to every community without its own one (as the send path
// resolves it), so the editor reports it as active too.
func TestMailTemplates_GlobalLegacyTemplateActive(t *testing.T) {
	c := newMailTemplateClient(t)
	vol := t.TempDir()
	dir := filepath.Join(vol, "templates")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "activation-mail-template.toml"), []byte(`TemplateFile="t.html"`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "t.html"), []byte(`<html><body><p>Global {{.Eeg.Name}}</p></body></html>`), 0o644))
	orig := viper.GetString("file-content.templates")
	viper.Set("file-content.templates", vol)
	t.Cleanup(func() { viper.Set("file-content.templates", orig) })

	w := c.json("admin", mtTenant, "GET", "/eeg/mailtemplates/consent-request", nil)
	assert.True(t, decode[mtDTO](t, w).LegacyTemplateActive)
	w = c.json("admin", mtTenant, "GET", "/eeg/mailtemplates/zp-active", nil)
	assert.False(t, decode[mtDTO](t, w).LegacyTemplateActive)
}

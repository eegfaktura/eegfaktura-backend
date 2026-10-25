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

	// a saved version replaces the hand-made template
	w = c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/consent-request", map[string]interface{}{
		"subject": "S", "bodyHtml": "<p>x</p>", "version": 0})
	require.Equal(t, http.StatusOK, w.Code)
	w = c.json("admin", mtTenant, "GET", "/eeg/mailtemplates/consent-request", nil)
	assert.False(t, decode[mtDTO](t, w).LegacyTemplateActive)
}

// An image the text references must exist when saving: an old draft is kept and
// committed, a vanished one is a 400 instead of being dropped silently.
func TestMailTemplates_SaveReferencedImages(t *testing.T) {
	c := newMailTemplateClient(t)
	db, err := database.GetDB(context.Background())
	require.NoError(t, err)
	upload := func() string {
		w := c.upload("admin", mtTenant, "zp-active", "image", "a.png", logoPNG(t))
		require.Equal(t, http.StatusCreated, w.Code)
		return decode[map[string]interface{}](t, w)["id"].(string)
	}
	old, gone := upload(), upload()
	_, err = execTestSQL(context.Background(), db,
		`UPDATE base.mail_template_file SET created_at = now() - interval '25 hours' WHERE id = $1`, old)
	require.NoError(t, err)

	w := c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/zp-active", map[string]interface{}{
		"subject": "S", "bodyHtml": `<p>x</p><img src="mailfile:` + old + `">`, "version": 0})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	saved := decode[mtDTO](t, w)
	assert.Contains(t, saved.BodyHtml, "mailfile:"+old)
	found := false
	for _, f := range saved.Files {
		if f.Id == old {
			found = true
			assert.True(t, f.Committed, "a referenced old draft is committed, not purged")
		}
	}
	assert.True(t, found)

	w = c.do("admin", mtTenant, "DELETE", "/eeg/mailtemplates/zp-active/files/"+gone, nil, "")
	require.Equal(t, http.StatusNoContent, w.Code)
	w = c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/zp-active", map[string]interface{}{
		"subject": "S", "bodyHtml": `<p>x</p><img src="mailfile:` + gone + `">`, "version": 1})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	errs := decode[mtErrors](t, w)
	assert.Equal(t, "bodyHtml", errs.Errors[0].Field)
	assert.Equal(t, "Ein Bild im Text gibt es nicht (mehr); bitte erneut einfügen.", errs.Errors[0].Message)
}

func TestMailTemplates_FileLifecycle(t *testing.T) {
	c := newMailTemplateClient(t)
	upload := func(kind string, content []byte) string {
		w := c.upload("admin", mtTenant, "zp-active", kind, "f", content)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		return decode[map[string]interface{}](t, w)["id"].(string)
	}
	committed := func() map[string]bool {
		w := c.json("admin", mtTenant, "GET", "/eeg/mailtemplates/zp-active", nil)
		out := map[string]bool{}
		for _, f := range decode[mtDTO](t, w).Files {
			out[f.Id] = f.Committed
		}
		return out
	}
	img1, img2, other := upload("image", logoPNG(t)), upload("image", logoPNG(t)), upload("image", logoPNG(t))
	pdf := upload("attachment", minimalPDF)

	w := c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/zp-active", map[string]interface{}{
		"subject": "S", "bodyHtml": `<p>x</p><img src="mailfile:` + img1 + `">`, "version": 0, "attachmentFileId": pdf})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, map[string]bool{img1: true, img2: false, other: false, pdf: true}, committed(),
		"used files are committed; fresh drafts (also of other editors) stay")

	// a file the saved version uses can not be deleted; a draft can
	w = c.do("admin", mtTenant, "DELETE", "/eeg/mailtemplates/zp-active/files/"+img1, nil, "")
	assert.Equal(t, http.StatusConflict, w.Code)
	w = c.do("admin", mtTenant, "DELETE", "/eeg/mailtemplates/zp-active/files/"+pdf, nil, "")
	assert.Equal(t, http.StatusConflict, w.Code)
	w = c.do("admin", mtTenant, "DELETE", "/eeg/mailtemplates/zp-active/files/"+other, nil, "")
	assert.Equal(t, http.StatusNoContent, w.Code)

	// the next version uses img2 and no PDF: img1 and the PDF go
	w = c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/zp-active", map[string]interface{}{
		"subject": "S", "bodyHtml": `<p>x</p><img src="mailfile:` + img2 + `">`, "version": 1, "attachmentFileId": nil})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Nil(t, decode[mtDTO](t, w).AttachmentFileId)
	assert.Equal(t, map[string]bool{img2: true}, committed())

	// referencing a PDF that does not exist (or an image as PDF) → 400
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", img2, "not-a-uuid"} {
		w = c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/zp-active", map[string]interface{}{
			"subject": "S", "bodyHtml": `<p>x</p>`, "version": 2, "attachmentFileId": id})
		require.Equal(t, http.StatusBadRequest, w.Code, id)
		assert.Equal(t, "attachmentFileId", decode[mtErrors](t, w).Errors[0].Field)
	}
}

func TestMailTemplates_Validation(t *testing.T) {
	c := newMailTemplateClient(t)
	put := func(payload map[string]interface{}) *httptest.ResponseRecorder {
		return c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/consent-request", payload)
	}

	cases := map[string]struct {
		payload map[string]interface{}
		field   string
	}{
		"unknown placeholder":    {map[string]interface{}{"subject": "S", "bodyHtml": `<p><span data-ph="member.iban">x</span></p>`}, "bodyHtml"},
		"placeholder wrong mail": {map[string]interface{}{"subject": "S", "bodyHtml": `<p><span data-ph="reason">x</span></p>`}, "bodyHtml"},
		"empty body":             {map[string]interface{}{"subject": "S", "bodyHtml": `<script>alert(1)</script>`}, "bodyHtml"},
		"empty subject":          {map[string]interface{}{"subject": " ", "bodyHtml": `<p>x</p>`}, "subject"},
		"unknown subject token":  {map[string]interface{}{"subject": "Hallo {member.iban}", "bodyHtml": `<p>x</p>`}, "subject"},
		"block in subject":       {map[string]interface{}{"subject": "{eeg.contactBlock}", "bodyHtml": `<p>x</p>`}, "subject"},
		"block in paragraph":     {map[string]interface{}{"subject": "S", "bodyHtml": `<p>Gruß <span data-ph="eeg.contactBlock">x</span></p>`}, "bodyHtml"},
		"subject too long":       {map[string]interface{}{"subject": strings.Repeat("x", 201), "bodyHtml": `<p>x</p>`}, "subject"},
		"body too large":         {map[string]interface{}{"subject": "S", "bodyHtml": "<p>" + strings.Repeat("x", 100*1024) + "</p>"}, "bodyHtml"},
		"negative version":       {map[string]interface{}{"subject": "S", "bodyHtml": `<p>x</p>`, "version": -1}, "version"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			w := put(tc.payload)
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			errs := decode[mtErrors](t, w)
			require.NotEmpty(t, errs.Errors)
			assert.Equal(t, tc.field, errs.Errors[0].Field)
		})
	}
	w := c.do("admin", mtTenant, "PUT", "/eeg/mailtemplates/consent-request", strings.NewReader("{no json"), "application/json")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// unsafe HTML is removed, not rejected
	w = put(map[string]interface{}{"subject": "S", "bodyHtml": `<p onclick="x()">a <a href="javascript:x()">b</a> <img src="https://t.example/p.gif"></p>`})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, `<p>a b </p>`, decode[mtDTO](t, w).BodyHtml)

	// uploads: type checked on the content, limits per mail
	w = c.upload("admin", mtTenant, "zp-active", "image", "fake.png", []byte("this is not an image"))
	assert.Equal(t, http.StatusBadRequest, w.Code, "a disguised file is rejected")
	w = c.upload("admin", mtTenant, "zp-active", "image", "doc.png", minimalPDF)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = c.upload("admin", mtTenant, "zp-active", "attachment", "img.pdf", logoPNG(t))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = c.upload("admin", mtTenant, "zp-active", "video", "a.png", logoPNG(t))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	big := append(append([]byte{}, logoPNG(t)...), make([]byte, 1024*1024)...)
	w = c.upload("admin", mtTenant, "zp-active", "image", "big.png", big)
	assert.Equal(t, http.StatusBadRequest, w.Code, "image over 1 MB")

	w = c.upload("admin", mtTenant, "zp-active", "attachment", "a.pdf", minimalPDF)
	require.Equal(t, http.StatusCreated, w.Code)
	w = c.upload("admin", mtTenant, "zp-active", "attachment", "b.pdf", minimalPDF)
	assert.Equal(t, http.StatusBadRequest, w.Code, "one PDF per mail")
	for i := 0; i < 10; i++ {
		w = c.upload("admin", mtTenant, "zp-active", "image", "a.png", logoPNG(t))
		require.Equal(t, http.StatusCreated, w.Code)
	}
	w = c.upload("admin", mtTenant, "zp-active", "image", "a.png", logoPNG(t))
	assert.Equal(t, http.StatusBadRequest, w.Code, "at most 10 images")

	// test mail: only "me" or "eeg", never a free address
	body := map[string]interface{}{"recipient": "someone@example.org", "subject": "S", "bodyHtml": "<p>x</p>"}
	w = c.json("admin", mtTenant, "POST", "/eeg/mailtemplates/consent-request/test", body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	body["recipient"] = "me"
	w = c.json("admin-noemail", mtTenant, "POST", "/eeg/mailtemplates/consent-request/test", body)
	assert.Equal(t, http.StatusBadRequest, w.Code, "login without e-mail")
	assert.Empty(t, c.sent)

	w = c.json("admin", mtTenant, "GET", "/eeg/mailtemplates/billing-document", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
	w = c.do("admin", mtTenant, "GET", "/eeg/mailtemplates/zp-active/files/not-a-uuid", nil, "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMailTemplates_VersionConflict(t *testing.T) {
	c := newMailTemplateClient(t)
	payload := map[string]interface{}{"subject": "S", "bodyHtml": "<p>x</p>", "version": 0}
	w := c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/zp-active", payload)
	require.Equal(t, http.StatusOK, w.Code)

	w = c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/zp-active", payload)
	require.Equal(t, http.StatusConflict, w.Code)
	conflict := decode[map[string]interface{}](t, w)
	assert.Equal(t, float64(1), conflict["currentVersion"])
	assert.NotEmpty(t, conflict["message"])

	payload["version"] = 1
	w = c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/zp-active", payload)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 2, decode[mtDTO](t, w).Version)
	w = c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/zp-active", payload)
	assert.Equal(t, http.StatusConflict, w.Code, "stale version")
}

func TestMailTemplates_TenantIsolationAndRole(t *testing.T) {
	c := newMailTemplateClient(t)
	w := c.upload("admin", mtTenant, "zp-active", "image", "a.png", logoPNG(t))
	require.Equal(t, http.StatusCreated, w.Code)
	imgId := decode[map[string]interface{}](t, w)["id"].(string)
	w = c.json("admin", mtTenant, "PUT", "/eeg/mailtemplates/zp-active", map[string]interface{}{
		"subject": "S", "bodyHtml": `<p>x</p><img src="mailfile:` + imgId + `">`, "version": 0})
	require.Equal(t, http.StatusOK, w.Code)

	// an admin of another community can not use this tenant header
	for _, r := range []struct{ method, path string }{
		{"GET", "/eeg/mailtemplates"},
		{"GET", "/eeg/mailtemplates/zp-active"},
		{"PUT", "/eeg/mailtemplates/zp-active"},
		{"DELETE", "/eeg/mailtemplates/zp-active"},
		{"GET", "/eeg/mailtemplates/zp-active/files/" + imgId},
		{"DELETE", "/eeg/mailtemplates/zp-active/files/" + imgId},
		{"POST", "/eeg/mailtemplates/zp-active/preview"},
		{"POST", "/eeg/mailtemplates/zp-active/test"},
	} {
		w = c.json("other", mtTenant, r.method, r.path, map[string]interface{}{})
		assert.Equal(t, http.StatusForbidden, w.Code, r.method+" "+r.path)
		// a login without the EEG_ADMIN group gets no access (existing Protect: 401)
		w = c.json("user", mtTenant, r.method, r.path, map[string]interface{}{})
		assert.Equal(t, http.StatusUnauthorized, w.Code, r.method+" "+r.path)
	}

	// with its own tenant header it can not reach the file by a guessed id
	w = c.do("other", mtOtherTenant, "GET", "/eeg/mailtemplates/zp-active/files/"+imgId, nil, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
	w = c.do("other", mtOtherTenant, "DELETE", "/eeg/mailtemplates/zp-active/files/"+imgId, nil, "")
	assert.Equal(t, http.StatusForbidden, w.Code)
	// nor reference it in its own mail
	w = c.json("other", mtOtherTenant, "PUT", "/eeg/mailtemplates/zp-active", map[string]interface{}{
		"subject": "S", "bodyHtml": `<p>x</p><img src="mailfile:` + imgId + `">`, "version": 0})
	require.Equal(t, http.StatusBadRequest, w.Code, "an image of another community does not exist for this one")
	assert.NotContains(t, w.Body.String(), mtTenant)
	w = c.json("other", mtOtherTenant, "GET", "/eeg/mailtemplates/zp-active", nil)
	assert.False(t, decode[mtDTO](t, w).Customized, "nothing saved")
	// the file of a mail is not served under another mail key
	w = c.do("admin", mtTenant, "GET", "/eeg/mailtemplates/consent-request/files/"+imgId, nil, "")
	assert.Equal(t, http.StatusNotFound, w.Code)

	// the file is untouched
	w = c.do("admin", mtTenant, "GET", "/eeg/mailtemplates/zp-active/files/"+imgId, nil, "")
	assert.Equal(t, http.StatusOK, w.Code)
}

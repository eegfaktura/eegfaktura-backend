package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"at.ourproject/vfeeg-backend/mailtemplate"
	"at.ourproject/vfeeg-backend/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mailfileRef = regexp.MustCompile(`mailfile:([A-Za-z0-9-]+)`)

// The ten hand-made templates in public/<tenant>/templates: every one converts
// or stops with a clear finding.
func TestConvertTheTenHandMadeTemplates(t *testing.T) {
	blocked := map[string]string{
		"CC100438": "Logo_4c_cmyk.jpg", // 2.3 MB logo, must be made smaller first
	}
	entries, err := os.ReadDir("../../public")
	require.NoError(t, err)
	count := 0
	for _, e := range entries {
		dir := filepath.Join("../../public", e.Name(), "templates")
		if e.Name() == "templates" || !e.IsDir() {
			continue
		}
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		count++
		t.Run(e.Name(), func(t *testing.T) {
			convs := ConvertTenant(os.DirFS(dir), e.Name())
			require.Len(t, convs, 1)
			c := convs[0]
			assert.Equal(t, mailtemplate.KeyConsentRequest, c.Key)
			if want, ok := blocked[e.Name()]; ok {
				require.True(t, c.Blocked())
				assert.Contains(t, findingTexts(c), want)
				return
			}
			require.False(t, c.Blocked(), findingTexts(c))
			assert.NotContains(t, c.Body, "{{")
			assert.NotContains(t, c.Body, "}}")
			assert.NotContains(t, c.Body, "cid:")
			assert.Equal(t, "Aktivierung im Serviceportal", c.Subject)

			raw, err := os.ReadFile(filepath.Join(dir, "AktivierungsEmail-template.html"))
			require.NoError(t, err)
			if strings.Contains(string(raw), "{{.Participant.FirstName}}") {
				assert.Contains(t, c.Body, `data-ph="member.firstName"`)
			}
			if strings.Contains(string(raw), "range .Meteringpoints") {
				assert.Contains(t, c.Body, `data-ph="meteringPoints"`)
			}

			// every image reference is an upload of this mail or the built-in logo
			ids := map[string]bool{mailtemplate.BuiltinLogoId: true}
			for _, f := range c.Files {
				ids[f.PreId] = true
			}
			for _, m := range mailfileRef.FindAllStringSubmatch(c.Body, -1) {
				assert.True(t, ids[m[1]], m[0])
			}

			// what the import saves renders with sample data
			_, err = mailtemplate.Render(mailtemplate.Lookup(c.Key), c.Subject, c.Body, mailtemplate.SampleData(nil), uploadsAsFiles(c), mailtemplate.ImagesDataURI)
			assert.NoError(t, err)
		})
	}
	assert.Equal(t, 10, count)
}

func TestConvert_RC101162KeepsHeaderAndPdf(t *testing.T) {
	c := ConvertTenant(os.DirFS("../../public/RC101162/templates"), "RC101162")[0]
	require.False(t, c.Blocked())
	require.Len(t, c.Files, 2)
	assert.Equal(t, "image", c.Files[0].Kind)
	assert.Equal(t, "attachment", c.Files[1].Kind)
	assert.True(t, strings.HasPrefix(c.Files[1].Filename, "Anleitung Zusatzvereinbarung Netz O"), c.Files[1].Filename)
	assert.True(t, strings.HasSuffix(c.Files[1].Filename, ".pdf"))
}

func templateFS(html string, extra ...string) fstest.MapFS {
	fs := fstest.MapFS{
		"activation-mail-template.toml": {Data: []byte(`TemplateFile="t.html"` + "\n" + strings.Join(extra, "\n"))},
		"t.html":                        {Data: []byte("<html><body>" + html + "</body></html>")},
	}
	return fs
}

func TestConvert_Actions(t *testing.T) {
	c := ConvertTenant(templateFS(`<p>Hallo {{.Participant.FirstName}} {{ .Participant.LastName }}</p>
		{{if .Eeg.Contact.Phone.Valid}}<div>T: {{.Eeg.Contact.Phone.String}}</div>{{end}}
		<p>x</p>
		{{if .Eeg.ContactPerson.Valid}}<div>{{.Eeg.ContactPerson.String}}</div>{{end}}
		{{if .Eeg.Website.Valid}}<div>{{.Eeg.Website.String}}</div>{{end}}`), "TE000300")[0]
	require.False(t, c.Blocked(), findingTexts(c))
	assert.Contains(t, c.Body, `Hallo <span data-ph="member.firstName">Vorname</span> <span data-ph="member.lastName">Nachname</span>`)
	assert.Contains(t, c.Body, `T: <span data-ph="eeg.phone">Telefon der EEG</span><br/>`, "a single optional line keeps its placeholder")
	assert.Equal(t, 1, strings.Count(c.Body, `data-ph="eeg.contactBlock"`), "consecutive optional lines become one contact block")
	assert.NotContains(t, c.Body, `eeg.website`)
}

func TestConvert_UnmappableStops(t *testing.T) {
	for name, tpl := range map[string]string{
		"unknown field":       `<p>{{.Participant.Contact.Email.String}}</p>`,
		"condition":           `<p>{{if .Participant.FirstName}}Hallo{{end}}</p>`,
		"range other":         `<p>{{range .Participant.MeteringPoint}}{{.MeteringPoint}}{{end}}</p>`,
		"contact person bare": `<p>{{.Eeg.ContactPerson.String}}</p>`,
		"wrong mail key":      `<p>{{.Reason}}</p>`,
		"unclosed":            `<p>{{.Participant.FirstName</p>`,
		"in href":             `<p><a href="mailto:{{.Eeg.Contact.Email.String}}">Mail</a></p>`,
		"in img src":          `<p>x</p><img src="https://x.example/{{.Eeg.Name}}.png">`,
		"in style":            `<p style="color: {{.Eeg.Name}}">x</p>`,
	} {
		t.Run(name, func(t *testing.T) {
			c := ConvertTenant(templateFS(tpl), "TE000300")[0]
			assert.True(t, c.Blocked(), name)
			assert.Empty(t, c.Body)
			assert.NotContains(t, findingTexts(c), "data-ph")
		})
	}
}

func TestConvert_ActionInAttributeIsNamed(t *testing.T) {
	c := ConvertTenant(templateFS(`<p>Hallo {{.Participant.FirstName}}, <a href="mailto:{{.Eeg.Contact.Email.String}}">schreib uns</a></p>`), "TE000300")[0]
	require.True(t, c.Blocked())
	assert.Contains(t, findingTexts(c), `href="mailto:{{.Eeg.Contact.Email.String}}"`)
}

func TestConvert_Images(t *testing.T) {
	png, err := os.ReadFile("../../public/templates/eegfaktura-logo.png") // 528x142
	require.NoError(t, err)
	fs := templateFS(`<p>a</p><img src="cid:logo" style="max-height: 71px"/><img src="cid:logo"/><img src="cid:eegfaktura-logo-1"/><img src="cid:missing"/>`,
		`InlinePictures = [ { Filepath="l.png", contentId="logo" }, { Filepath="eegfaktura-logo.png", contentId="eegfaktura-logo-1" }, { Filepath="u.png", contentId="unused" } ]`)
	fs["l.png"] = &fstest.MapFile{Data: png}
	c := ConvertTenant(fs, "TE000300")[0]
	require.False(t, c.Blocked(), findingTexts(c))
	require.Len(t, c.Files, 1, "an image used twice is uploaded once, the eegfaktura logo is built in")
	assert.Contains(t, c.Body, `<img src="mailfile:`+c.Files[0].PreId+`" width="264"/>`)
	assert.Contains(t, c.Body, `src="mailfile:eegfaktura-logo"`)
	assert.Contains(t, findingTexts(c), "cid:missing")
	assert.Contains(t, findingTexts(c), "cid:unused")

	fs["l.png"] = &fstest.MapFile{Data: []byte("not an image")}
	assert.True(t, ConvertTenant(fs, "TE000300")[0].Blocked(), "a disguised image stops the import")
}

// The import uploads the files, swaps the ids and saves; an error after an upload resets the mail.
func TestImportMail(t *testing.T) {
	png, err := os.ReadFile("../../public/templates/eegfaktura-logo.png")
	require.NoError(t, err)
	fs := templateFS(`<p>Hallo {{.Participant.FirstName}}</p><img src="cid:logo"/>`, `InlinePictures = [ { Filepath="l.png", contentId="logo" } ]`)
	fs["l.png"] = &fstest.MapFile{Data: png}
	c := ConvertTenant(fs, "TE000300")[0]
	require.False(t, c.Blocked())

	var calls []string
	var saved map[string]interface{}
	failPut := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		assert.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		assert.Equal(t, "TE000300", r.Header.Get("tenant"))
		switch {
		case r.Method == "GET":
			_, _ = io.WriteString(w, `{"customized":false,"files":[]}`)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/files"):
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"id":"11111111-2222-3333-4444-555555555555"}`)
		case r.Method == "PUT":
			if failPut {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&saved)
			_, _ = io.WriteString(w, `{}`)
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/test"):
			_, _ = io.WriteString(w, `{"sentTo":"eeg@example.org"}`)
		case r.Method == "DELETE":
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	client := &apiClient{base: srv.URL, token: "secret", http: srv.Client()}

	require.NoError(t, client.importMail(c, false, "eeg"))
	assert.Equal(t, []string{"GET /eeg/mailtemplates/consent-request", "POST /eeg/mailtemplates/consent-request/files",
		"PUT /eeg/mailtemplates/consent-request", "POST /eeg/mailtemplates/consent-request/test"}, calls)
	assert.Contains(t, saved["bodyHtml"], "mailfile:11111111-2222-3333-4444-555555555555")
	assert.NotContains(t, saved["bodyHtml"], c.Files[0].PreId)
	assert.Equal(t, float64(0), saved["version"])

	calls, failPut = nil, true
	assert.Error(t, client.importMail(c, false, ""))
	assert.Equal(t, "DELETE /eeg/mailtemplates/consent-request", calls[len(calls)-1], "a failed import is reset")
}

func findingTexts(c *Conversion) string {
	var out []string
	for _, f := range c.Findings {
		out = append(out, f.Text)
	}
	return strings.Join(out, "\n")
}

func uploadsAsFiles(c *Conversion) []model.MailTemplateFile {
	var out []model.MailTemplateFile
	for _, f := range c.Files {
		out = append(out, model.MailTemplateFile{Id: f.PreId, Kind: f.Kind, Filename: f.Filename, MimeType: f.MimeType, Content: f.Content})
	}
	return out
}

package parser

import (
	"os"
	"strings"
	"testing"

	"at.ourproject/vfeeg-backend/config"
	"at.ourproject/vfeeg-backend/model"
	"github.com/spf13/viper"
	"gopkg.in/guregu/null.v4"
)

// Regression guard for the zp-complete (Zählpunkt-aktiv) mail: the template
// references {{.MeteringPoint}}, which must be provided by the template data
// struct built in sendMailFromTemplate. If the field is dropped again the
// render fails with "can't evaluate field MeteringPoint" and the mail is lost.
func TestParseTemplateZpCompleteMeteringPoint(t *testing.T) {
	viper.Set("file-content.templates", "../public")

	eeg := &model.Eeg{
		Name:          "TE-EEG",
		Description:   "TEST EEG",
		ContactPerson: null.StringFrom("Max Sonnenmann"),
		Contact:       model.Contact{Phone: null.StringFrom("123456789")},
	}
	participant := &model.EegParticipant{
		EegParticipantBase: model.EegParticipantBase{FirstName: "Max"},
		Contact:            model.ContactInfo{Email: null.StringFrom("my@mail.com")},
	}
	meter := "AT0010000000000000000000000111"

	data := struct {
		Eeg            *model.Eeg
		Participant    *model.EegParticipant
		Meteringpoints []string
		MeteringPoint  string
	}{eeg, participant, []string{meter}, meter}

	buf, err := ParseTemplate(os.DirFS("../public/templates"), "zp-complete-mail-template.html", data)
	if err != nil {
		t.Fatalf("zp-complete template must render without error, got: %v", err)
	}
	if !strings.Contains(buf.String(), meter) {
		t.Errorf("rendered zp-complete mail should contain the metering point %q; got:\n%s", meter, buf.String())
	}
}

// TestZpCompleteMailRendersFromEmbeddedDefault proves the hybrid-embed
// behaviour: with nothing seeded on the data volume, the zp-complete
// ("Zählpunkt aktiv") mail — config, template and inline logo — still resolves
// from the defaults embedded in the binary. This is what lets a fresh
// deployment send the mail without hand-seeding the PVC.
func TestZpCompleteMailRendersFromEmbeddedDefault(t *testing.T) {
	viper.Set("file-content.templates", t.TempDir()) // empty: no template on the volume

	fsys, source := resolveTemplateSource("any-tenant", "zp-complete-mail-template.toml")
	if source != "embedded" {
		t.Fatalf("expected the embedded defaults to be used, got source %q", source)
	}

	cfg, err := config.ReadActivationMailTemplateConfig(fsys, "zp-complete-mail-template.toml")
	if err != nil {
		t.Fatalf("read embedded zp-complete config: %v", err)
	}

	meter := "AT0010000000000000000000000111"
	eeg := &model.Eeg{
		Name:        "TE-EEG",
		Description: "TEST EEG",
		Contact:     model.Contact{Phone: null.StringFrom("123456789")},
	}
	participant := &model.EegParticipant{
		EegParticipantBase: model.EegParticipantBase{FirstName: "Max"},
		Contact:            model.ContactInfo{Email: null.StringFrom("my@mail.com")},
	}
	data := struct {
		Eeg            *model.Eeg
		Participant    *model.EegParticipant
		Meteringpoints []string
		MeteringPoint  string
	}{eeg, participant, []string{meter}, meter}

	buf, err := ParseTemplate(fsys, cfg.TemplateFile, data)
	if err != nil {
		t.Fatalf("render embedded zp-complete template: %v", err)
	}
	if body := buf.String(); !strings.Contains(body, meter) || !strings.Contains(body, "dein Zählpunkt") {
		t.Errorf("rendered mail missing expected content; got:\n%s", body)
	}

	inline := buildInlineContent(fsys, cfg.InlinePictures)
	if len(inline) != 1 || inline[0].Filecontent.Len() == 0 {
		t.Errorf("expected one non-empty inline logo from the embed, got %d", len(inline))
	}
}

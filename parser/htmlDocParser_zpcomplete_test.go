package parser

import (
	"strings"
	"testing"

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

	buf, err := ParseTemplate("../public/templates/zp-complete-mail-template.html", data)
	if err != nil {
		t.Fatalf("zp-complete template must render without error, got: %v", err)
	}
	if !strings.Contains(buf.String(), meter) {
		t.Errorf("rendered zp-complete mail should contain the metering point %q; got:\n%s", meter, buf.String())
	}
}

package eda

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/logging/logtest"
	"at.ourproject/vfeeg-backend/model"
	"at.ourproject/vfeeg-backend/services"
	"github.com/jjeffery/civil"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// platform#238: a deregistration by the community (AUFHEBUNG_CCMS) leaves the metering point as it is; the grid
// operator's answer decides. 176, and the rejections 177/180, set it INACTIVE since the consent end of the request;
// any other answer leaves it. The member gets the mail on 176 only.
func TestRevokeOnlyAfterAnswer(t *testing.T) {
	requested := civil.DateFor(2026, 10, 15)
	requestedMs := time.Date(2026, 10, 15, 0, 0, 0, 0, time.Local).UnixMilli()

	var mails atomic.Int32
	orig := sendInactiveMail
	sendInactiveMail = func(string, string, string, *string, *bytes.Buffer, []*services.Attachment, *services.Attachment) error {
		mails.Add(1)
		return nil
	}
	t.Cleanup(func() { sendInactiveMail = orig })

	// the same metering point number in another community must stay untouched by every answer
	const other = "TR238099"
	ecId := func(tenant string) string { return "AT00300000000" + tenant + "000000000001" }
	const meter = "AT0030000000000000000000000238001"
	inboundFixture(t, other, other, ecId(other), meter, 238099, 1)

	handle := func(tenant string, code model.EbMsMessageType, conversation, body string) {
		msg := model.SubscribeMessage{MessageCode: code, Protocol: model.CM_REV_SP, Tenant: strings.ToLower(tenant)}
		require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"conversationId":"%s","sender":"AT003000","receiver":"%s",
			"messageCode":"%s","ecId":"%s",%s}`, conversation, tenant, code, ecId(tenant), body)), &msg.Payload))
		protocolCmRevImpHandler(context.Background(), msg)
		flushMails(t)
	}
	// request is eda-xp's echo of the sent deregistration
	request := func(tenant, conversation string) {
		handle(tenant, model.EBMS_AUFHEBUNG_CCMS, conversation, fmt.Sprintf(
			`"meter":{"meteringPoint":"%s","consentId":"C-S4"},"consentEnd":%d`, meter, requestedMs))
	}
	// answer as eda-xp delivers it: codes in responseData, consentEnd copied from the stored request (0: none)
	answer := func(tenant string, code model.EbMsMessageType, conversation, codes string, consentEnd int64) {
		body := fmt.Sprintf(`"responseData":[{"meteringPoint":"%s","responseCode":[%s],"consentId":"C-S4"}]`, meter, codes)
		if consentEnd != 0 {
			body += fmt.Sprintf(`,"consentEnd":%d`, consentEnd)
		}
		handle(tenant, code, conversation, body)
	}
	type state struct{ status, process, inactiveSince string }
	stateOf := func(tenant string) state {
		var s state
		require.NoError(t, testDB.DbInstance.QueryRow(`SELECT status, process_state, to_char(inactivesince, 'YYYY-MM-DD')
			FROM base.meteringpoint WHERE tenant = $1 AND metering_point_id = $2`, tenant, meter).
			Scan(&s.status, &s.process, &s.inactiveSince))
		return s
	}
	unchanged := state{"ACTIVE", "INIT", "2999-12-31"}
	inactiveSince := func(d civil.Date) state { return state{"INACTIVE", "INACTIVE", d.String()} }
	// lastNotification is the type and codes of the newest notification of the tenant
	lastNotification := func(tenant string) (string, []string) {
		var raw string
		require.NoError(t, testDB.DbInstance.Get(&raw, `SELECT notification FROM base.notification WHERE tenant = $1
			ORDER BY id DESC LIMIT 1`, tenant))
		var n struct {
			Type          string   `json:"type"`
			ResponseCodes []string `json:"responseCodes"`
		}
		require.NoError(t, json.Unmarshal([]byte(raw), &n))
		return n.Type, n.ResponseCodes
	}
	codeTexts := func(codes string) []string {
		texts := []string{}
		for _, c := range strings.Split(codes, ",") {
			if c == "" {
				continue
			}
			n, err := strconv.Atoi(c)
			require.NoError(t, err)
			texts = append(texts, ECON_RESPONSE_CODES[int16(n)])
		}
		return texts
	}

	cases := []struct {
		name       string
		code       model.EbMsMessageType
		codes      string
		noRequest  bool  // the request is missing from the history
		want       state // after the answer
		notifies   bool
		mailsAfter int32
	}{
		{"confirmation 176", model.EBMS_ANTWORT_CCMS, "176", false, inactiveSince(requested), true, 1},
		{"received 99", model.EBMS_ANTWORT_CCMS, "99", false, unchanged, false, 0},
		{"rejection 177 (no data release)", model.EBMS_ABLEHNUNG_CCMS, "177", false, inactiveSince(requested), true, 0},
		{"rejection 180 (consent expired)", model.EBMS_ABLEHNUNG_CCMS, "180", false, inactiveSince(requested), true, 0},
		{"rejection 82 (process date wrong)", model.EBMS_ABLEHNUNG_CCMS, "82", false, unchanged, true, 0},
		{"rejection 187 (consent and metering point do not match)", model.EBMS_ABLEHNUNG_CCMS, "187", false, unchanged, true, 0},
		{"rejection with another code", model.EBMS_ABLEHNUNG_CCMS, "37", false, unchanged, true, 0},
		{"rejection without code", model.EBMS_ABLEHNUNG_CCMS, "", false, unchanged, true, 0},
		{"176 without the request in the history: date of the answer", model.EBMS_ANTWORT_CCMS, "176", true, inactiveSince(civil.Today()), true, 1},
		{"177 without the request in the history: date of the answer", model.EBMS_ABLEHNUNG_CCMS, "177", true, inactiveSince(civil.Today()), true, 0},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tenant := fmt.Sprintf("TR2380%02d", i)
			conversation := "RC238" + tenant
			inboundFixture(t, tenant, tenant, ecId(tenant), meter, 238000+i, 1)
			mails.Store(0)

			if !c.noRequest {
				request(tenant, conversation)
				assert.Equal(t, unchanged, stateOf(tenant), "sending the deregistration keeps the metering point")
				assert.Equal(t, 1, historyCount(t, tenant, conversation))
				typ, _ := lastNotification(tenant)
				assert.Equal(t, string(model.EBMS_AUFHEBUNG_CCMS), typ)
			}
			before := notificationCount(t, tenant)
			logged := logtest.Capture(t, func(w io.Writer) { logrus.SetOutput(w) })

			// eda-xp copies the request's consent end into the answer; without the request it has none
			consentEnd := requestedMs
			if c.noRequest {
				consentEnd = 0
			}
			answer(tenant, c.code, conversation, c.codes, consentEnd)

			assert.Equal(t, c.want, stateOf(tenant))
			assert.Equal(t, unchanged, stateOf(other), "the other community keeps its metering point")
			assert.Equal(t, c.mailsAfter, mails.Load())
			if c.notifies {
				require.Equal(t, before+1, notificationCount(t, tenant))
				typ, codes := lastNotification(tenant)
				assert.Equal(t, string(c.code), typ)
				assert.Equal(t, codeTexts(c.codes), codes)
			} else {
				assert.Equal(t, before, notificationCount(t, tenant))
			}
			if c.noRequest {
				assert.Equal(t, 1, historyCount(t, tenant, conversation))
				assert.Contains(t, logged.String(), "event.action=eda.revoke.request_not_found")
			} else {
				assert.Equal(t, 2, historyCount(t, tenant, conversation))
			}
			if c.want != unchanged {
				out := logged.String()
				assert.Contains(t, out, "event.action=eda.revoke.inactive")
				assert.Contains(t, out, "(conversation "+conversation+")")
				assert.NotContains(t, out, meter, "no metering point in the status log")
			}
		})
	}

	t.Run("the same rejection 177 twice: no error, stays inactive", func(t *testing.T) {
		const tenant, conversation = "TR238050", "RC238TR238050"
		inboundFixture(t, tenant, tenant, ecId(tenant), meter, 238050, 1)
		request(tenant, conversation)
		answer(tenant, model.EBMS_ABLEHNUNG_CCMS, conversation, "177", requestedMs)
		answer(tenant, model.EBMS_ABLEHNUNG_CCMS, conversation, "177", requestedMs)
		assert.Equal(t, inactiveSince(requested), stateOf(tenant))
		assert.Equal(t, 3, historyCount(t, tenant, conversation))
	})

	t.Run("answer with another date than the request: the requested date, the difference is logged", func(t *testing.T) {
		const tenant, conversation = "TR238051", "RC238TR238051"
		inboundFixture(t, tenant, tenant, ecId(tenant), meter, 238051, 1)
		request(tenant, conversation)
		logged := logtest.Capture(t, func(w io.Writer) { logrus.SetOutput(w) })
		answer(tenant, model.EBMS_ANTWORT_CCMS, conversation, "176",
			time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local).UnixMilli())
		assert.Equal(t, inactiveSince(requested), stateOf(tenant))
		assert.Contains(t, logged.String(), "event.action=eda.revoke.date_mismatch")
	})

	t.Run("a request of another metering point in the conversation: date of the answer", func(t *testing.T) {
		const tenant, conversation = "TR238052", "RC238TR238052"
		inboundFixture(t, tenant, tenant, ecId(tenant), meter, 238052, 1)
		handle(tenant, model.EBMS_AUFHEBUNG_CCMS, conversation, fmt.Sprintf(
			`"meter":{"meteringPoint":"AT0030000000000000000000000238999","consentId":"C-X"},"consentEnd":%d`, requestedMs))
		require.Equal(t, 1, historyCount(t, tenant, conversation))
		answer(tenant, model.EBMS_ABLEHNUNG_CCMS, conversation, "180", requestedMs)
		assert.Equal(t, inactiveSince(civil.Today()), stateOf(tenant))
	})
}

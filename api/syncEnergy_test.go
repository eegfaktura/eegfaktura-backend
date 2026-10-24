package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"at.ourproject/vfeeg-backend/energyrequest"
	"at.ourproject/vfeeg-backend/model"
	mqttclient "at.ourproject/vfeeg-backend/mqtt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixture TE001006: 953014 active 2024-01-01..2024-05-28, 953015 from 2024-05-20, 953016 from 2024-01-01. 953013 is
// used (and locked) by TestRequestMeteringPointValues, so these tests use other metering points and periods.
const (
	zp14 = "AT0030000000000000000000000953014"
	zp15 = "AT0030000000000000000000000953015"
	zp16 = "AT0030000000000000000000000953016"
)

type sentRequest struct {
	meter    string
	from, to int64
}

// syncResponse is the body of a syncenergy answer (online community).
type syncResponse struct {
	Requested int                     `json:"requested"`
	Skipped   []energyrequest.Skipped `json:"skipped"`
}

// syncEnergy posts body to the syncenergy handler of TE001006 and returns the status, the energy data requests
// sent via MQTT and the run stored last; the response body goes to resp.
func syncEnergy(t *testing.T, body string, resp ...*syncResponse) (int, []sentRequest, energyrequest.Run) {
	t.Helper()
	h := newTestMeteringHandler(t)
	orig := mqttclient.RequestingEnergyData
	t.Cleanup(func() { mqttclient.RequestingEnergyData = orig })
	var sent []sentRequest
	mqttclient.RequestingEnergyData = func(eeg *model.Eeg, meter *model.MeteringPoint, fromDate, toDate int64) error {
		sent = append(sent, sentRequest{meter: meter.MeteringPoint, from: fromDate, to: toDate})
		return nil
	}
	req, _ := http.NewRequest("POST", "/meteringpoint/syncenergy", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.requestMeteringPointValues()(w, req, nil, "TE001006")
	if w.Code == http.StatusOK {
		var got syncResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		for _, r := range resp {
			*r = got
		}
	}

	rows, _, err := h.db.EnergyRequestTenantRows(context.Background(), "TE001006", energyrequest.LockWindow)
	require.NoError(t, err)
	var last energyrequest.Run
	for _, r := range rows {
		if r.Type == energyrequest.RowTypeRun {
			last = energyrequest.Run{}
			require.NoError(t, json.Unmarshal(r.Message, &last))
		}
	}
	return w.Code, sent, last
}

func start(y int, m time.Month, d int) int64 {
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local).UnixMilli()
}

func end(y int, m time.Month, d int) int64 {
	return time.Date(y, m, d, 23, 45, 0, 0, time.Local).UnixMilli()
}

func TestSyncEnergy_periodsCompleteAndLock(t *testing.T) {
	// 953015 with its own period (times inside the day are cut to start and end of day), 953016 with the request's
	body := fmt.Sprintf(`{"meteringPoints": [
		{"meter": %q, "direction": "CONSUMPTION", "from": %d, "to": %d},
		{"meter": %q, "direction": "CONSUMPTION"}],
		"from": %d, "to": %d, "complete": [%q]}`,
		zp15, start(2025, 3, 5)+3_600_000, start(2025, 3, 7)+3_600_000, zp16,
		start(2025, 3, 1), start(2025, 3, 31), zp14)

	var resp syncResponse
	code, sent, first := syncEnergy(t, body, &resp)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, syncResponse{Requested: 2, Skipped: first.Skipped}, resp)
	assert.ElementsMatch(t, []sentRequest{
		{meter: zp15, from: start(2025, 3, 5), to: end(2025, 3, 7)},
		{meter: zp16, from: start(2025, 3, 1), to: end(2025, 3, 31)},
	}, sent)
	assert.Equal(t, []energyrequest.Skipped{{MeteringPoint: zp14, Reason: energyrequest.ReasonComplete}}, first.Skipped)
	assert.False(t, first.NoSummary, "complete metering points count for the summary")

	// the same request again within the hour: nothing is sent, the run says why
	code, sent, second := syncEnergy(t, body, &resp)
	assert.Equal(t, http.StatusOK, code)
	assert.Empty(t, sent)
	assert.Equal(t, 0, resp.Requested)
	assert.Equal(t, second.Skipped, resp.Skipped)
	until := time.UnixMilli(first.CreatedAt).Add(energyrequest.LockWindow).UnixMilli()
	assert.NotEqual(t, first.RunId, second.RunId, "a run where everything is skipped is stored too")
	assert.Empty(t, second.Meters)
	assert.ElementsMatch(t, []energyrequest.Skipped{
		{MeteringPoint: zp14, Reason: energyrequest.ReasonComplete},
		{MeteringPoint: zp15, Reason: energyrequest.ReasonAlreadyRequested, Until: until},
		{MeteringPoint: zp16, Reason: energyrequest.ReasonAlreadyRequested, Until: until},
	}, second.Skipped)

	// a rejected metering point is free at once
	h := newTestMeteringHandler(t)
	require.NoError(t, h.db.SaveNotification("TE001006", model.EBMS_ZP_REJ, []string{zp15}, []string{"86"}, model.CR_REQ_PT))
	_, sent, _ = syncEnergy(t, body)
	assert.Equal(t, []sentRequest{{meter: zp15, from: start(2025, 3, 5), to: end(2025, 3, 7)}}, sent)

	// a single request is locked too; a period that does not overlap is free
	single := func(from, to int64) string {
		return fmt.Sprintf(`{"meteringPoints": [{"meter": %q, "direction": "CONSUMPTION"}], "from": %d, "to": %d}`,
			zp16, from, to)
	}
	code, sent, _ = syncEnergy(t, single(start(2025, 3, 31), start(2025, 4, 30)), &resp)
	assert.Equal(t, http.StatusOK, code)
	assert.Empty(t, sent, "31.03. was requested already")
	assert.Equal(t, syncResponse{Requested: 0, Skipped: []energyrequest.Skipped{{MeteringPoint: zp16,
		Reason: energyrequest.ReasonAlreadyRequested, Until: until}}}, resp, "a locked single request says why")
	_, sent, _ = syncEnergy(t, single(start(2025, 4, 1), start(2025, 4, 30)))
	assert.Equal(t, []sentRequest{{meter: zp16, from: start(2025, 4, 1), to: end(2025, 4, 30)}}, sent)
}

// contract: a request without the new fields requests every metering point for the request period, as before.
func TestSyncEnergy_withoutNewFieldsAsBefore(t *testing.T) {
	body := fmt.Sprintf(`{"meteringPoints": [{"meter": %q, "direction": "CONSUMPTION"}, {"meter": %q, "direction": "CONSUMPTION"}], "from": %d, "to": %d}`,
		zp15, zp16, start(2025, 6, 1), start(2025, 6, 30))
	var resp syncResponse
	code, sent, run := syncEnergy(t, body, &resp)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, syncResponse{Requested: 2, Skipped: []energyrequest.Skipped{}}, resp)
	assert.ElementsMatch(t, []sentRequest{
		{meter: zp15, from: start(2025, 6, 1), to: end(2025, 6, 30)},
		{meter: zp16, from: start(2025, 6, 1), to: end(2025, 6, 30)},
	}, sent)
	assert.Equal(t, start(2025, 6, 1), run.From)
	assert.Equal(t, end(2025, 6, 30), run.To)
	assert.Empty(t, run.Skipped)
}

// web found every metering point complete: nothing is requested, the run and the answer say so.
func TestSyncEnergy_allComplete(t *testing.T) {
	body := fmt.Sprintf(`{"meteringPoints": [], "from": %d, "to": %d, "complete": [%q, %q]}`,
		start(2025, 8, 1), start(2025, 8, 31), zp15, zp16)
	var resp syncResponse
	code, sent, run := syncEnergy(t, body, &resp)
	assert.Equal(t, http.StatusOK, code)
	assert.Empty(t, sent)
	want := []energyrequest.Skipped{{MeteringPoint: zp15, Reason: energyrequest.ReasonComplete},
		{MeteringPoint: zp16, Reason: energyrequest.ReasonComplete}}
	assert.Equal(t, syncResponse{Requested: 0, Skipped: want}, resp)
	assert.Equal(t, want, run.Skipped, "stored, so the summary shows it")
	assert.Empty(t, run.Meters)
}

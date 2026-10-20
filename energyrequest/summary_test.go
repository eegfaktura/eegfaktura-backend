package energyrequest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	sepFrom = time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	sepTo   = time.Date(2026, 9, 30, 23, 45, 0, 0, time.Local).UnixMilli()
	created = time.Date(2026, 10, 20, 8, 0, 0, 0, time.Local)
	tenants = map[string]bool{"RC100100": true}
)

func row(t *testing.T, id int64, tenant, rowType string, v interface{}) Row {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return Row{Id: id, Tenant: tenant, Type: rowType, Message: b}
}

func testRun(meters ...string) Run {
	r := NewRun("AT00000000000000000000RC100100", sepFrom, sepTo)
	for _, m := range meters {
		r.Add(m, sepFrom, sepTo)
	}
	r.CreatedAt = created.UnixMilli()
	return *r
}

func answerRow(t *testing.T, id int64, tenant, typ, meter string, codes ...string) Row {
	return row(t, id, tenant, messageRow, map[string]interface{}{"type": typ, "meteringPoints": []string{meter}, "responseCodes": codes})
}

func TestEvaluate_allStates(t *testing.T) {
	run := testRun("ZP1", "ZP2", "ZP3")
	run.Skipped = []Skipped{{MeteringPoint: "ZP4", Reason: ReasonNotActiveInPeriod}}
	rows := []Row{
		row(t, 10, "RC100100", RowTypeRun, run),
		answerRow(t, 11, "RC100100", answerAccepted, "ZP1"),
		answerRow(t, 12, "RC100100", answerRejected, "ZP2", "Zählpunkt nicht gefunden"),
		// ZP3: no answer
	}

	s, done := Evaluate(run, 10, rows, tenants, created.Add(time.Hour))
	assert.False(t, done, "a missing answer keeps the run open")
	assert.Equal(t, 3, s.Requested)
	assert.Equal(t, []MeterRange{{MeteringPoint: "ZP1", From: sepFrom, To: sepTo}}, s.Accepted)
	assert.Equal(t, []Rejected{{MeteringPoint: "ZP2", Codes: []string{"Zählpunkt nicht gefunden"}}}, s.Rejected)
	assert.Equal(t, []string{"ZP3"}, s.NoAnswer)
	assert.Equal(t, run.Skipped, s.Skipped)

	_, done = Evaluate(run, 10, rows, tenants, created.Add(Deadline))
	assert.True(t, done, "closed at the deadline")

	rows = append(rows, answerRow(t, 13, "RC100100", answerAccepted, "ZP3"))
	_, done = Evaluate(run, 10, rows, tenants, created.Add(time.Hour))
	assert.True(t, done, "closed as soon as every metering point has an answer")
}

func TestEvaluate_rowsBeforeTheRunAndOfOtherTenantsDoNotCount(t *testing.T) {
	run := testRun("ZP1", "ZP2")
	rows := []Row{
		answerRow(t, 9, "RC100100", answerRejected, "ZP1", "56"),  // before the run
		answerRow(t, 11, "RC999999", answerRejected, "ZP2", "56"), // other tenant
	}
	s, _ := Evaluate(run, 10, rows, tenants, created)
	assert.Equal(t, []string{"ZP1", "ZP2"}, s.NoAnswer)
}

func TestEvaluate_gemeinschaftWithSeveralTenants(t *testing.T) {
	// a GEA: the answer was stored under another tenant of the same community id
	run := testRun("ZP1", "ZP2")
	rows := []Row{answerRow(t, 11, "GC100019-002", answerRejected, "ZP1", "56")}
	s, _ := Evaluate(run, 10, rows, map[string]bool{"GC100019-001": true, "GC100019-002": true}, created)
	assert.Len(t, s.Rejected, 1)
}

func TestEvaluate_laterRunSupersedes(t *testing.T) {
	run := testRun("ZP1", "ZP2")
	later := testRun("ZP2", "ZP3")
	rows := []Row{
		row(t, 20, "RC100100", RowTypeRun, later),
		answerRow(t, 21, "RC100100", answerRejected, "ZP2", "56"), // belongs to the later run
	}
	s, _ := Evaluate(run, 10, rows, tenants, created)
	assert.Equal(t, []string{"ZP2"}, s.Superseded)
	assert.Equal(t, []string{"ZP1"}, s.NoAnswer)
	assert.Empty(t, s.Rejected)

	s, _ = Evaluate(later, 20, rows, tenants, created)
	assert.Len(t, s.Rejected, 1)
}

func TestEvaluate_latestAnswerWins(t *testing.T) {
	run := testRun("ZP1", "ZP2")
	rows := []Row{
		answerRow(t, 11, "RC100100", answerAccepted, "ZP1"),
		answerRow(t, 12, "RC100100", answerRejected, "ZP1", "86"),
	}
	s, _ := Evaluate(run, 10, rows, tenants, created)
	assert.Len(t, s.Rejected, 1)
	assert.Empty(t, s.Accepted)
}

func TestRun_AddAndSkip(t *testing.T) {
	r := NewRun("EC", sepFrom, sepTo)
	r.Skip("ZP1", ReasonNotAssigned) // an old participation of ZP1
	r.Add("ZP1", sepFrom, sepFrom+1000)
	r.Add("ZP1", sepFrom+500, sepTo) // a second participation: widest period
	r.Skip("ZP1", ReasonNotAssigned)
	r.Skip("ZP2", ReasonNotRegistered)
	r.Skip("ZP2", ReasonNotAssigned)
	// an ended participation first, then the assigned one without an inactive date: its reason counts
	r.Skip("ZP3", ReasonNotAssigned)
	r.Skip("ZP3", ReasonNoActivePeriod)

	assert.Equal(t, []MeterRange{{MeteringPoint: "ZP1", From: sepFrom, To: sepTo}}, r.Meters)
	assert.Equal(t, []Skipped{{MeteringPoint: "ZP2", Reason: ReasonNotRegistered},
		{MeteringPoint: "ZP3", Reason: ReasonNoActivePeriod}}, r.Skipped)
}

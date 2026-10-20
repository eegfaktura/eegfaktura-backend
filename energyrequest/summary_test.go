package energyrequest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// request period September 2026 as the handler computes it (local time, end = 23:45 of the last day)
var (
	sepFrom = time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	sepTo   = time.Date(2026, 9, 30, 23, 45, 0, 0, time.Local).UnixMilli()
	// what a full delivery carries: end exclusive at the start of the next day
	octStart = time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	created  = time.Date(2026, 10, 20, 8, 0, 0, 0, time.Local)
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

func dataRow(t *testing.T, id int64, ecId, meter string, from, to int64) Row {
	return row(t, id, "RC100100", RowTypeData, Data{Type: RowTypeData, EcId: ecId, MeteringPoint: meter, From: from, To: to})
}

var tenants = map[string]bool{"RC100100": true}

func TestEvaluate_allStates(t *testing.T) {
	run := testRun("ZP1", "ZP2", "ZP3", "ZP4", "ZP5")
	run.Skipped = []Skipped{{MeteringPoint: "ZP6", Reason: ReasonNotActiveInPeriod}}
	rows := []Row{
		row(t, 10, "RC100100", RowTypeRun, run),
		answerRow(t, 11, "RC100100", answerAccepted, "ZP1"),
		dataRow(t, 12, run.EcId, "ZP1", sepFrom, octStart),
		answerRow(t, 13, "RC100100", answerRejected, "ZP2", "Zählpunkt nicht gefunden"),
		answerRow(t, 14, "RC100100", answerAccepted, "ZP3"),
		dataRow(t, 15, run.EcId, "ZP3", sepFrom, time.Date(2026, 9, 28, 0, 0, 0, 0, time.Local).UnixMilli()),
		answerRow(t, 16, "RC100100", answerAccepted, "ZP4"),
		// ZP5: no answer
	}

	s, done := Evaluate(run, 10, rows, tenants, created.Add(time.Hour))
	assert.False(t, done, "accepted without complete data and missing answers keep the run open")
	assert.Equal(t, 5, s.Requested)
	assert.Equal(t, 1, s.Complete)
	assert.Equal(t, []Rejected{{MeteringPoint: "ZP2", Codes: []string{"Zählpunkt nicht gefunden"}}}, s.Rejected)
	assert.Equal(t, []string{"ZP5"}, s.NoAnswer)
	require.Len(t, s.Incomplete, 2)
	assert.Equal(t, "ZP3", s.Incomplete[0].MeteringPoint)
	assert.Equal(t, "ZP4", s.Incomplete[1].MeteringPoint)
	assert.Zero(t, s.Incomplete[1].DataTo, "accepted without any data")
	assert.Equal(t, run.Skipped, s.Skipped)

	_, done = Evaluate(run, 10, rows, tenants, created.Add(Deadline))
	assert.True(t, done, "closed at the deadline")
}

func TestEvaluate_doneEarlyWhenEveryMeterIsFinal(t *testing.T) {
	run := testRun("ZP1", "ZP2")
	rows := []Row{
		dataRow(t, 11, run.EcId, "ZP1", sepFrom, octStart),
		answerRow(t, 12, "RC100100", answerRejected, "ZP2", "56"),
	}
	s, done := Evaluate(run, 10, rows, tenants, created.Add(time.Minute))
	assert.True(t, done)
	assert.Equal(t, 1, s.Complete)
}

func TestEvaluate_deliveriesOverSeveralMessages(t *testing.T) {
	run := testRun("ZP1", "ZP2")
	mid := time.Date(2026, 9, 15, 0, 0, 0, 0, time.Local).UnixMilli()
	rows := []Row{
		dataRow(t, 11, run.EcId, "ZP1", mid, octStart),
		dataRow(t, 12, run.EcId, "ZP1", sepFrom, mid),
		// a delivery ending with the last quarter hour (inclusive end) is complete as well
		dataRow(t, 13, run.EcId, "ZP2", sepFrom, sepTo),
	}
	s, _ := Evaluate(run, 10, rows, tenants, created)
	assert.Equal(t, 2, s.Complete)
}

func TestEvaluate_periodCutAtInactiveDate(t *testing.T) {
	run := testRun("ZP2")
	// inactive since 15.09. as the handler passes it: the stored date, i.e. UTC midnight
	inactive := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC).UnixMilli()
	run.Add("ZP1", sepFrom, inactive)
	rows := []Row{dataRow(t, 11, run.EcId, "ZP1", sepFrom, time.Date(2026, 9, 15, 0, 0, 0, 0, time.Local).UnixMilli())}
	s, _ := Evaluate(run, 10, rows, tenants, created)
	assert.Equal(t, 1, s.Complete)
}

func TestEvaluate_rowsBeforeTheRunAndOfOtherCommunitiesDoNotCount(t *testing.T) {
	run := testRun("ZP1", "ZP2")
	rows := []Row{
		answerRow(t, 9, "RC100100", answerRejected, "ZP1", "56"),                   // before the run
		answerRow(t, 11, "RC999999", answerRejected, "ZP2", "56"),                  // other tenant
		dataRow(t, 12, "AT00000000000000000000RC999999", "ZP1", sepFrom, octStart), // other community
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
}

func TestRun_AddAndSkip(t *testing.T) {
	r := NewRun("EC", sepFrom, sepTo)
	r.Skip("ZP1", ReasonNotAssigned) // an old participation of ZP1
	r.Add("ZP1", sepFrom, sepFrom+1000)
	r.Add("ZP1", sepFrom+500, sepTo) // a second participation: widest period
	r.Skip("ZP1", ReasonNotAssigned)
	r.Skip("ZP2", ReasonNotRegistered)
	r.Skip("ZP2", ReasonNotAssigned)

	assert.Equal(t, []MeterRange{{MeteringPoint: "ZP1", From: sepFrom, To: sepTo}}, r.Meters)
	assert.Equal(t, []Skipped{{MeteringPoint: "ZP2", Reason: ReasonNotRegistered}}, r.Skipped)

	r.markNotFound("ZP1")
	r.markNotFound("ZP3")
	assert.Equal(t, Skipped{MeteringPoint: "ZP3", Reason: ReasonNotFound}, r.Skipped[1])
}

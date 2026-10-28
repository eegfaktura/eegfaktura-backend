package energyrequest

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func msg(t *testing.T, id int64, at time.Time, typ string, meters ...string) Message {
	b, err := json.Marshal(map[string]interface{}{"type": typ, "meteringPoints": meters, "responseCodes": []string{}})
	require.NoError(t, err)
	return Message{Id: id, Date: at, Content: b}
}

func summaryMsg(t *testing.T, id int64, at time.Time, s Summary) Message {
	s.Type = SummaryType
	b, err := json.Marshal(s)
	require.NoError(t, err)
	return Message{Id: id, Date: at, Content: b}
}

func TestCollapse_requestsAndAnswersOfARunWithSummary(t *testing.T) {
	single := testRun("ZP3")
	single.NoSummary = true
	runs := []Row{
		row(t, 10, "RC100100", RowTypeRun, testRun("ZP1", "ZP2")),
		row(t, 20, "RC100100", RowTypeRun, single),
	}
	at := created.Add(time.Minute)
	messages := []Message{
		msg(t, 9, at, "ANFORDERUNG_PT", "ZP1"), // before the run
		msg(t, 11, at, "ANFORDERUNG_PT", "ZP1"),
		msg(t, 12, at, "ANFORDERUNG_PT", "zp2"), // metering points compare case-insensitively
		msg(t, 13, at, answerAccepted, "ZP1"),
		msg(t, 14, at, answerAccepted, "ZP9"),   // not in the run
		msg(t, 15, at, "ABSCHLUSS_ECON", "ZP1"), // another process
		msg(t, 21, at, "ANFORDERUNG_PT", "ZP3"), // the single request keeps its messages
		msg(t, 22, at, answerRejected, "ZP3"),
		msg(t, 23, at, answerRejected, "ZP2"),
	}
	hidden, latest := Collapse(runs, nil, messages)
	assert.Equal(t, map[int64]bool{11: true, 12: true, 13: true, 23: true}, hidden)
	assert.Equal(t, map[int64]int64{10: 23}, latest)
}

func TestCollapse_aLaterSingleRequestTakesOverItsMeteringPoint(t *testing.T) {
	single := testRun("ZP1")
	single.NoSummary = true
	runs := []Row{
		row(t, 10, "RC100100", RowTypeRun, testRun("ZP1", "ZP2")),
		row(t, 20, "RC100100", RowTypeRun, single),
	}
	at := created.Add(time.Minute)
	hidden, _ := Collapse(runs, nil, []Message{
		msg(t, 11, at, "ANFORDERUNG_PT", "ZP1"),
		msg(t, 21, at, answerAccepted, "ZP1"),
		msg(t, 22, at, answerAccepted, "ZP2"),
	})
	assert.Equal(t, map[int64]bool{11: true, 22: true}, hidden)
}

func TestCollapse_onlyWithinTheWindowAfterTheRun(t *testing.T) {
	runs := []Row{row(t, 10, "RC100100", RowTypeRun, testRun("ZP1", "ZP2"))}
	hidden, latest := Collapse(runs, nil, []Message{
		msg(t, 11, created.Add(CollapseWindow), answerAccepted, "ZP1"),
		msg(t, 12, created.Add(CollapseWindow+time.Minute), answerAccepted, "ZP2"),
	})
	assert.Equal(t, map[int64]bool{11: true}, hidden)
	assert.Equal(t, map[int64]int64{10: 11}, latest)
}

// Older messages, whose run is no longer read: the stored summary written after them stands for them.
func TestCollapse_storedSummaryForOlderMessages(t *testing.T) {
	at := created.Add(time.Minute)
	written := created.Add(Deadline)
	summaries := []Message{
		summaryMsg(t, 30, written, Summary{Accepted: []MeterRange{{MeteringPoint: "ZP1"}},
			Rejected: []Rejected{{MeteringPoint: "ZP2"}}, NoAnswer: []string{"ZP3"}}),
		summaryMsg(t, 40, written, Summary{Accepted: []MeterRange{{MeteringPoint: "ZP7"}}, Live: true}), // a live entry
	}
	hidden, latest := Collapse(nil, summaries, []Message{
		msg(t, 11, at, "ANFORDERUNG_PT", "ZP1"),
		msg(t, 12, at, answerRejected, "ZP2"),
		msg(t, 13, at, "ANFORDERUNG_PT", "ZP3"),
		msg(t, 14, at, answerAccepted, "ZP9"),                                       // not named
		msg(t, 15, written.Add(-CollapseWindow-time.Minute), answerAccepted, "ZP1"), // too long before
		msg(t, 31, written.Add(time.Minute), answerAccepted, "ZP1"),                 // after the summary
		msg(t, 35, at, answerAccepted, "ZP7"),
	})
	assert.Equal(t, map[int64]bool{11: true, 12: true, 13: true}, hidden)
	assert.Empty(t, latest)
}

// A run that is read decides for its messages, also against a stored summary.
func TestCollapse_runBeforeSummary(t *testing.T) {
	single := testRun("ZP1")
	single.NoSummary = true
	runs := []Row{row(t, 20, "RC100100", RowTypeRun, single)}
	summaries := []Message{summaryMsg(t, 30, created.Add(time.Hour), Summary{Accepted: []MeterRange{{MeteringPoint: "ZP1"}}})}
	hidden, _ := Collapse(runs, summaries, []Message{msg(t, 21, created.Add(time.Minute), answerAccepted, "ZP1")})
	assert.Empty(t, hidden)
}

func TestIsSummary(t *testing.T) {
	assert.True(t, IsSummary(summaryMsg(t, 1, created, Summary{}).Content))
	assert.False(t, IsSummary(summaryMsg(t, 1, created, Summary{Live: true}).Content))
	assert.False(t, IsSummary(msg(t, 1, created, answerAccepted, "ZP1").Content))
	assert.False(t, IsSummary(json.RawMessage(`not json`)))
}

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

func collapse(t *testing.T, runs []Row, messages ...Message) (map[int64]bool, map[int64]int64) {
	c := NewCollapser("RC100100")
	c.AddRuns(runs)
	return c.Hide(messages)
}

func TestCollapser_requestsAndAnswersOfARunWithSummary(t *testing.T) {
	single := testRun("ZP3")
	single.NoSummary = true
	runs := []Row{
		row(t, 10, "RC100100", RowTypeRun, testRun("ZP1", "ZP2")),
		row(t, 20, "RC100100", RowTypeRun, single),
	}
	at := created.Add(time.Minute)
	hidden, latest := collapse(t, runs,
		msg(t, 9, at, "ANFORDERUNG_PT", "ZP1"), // before the run
		msg(t, 11, at, "ANFORDERUNG_PT", "ZP1"),
		msg(t, 12, at, "ANFORDERUNG_PT", "zp2"), // metering points compare case-insensitively
		msg(t, 13, at, answerAccepted, "ZP1"),
		msg(t, 14, at, answerAccepted, "ZP9"),   // not in the run
		msg(t, 15, at, "ABSCHLUSS_ECON", "ZP1"), // another process
		msg(t, 21, at, "ANFORDERUNG_PT", "ZP3"), // the single request keeps its messages
		msg(t, 22, at, answerRejected, "ZP3"),
		msg(t, 23, at, answerRejected, "ZP2"),
	)
	assert.Equal(t, map[int64]bool{11: true, 12: true, 13: true, 23: true}, hidden)
	assert.Equal(t, map[int64]int64{10: 23}, latest)
}

func TestCollapser_aLaterSingleRequestTakesOverItsMeteringPoint(t *testing.T) {
	single := testRun("ZP1")
	single.NoSummary = true
	runs := []Row{
		row(t, 10, "RC100100", RowTypeRun, testRun("ZP1", "ZP2")),
		row(t, 20, "RC100100", RowTypeRun, single),
	}
	at := created.Add(time.Minute)
	hidden, _ := collapse(t, runs,
		msg(t, 11, at, "ANFORDERUNG_PT", "ZP1"),
		msg(t, 21, at, answerAccepted, "ZP1"),
		msg(t, 22, at, answerAccepted, "ZP2"),
	)
	assert.Equal(t, map[int64]bool{11: true, 22: true}, hidden)
}

// An earlier single request keeps its messages, also when a run with a summary follows (F1 of the review).
func TestCollapser_anEarlierSingleRequestKeepsItsMessages(t *testing.T) {
	single := testRun("ZP1")
	single.NoSummary = true
	runs := []Row{
		row(t, 10, "RC100100", RowTypeRun, single),
		row(t, 20, "RC100100", RowTypeRun, testRun("ZP1", "ZP2")),
	}
	at := created.Add(time.Minute)
	hidden, _ := collapse(t, runs,
		msg(t, 11, at, "ANFORDERUNG_PT", "ZP1"),
		msg(t, 12, at, answerAccepted, "ZP1"),
		msg(t, 21, at, answerAccepted, "ZP1"),
	)
	assert.Equal(t, map[int64]bool{21: true}, hidden)
}

func TestCollapser_onlyWithinTheWindowAfterTheRun(t *testing.T) {
	runs := []Row{row(t, 10, "RC100100", RowTypeRun, testRun("ZP1", "ZP2"))}
	hidden, latest := collapse(t, runs,
		msg(t, 11, created.Add(CollapseWindow), answerAccepted, "ZP1"),
		msg(t, 12, created.Add(CollapseWindow+time.Minute), answerAccepted, "ZP2"),
	)
	assert.Equal(t, map[int64]bool{11: true}, hidden)
	assert.Equal(t, map[int64]int64{10: 11}, latest)
}

// GEA: the messages of another tenant's run stay in this tenant's list (that tenant has the live entry and summary),
// and that run still owns them against an older run of this tenant.
func TestCollapser_runOfAnotherTenantOfTheCommunity(t *testing.T) {
	runs := []Row{
		row(t, 10, "RC100100", RowTypeRun, testRun("ZP1", "ZP2")),
		row(t, 20, "RC200200", RowTypeRun, testRun("ZP1", "ZP2")),
	}
	at := created.Add(time.Minute)
	hidden, _ := collapse(t, runs,
		msg(t, 11, at, "ANFORDERUNG_PT", "ZP1"),
		msg(t, 21, at, "ANFORDERUNG_PT", "ZP1"),
		msg(t, 22, at, answerAccepted, "ZP2"),
	)
	assert.Equal(t, map[int64]bool{11: true}, hidden)
}

func TestCollapser_addRunsTwice(t *testing.T) {
	c := NewCollapser("RC100100")
	r := row(t, 10, "RC100100", RowTypeRun, testRun("ZP1", "ZP2"))
	c.AddRuns([]Row{r})
	c.AddRuns([]Row{r, answerRow(t, 11, "RC100100", answerAccepted, "ZP1")}) // not a run: ignored
	hidden, _ := c.Hide([]Message{msg(t, 12, created.Add(time.Minute), answerAccepted, "ZP1")})
	assert.Equal(t, map[int64]bool{12: true}, hidden)
}

func TestCollapsible(t *testing.T) {
	assert.True(t, Collapsible(msg(t, 1, created, "ANFORDERUNG_PT", "ZP1").Content))
	assert.True(t, Collapsible(msg(t, 1, created, answerRejected, "ZP1").Content))
	assert.False(t, Collapsible(msg(t, 1, created, SummaryType).Content))
	assert.False(t, Collapsible(json.RawMessage(`not json`)))
}

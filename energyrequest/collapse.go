package energyrequest

import (
	"encoding/json"
	"strings"
	"time"
)

// Message is a listed notification as far as a Collapser needs it.
type Message struct {
	Id      int64
	Date    time.Time
	Content json.RawMessage
}

// collapsedTypes are the messages a summary stands for: the request echo per metering point and the answers.
var collapsedTypes = map[string]bool{"ANFORDERUNG_PT": true, answerAccepted: true, answerRejected: true}

// CollapseWindow is how long after a run its requests and answers still belong to it (the summary is written at
// Deadline at the latest).
const CollapseWindow = Deadline + time.Hour

// Collapsible reports whether a message is of a type a Collapser may leave out, so its runs have to be read.
func Collapsible(content json.RawMessage) bool {
	var m struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(content, &m) == nil && collapsedTypes[m.Type]
}

type collapseRun struct {
	id        int64
	createdAt time.Time
	own       bool // a run of the listed tenant with a summary: its messages are left out
	meters    map[string]bool
}

// Collapser decides which requests (ANFORDERUNG_PT) and answers (ANTWORT_PT, ABLEHNUNG_PT) the Messages list of a
// tenant leaves out (platform#173, decision 28.10.2026): those of a run with a summary of that tenant. The live entry
// stands for them while the run is open, the stored summary after it closed. A message belongs to the newest run of
// the community before it (lower row id, created at most CollapseWindow earlier) that sent one of its metering points.
// A request for a single metering point (NoSummary) keeps its messages, and so does a run of another tenant of the
// community (GEA): that tenant gets its live entry and summary, the messages may be stored under this one.
type Collapser struct {
	tenant string
	runs   map[int64]collapseRun
}

func NewCollapser(tenant string) *Collapser {
	return &Collapser{tenant: tenant, runs: map[int64]collapseRun{}}
}

// AddRuns adds RUN rows of the community; rows already added and other rows are ignored.
func (c *Collapser) AddRuns(rows []Row) {
	for _, r := range rows {
		if _, ok := c.runs[r.Id]; ok || r.Type != RowTypeRun {
			continue
		}
		var run Run
		if json.Unmarshal(r.Message, &run) != nil {
			continue
		}
		meters := map[string]bool{}
		for _, m := range run.Meters {
			meters[strings.ToUpper(m.MeteringPoint)] = true
		}
		c.runs[r.Id] = collapseRun{id: r.Id, createdAt: time.UnixMilli(run.CreatedAt),
			own: !run.NoSummary && strings.EqualFold(r.Tenant, c.tenant), meters: meters}
	}
}

// Hide returns the ids of the messages left out, and for each run the highest id of its messages left out (its live
// entry is placed there).
func (c *Collapser) Hide(messages []Message) (hidden map[int64]bool, latest map[int64]int64) {
	hidden = map[int64]bool{}
	latest = map[int64]int64{}
	for _, msg := range messages {
		var m struct {
			Type           string   `json:"type"`
			MeteringPoints []string `json:"meteringPoints"`
		}
		if json.Unmarshal(msg.Content, &m) != nil || !collapsedTypes[m.Type] {
			continue
		}
		var owner *collapseRun
		for _, r := range c.runs {
			if r.id >= msg.Id || msg.Date.Sub(r.createdAt) > CollapseWindow || (owner != nil && owner.id > r.id) {
				continue
			}
			for _, mp := range m.MeteringPoints {
				if r.meters[strings.ToUpper(mp)] {
					r := r
					owner = &r
					break
				}
			}
		}
		if owner == nil || !owner.own {
			continue
		}
		hidden[msg.Id] = true
		if msg.Id > latest[owner.id] {
			latest[owner.id] = msg.Id
		}
	}
	return hidden, latest
}

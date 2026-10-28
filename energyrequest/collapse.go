package energyrequest

import (
	"encoding/json"
	"strings"
	"time"
)

// Message is a listed notification as far as Collapse needs it.
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

type owner struct {
	id      int64
	at      time.Time // run: creation; stored summary: when it was written
	summary bool      // false: a request for a single metering point, its messages stay
	meters  map[string]bool
}

// IsSummary reports whether content is a stored summary (not a live entry).
func IsSummary(content json.RawMessage) bool {
	var s Summary
	return json.Unmarshal(content, &s) == nil && s.Type == SummaryType && !s.Live
}

// Collapse returns the ids of the requests (ANFORDERUNG_PT) and answers (ANTWORT_PT, ABLEHNUNG_PT) among messages
// that belong to a run with a summary: in Messages the summary, or the live entry while the run is open, stands for
// them (platform#173). A message belongs to the newest run before it (lower row id, created at most CollapseWindow
// earlier) that sent one of its metering points; a request for a single metering point (NoSummary) keeps its
// messages. runs are the RUN rows of the community that Live reads (the last LookBack). For an older message, whose
// run is no longer among them, the stored summary written after it (higher id, at most CollapseWindow later) that
// names one of its metering points stands for it; a single request between that run and its summary is then left
// out too.
//
// latest maps the row id of a run to the highest id of its messages, so its live entry can be placed with its
// newest message.
func Collapse(runs []Row, summaries []Message, messages []Message) (hidden map[int64]bool, latest map[int64]int64) {
	runOwners := make([]owner, 0, len(runs))
	for _, r := range runs {
		var run Run
		if r.Type != RowTypeRun || json.Unmarshal(r.Message, &run) != nil {
			continue
		}
		meters := map[string]bool{}
		for _, m := range run.Meters {
			meters[strings.ToUpper(m.MeteringPoint)] = true
		}
		runOwners = append(runOwners, owner{id: r.Id, at: time.UnixMilli(run.CreatedAt), summary: !run.NoSummary,
			meters: meters})
	}
	summaryOwners := make([]owner, 0, len(summaries))
	for _, m := range summaries {
		var s Summary
		if json.Unmarshal(m.Content, &s) != nil || s.Type != SummaryType || s.Live {
			continue
		}
		meters := map[string]bool{}
		add := func(mp string) { meters[strings.ToUpper(mp)] = true }
		for _, a := range s.Accepted {
			add(a.MeteringPoint)
		}
		for _, r := range s.Rejected {
			add(r.MeteringPoint)
		}
		for _, mp := range s.NoAnswer {
			add(mp)
		}
		for _, mp := range s.Superseded {
			add(mp)
		}
		summaryOwners = append(summaryOwners, owner{id: m.Id, at: m.Date, summary: true, meters: meters})
	}

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
		names := func(o owner) bool {
			for _, mp := range m.MeteringPoints {
				if o.meters[strings.ToUpper(mp)] {
					return true
				}
			}
			return false
		}
		// the newest run before the message
		var run *owner
		for i := range runOwners {
			o := &runOwners[i]
			if o.id < msg.Id && msg.Date.Sub(o.at) <= CollapseWindow && (run == nil || o.id > run.id) && names(*o) {
				run = o
			}
		}
		if run != nil {
			if run.summary {
				hidden[msg.Id] = true
				if msg.Id > latest[run.id] {
					latest[run.id] = msg.Id
				}
			}
			continue
		}
		// else the first stored summary after the message
		var summary *owner
		for i := range summaryOwners {
			o := &summaryOwners[i]
			if o.id > msg.Id && o.at.Sub(msg.Date) <= CollapseWindow && (summary == nil || o.id < summary.id) && names(*o) {
				summary = o
			}
		}
		if summary != nil {
			hidden[msg.Id] = true
		}
	}
	return hidden, latest
}

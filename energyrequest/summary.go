// Package energyrequest summarises an energy data request (ANFORDERUNG_PT) for several metering points in one
// message (platform#173). Nothing is stored outside base.notification: the request ("run") and the deliveries of its
// metering points are hidden notification rows (process ENERGY_REQUEST), the answers are the existing ANTWORT_PT /
// ABLEHNUNG_PT notifications, and the summary is a visible notification. The status of each metering point is
// computed from these rows on every check.
package energyrequest

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/jmoiron/sqlx/types"
)

const (
	RowTypeRun  = "RUN"
	RowTypeData = "DATA"

	SummaryType = "ZUSAMMENFASSUNG_PT"

	// Deadline after which a run is summarised even if answers or data are missing.
	Deadline = 24 * time.Hour

	// Reasons why a requested metering point was not requested from the grid operator.
	ReasonNotFound          = "NOT_FOUND"
	ReasonNotRegistered     = "NOT_REGISTERED"
	ReasonNotAssigned       = "NOT_ASSIGNED"
	ReasonNoActivePeriod    = "NO_ACTIVE_PERIOD"
	ReasonNotActiveInPeriod = "NOT_ACTIVE_IN_PERIOD"

	answerAccepted = "ANTWORT_PT"
	// notification type of the answers and of the summary (model.N_TYPE_MESSAGE)
	messageRow     = "MESSAGE"
	answerRejected = "ABLEHNUNG_PT"
)

// MeterRange is a requested metering point with the period actually requested (cut to its active period).
type MeterRange struct {
	MeteringPoint string `json:"meteringPoint"`
	From          int64  `json:"from"`
	To            int64  `json:"to"`
}

type Skipped struct {
	MeteringPoint string `json:"meteringPoint"`
	Reason        string `json:"reason"`
}

// Run is the content of the hidden RUN row.
type Run struct {
	Type      string       `json:"type"`
	RunId     string       `json:"runId"`
	EcId      string       `json:"ecId"`
	From      int64        `json:"from"`
	To        int64        `json:"to"`
	CreatedAt int64        `json:"createdAt"`
	Meters    []MeterRange `json:"meters"`
	Skipped   []Skipped    `json:"skipped"`
}

// Data is the content of the hidden DATA row: one energy delivery of a metering point of an open run.
type Data struct {
	Type          string `json:"type"`
	EcId          string `json:"ecId"`
	MeteringPoint string `json:"meteringPoint"`
	From          int64  `json:"from"`
	To            int64  `json:"to"`
}

type Rejected struct {
	MeteringPoint string   `json:"meteringPoint"`
	Codes         []string `json:"codes"`
}

type Incomplete struct {
	MeteringPoint string `json:"meteringPoint"`
	DataFrom      int64  `json:"dataFrom,omitempty"`
	DataTo        int64  `json:"dataTo,omitempty"`
}

// Summary is the content of the visible summary notification.
type Summary struct {
	Type       string       `json:"type"`
	RunId      string       `json:"runId"`
	From       int64        `json:"from"`
	To         int64        `json:"to"`
	Requested  int          `json:"requested"`
	Complete   int          `json:"complete"`
	Rejected   []Rejected   `json:"rejected"`
	NoAnswer   []string     `json:"noAnswer"`
	Incomplete []Incomplete `json:"incomplete"`
	Skipped    []Skipped    `json:"skipped"`
	Superseded []string     `json:"superseded"`
}

// Row is a notification row as far as the evaluation needs it.
type Row struct {
	Id      int64          `db:"id"`
	Tenant  string         `db:"tenant"`
	Process string         `db:"process"`
	Type    string         `db:"type"`
	Message types.JSONText `db:"notification"`
}

// answer is the content of an existing ANTWORT_PT / ABLEHNUNG_PT notification (database.SaveNotification).
type answer struct {
	Type           string   `json:"type"`
	MeteringPoints []string `json:"meteringPoints"`
	ResponseCodes  []string `json:"responseCodes"`
}

// Evaluate computes the summary of the run in row runRowId. rows are all relevant rows from runRowId on (ascending
// id); tenants are the tenants of the run's community: the answers are stored under one of them (a GEA can share the
// community id). done is true when every metering point has a final state or the deadline has passed.
func Evaluate(run Run, runRowId int64, rows []Row, tenants map[string]bool, now time.Time) (Summary, bool) {
	s := Summary{Type: SummaryType, RunId: run.RunId, From: run.From, To: run.To, Requested: len(run.Meters),
		Rejected: []Rejected{}, NoAnswer: []string{}, Incomplete: []Incomplete{}, Skipped: run.Skipped,
		Superseded: []string{}}
	if s.Skipped == nil {
		s.Skipped = []Skipped{}
	}

	open := 0
	for _, m := range run.Meters {
		// rows after a later run of the same community with this metering point belong to that run
		upper := nextRunWith(rows, runRowId, run.EcId, m.MeteringPoint)
		if upper > 0 {
			s.Superseded = append(s.Superseded, m.MeteringPoint)
			continue
		}

		var accepted bool
		var rejected []string
		var dataFrom, dataTo int64
		for _, r := range rows {
			if r.Id <= runRowId {
				continue
			}
			switch r.Type {
			case RowTypeData:
				var d Data
				if json.Unmarshal(r.Message, &d) != nil || !strings.EqualFold(d.EcId, run.EcId) || d.MeteringPoint != m.MeteringPoint {
					continue
				}
				if dataFrom == 0 || d.From < dataFrom {
					dataFrom = d.From
				}
				if d.To > dataTo {
					dataTo = d.To
				}
			case messageRow:
				if !tenants[r.Tenant] {
					continue
				}
				var a answer
				if json.Unmarshal(r.Message, &a) != nil || !contains(a.MeteringPoints, m.MeteringPoint) {
					continue
				}
				switch a.Type {
				case answerAccepted:
					accepted, rejected = true, nil
				case answerRejected:
					accepted, rejected = false, a.ResponseCodes
					if rejected == nil {
						rejected = []string{}
					}
				}
			}
		}

		switch {
		case dataFrom > 0 && covers(m, dataFrom, dataTo):
			s.Complete++
		case rejected != nil:
			s.Rejected = append(s.Rejected, Rejected{MeteringPoint: m.MeteringPoint, Codes: rejected})
		case accepted || dataFrom > 0:
			s.Incomplete = append(s.Incomplete, Incomplete{MeteringPoint: m.MeteringPoint, DataFrom: dataFrom, DataTo: dataTo})
			open++
		default:
			s.NoAnswer = append(s.NoAnswer, m.MeteringPoint)
			open++
		}
	}
	return s, open == 0 || now.Sub(time.UnixMilli(run.CreatedAt)) >= Deadline
}

// covers reports whether deliveries from..to cover the requested period of m. The requested bounds come from the
// request handler in local time: the start is a day start (or a stored date, i.e. UTC midnight), the end is the
// last quarter hour of the day (23:45) or, for an inactive date, UTC midnight of that date. Both are compared at the
// local day: the data must start at the local start of the first day and reach the last quarter hour of the last
// day, or the start of the inactive date.
func covers(m MeterRange, from, to int64) bool {
	return from <= startOfLocalDay(m.From).UnixMilli() && to >= requiredEnd(m.To).UnixMilli()
}

func startOfLocalDay(ms int64) time.Time {
	t := time.UnixMilli(ms)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func requiredEnd(ms int64) time.Time {
	t := time.UnixMilli(ms)
	if t.Hour() == 23 && t.Minute() == 45 {
		return t
	}
	return startOfLocalDay(ms)
}

// nextRunWith returns the row id of the first run after runRowId of the same community that requests meter, or 0.
func nextRunWith(rows []Row, runRowId int64, ecId, meter string) int64 {
	for _, r := range rows {
		if r.Id <= runRowId || r.Type != RowTypeRun {
			continue
		}
		var run Run
		if json.Unmarshal(r.Message, &run) != nil || !strings.EqualFold(run.EcId, ecId) {
			continue
		}
		for _, m := range run.Meters {
			if m.MeteringPoint == meter {
				return r.Id
			}
		}
	}
	return 0
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}

// Package energyrequest summarises an energy data request (ANFORDERUNG_PT) for several metering points in one
// message (platform#173). Nothing is stored outside base.notification: the request ("run") is a hidden notification
// row (process ENERGY_REQUEST), the answers are the existing ANTWORT_PT / ABLEHNUNG_PT notifications, and the
// summary is a visible notification. The state of each metering point is computed from these rows on every check.
// The energy data itself is not tracked here (the backend drops CR_MSG); web checks it from the summary on demand.
package energyrequest

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/jmoiron/sqlx/types"
)

const (
	RowTypeRun = "RUN"

	SummaryType = "ZUSAMMENFASSUNG_PT"

	// Deadline after which a run is summarised even if answers are missing.
	Deadline = 24 * time.Hour

	// Reasons why a requested metering point was not requested from the grid operator.
	ReasonNotFound          = "NOT_FOUND"
	ReasonNotRegistered     = "NOT_REGISTERED"
	ReasonNotAssigned       = "NOT_ASSIGNED"
	ReasonNoActivePeriod    = "NO_ACTIVE_PERIOD"
	ReasonNotActiveInPeriod = "NOT_ACTIVE_IN_PERIOD"

	answerAccepted = "ANTWORT_PT"
	answerRejected = "ABLEHNUNG_PT"
	// notification type of the answers and of the summary (model.N_TYPE_MESSAGE)
	messageRow = "MESSAGE"
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

type Rejected struct {
	MeteringPoint string   `json:"meteringPoint"`
	Codes         []string `json:"codes"`
}

// Summary is the content of the visible summary notification. Accepted carries the requested periods, so web can
// check the energy data of these metering points.
type Summary struct {
	Type       string       `json:"type"`
	RunId      string       `json:"runId"`
	From       int64        `json:"from"`
	To         int64        `json:"to"`
	Requested  int          `json:"requested"`
	Accepted   []MeterRange `json:"accepted"`
	Rejected   []Rejected   `json:"rejected"`
	NoAnswer   []string     `json:"noAnswer"`
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
// community id). done is true when every metering point has an answer or the deadline has passed.
func Evaluate(run Run, runRowId int64, rows []Row, tenants map[string]bool, now time.Time) (Summary, bool) {
	s := Summary{Type: SummaryType, RunId: run.RunId, From: run.From, To: run.To, Requested: len(run.Meters),
		Accepted: []MeterRange{}, Rejected: []Rejected{}, NoAnswer: []string{}, Skipped: run.Skipped,
		Superseded: []string{}}
	if s.Skipped == nil {
		s.Skipped = []Skipped{}
	}

	for _, m := range run.Meters {
		// answers after a later run of the same community with this metering point belong to that run
		if nextRunWith(rows, runRowId, run.EcId, m.MeteringPoint) > 0 {
			s.Superseded = append(s.Superseded, m.MeteringPoint)
			continue
		}

		var accepted bool
		var rejected []string
		for _, r := range rows {
			if r.Id <= runRowId || r.Type != messageRow || !tenants[r.Tenant] {
				continue
			}
			var a answer
			if json.Unmarshal(r.Message, &a) != nil || !contains(a.MeteringPoints, m.MeteringPoint) {
				continue
			}
			// the latest answer counts
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

		switch {
		case rejected != nil:
			s.Rejected = append(s.Rejected, Rejected{MeteringPoint: m.MeteringPoint, Codes: rejected})
		case accepted:
			s.Accepted = append(s.Accepted, m)
		default:
			s.NoAnswer = append(s.NoAnswer, m.MeteringPoint)
		}
	}
	return s, len(s.NoAnswer) == 0 || now.Sub(time.UnixMilli(run.CreatedAt)) >= Deadline
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

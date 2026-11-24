package model

import (
	"strings"

	"gopkg.in/guregu/null.v4"
)

// Planner of the bulk change (konzept-massenaenderungen.md, Fachliche Logik 4–8, Tech Design E1): preview and
// apply run the same planner on a snapshot of the community; it only computes, the DAO reads and writes.

// Classes of a hit.
const (
	BulkClassChanged            = "CHANGED"
	BulkClassUnchangedCondition = "UNCHANGED_CONDITION"
	BulkClassUnchangedTarget    = "UNCHANGED_TARGET"
	BulkClassError              = "ERROR"
	// BulkClassSkipped: apply only, a hit that would change but is not in the request or no longer matches its
	// expected values; it is not written and not counted.
	BulkClassSkipped = "SKIPPED"
)

// BulkSkipChanged is the skip reason of a requested hit whose state differs from the preview.
const BulkSkipChanged = "CHANGED_SINCE_PREVIEW"

// BulkEeg is the community of the run.
type BulkEeg struct {
	Tenant   string `db:"tenant"`
	Name     string `db:"name"`
	RcNumber string `db:"rcNumber"`
	// CreditorNorm is the creditor id without blanks, upper-cased (built in SQL); "" without creditor id.
	CreditorNorm string `db:"creditor_norm"`
	HasIban      bool   `db:"has_iban"`
	HasBic       bool   `db:"has_bic"`
}

// BulkMember is a member with its bank account row (BankId invalid: no row). Dates are ISO strings.
type BulkMember struct {
	Id           string      `db:"id"`
	Number       null.String `db:"participant_number"`
	Since        null.String `db:"participant_since"`
	Firstname    string      `db:"firstname"`
	Lastname     string      `db:"lastname"`
	BusinessRole string      `db:"business_role"`
	TariffId     null.String `db:"tariff_id"`
	BankId       null.String `db:"bank_id"`
	HasIban      bool        `db:"has_iban"`
	MandateRef   null.String `db:"mandate_reference"`
	MandateDate  null.String `db:"mandate_date"`
	DebitType    null.String `db:"sepa_direct_debit"`
}

// BulkMeter is a current metering point (flag 1) of the rule's direction.
type BulkMeter struct {
	ParticipantId   string      `db:"participant_id"`
	MeteringPointId string      `db:"metering_point_id"`
	Direction       string      `db:"direction"`
	TariffId        null.String `db:"tariff_id"`
}

// BulkTariff is the latest version of a tariff of the community; Active as in the view activetariff.
type BulkTariff struct {
	Id               string     `db:"id"`
	Name             string     `db:"name"`
	Type             string     `db:"type"`
	Active           bool       `db:"active"`
	UseVat           bool       `db:"use_vat"`
	Vat              float64    `db:"vat"`
	UseMeterFee      bool       `db:"use_meter_fee"`
	MeterVat         null.Float `db:"meter_vat"`
	JoiningFee       float64    `db:"joining_fee"`
	JoiningFeeUseVat bool       `db:"joining_fee_use_vat"`
	JoiningFeeVat    float64    `db:"joining_fee_vat"`
}

// BulkSnapshot is what the planner sees of the community.
type BulkSnapshot struct {
	Eeg     *BulkEeg
	Members []*BulkMember
	// Meters: METER_TARIFF only.
	Meters []*BulkMeter
	// Tariffs by lower-case id.
	Tariffs map[string]*BulkTariff
	// ForeignRefs: normalised effective mandate references of direct debit members of the other communities with
	// the same normalised creditor id (DEBIT_TYPE and MANDATE_REFERENCE only).
	ForeignRefs map[string]bool
}

// BulkRow is one hit of the plan.
type BulkRow struct {
	ParticipantId     string   `json:"participantId"`
	MeteringPointId   string   `json:"meteringPointId,omitempty"`
	ParticipantNumber string   `json:"participantNumber"`
	Name              string   `json:"name"`
	OldValue          *string  `json:"oldValue"`
	NewValue          *string  `json:"newValue"`
	OldText           string   `json:"oldText"`
	NewText           string   `json:"newText"`
	Class             string   `json:"class"`
	Hints             []string `json:"hints"`
	Error             string   `json:"error,omitempty"`

	// BankId is the bank account row the DAO writes (bank fields only).
	BankId string      `json:"-"`
	member *BulkMember // nil never
	meter  *BulkMeter  // METER_TARIFF only
}

// BulkCounts are the counters of the preview.
type BulkCounts struct {
	Changed            int `json:"changed"`
	WithHint           int `json:"withHint"`
	UnchangedCondition int `json:"unchangedCondition"`
	UnchangedTarget    int `json:"unchangedTarget"`
	Errors             int `json:"errors"`
}

// BulkPlan is the answer of the preview and the base of apply.
type BulkPlan struct {
	Counts   BulkCounts `json:"counts"`
	RunHints []string   `json:"runHints"`
	Rows     []*BulkRow `json:"rows"`
}

// BulkSkip is a requested hit apply did not write.
type BulkSkip struct {
	ParticipantId   string `json:"participantId"`
	MeteringPointId string `json:"meteringPointId,omitempty"`
	Reason          string `json:"reason"`
}

// BulkApplyResult is the answer of apply: the written hits with their hints, the skipped ones with the reason.
type BulkApplyResult struct {
	Changed  int         `json:"changed"`
	Skipped  []*BulkSkip `json:"skipped"`
	RunHints []string    `json:"runHints"`
	Rows     []*BulkRow  `json:"rows"`
}

// Key is the BulkItemKey of the row.
func (r *BulkRow) Key() string { return BulkItemKey(r.ParticipantId, r.MeteringPointId) }

// PlanBulkChange plans the rule (validated) on the snapshot. today is the ISO date of the day (F3). keep is nil
// for the preview; apply passes a filter: a hit that would change but is not kept becomes BulkClassSkipped
// before uniqueness and hints are computed, so the plan describes exactly what apply writes.
func PlanBulkChange(rule *BulkRule, s *BulkSnapshot, today string, keep func(*BulkRow) bool) *BulkPlan {
	ev := newBulkEvaluator(rule, s, today)
	var rows []*BulkRow
	if rule.Field == BulkMeterTariff {
		members := make(map[string]*BulkMember, len(s.Members))
		for _, m := range s.Members {
			members[m.Id] = m
		}
		for _, mt := range s.Meters {
			if m := members[mt.ParticipantId]; m != nil && strings.EqualFold(mt.Direction, rule.Direction) {
				rows = append(rows, ev.row(m, mt))
			}
		}
	} else {
		for _, m := range s.Members {
			rows = append(rows, ev.row(m, nil))
		}
	}
	if keep != nil {
		for _, r := range rows {
			if r.Class == BulkClassChanged && !keep(r) {
				r.Class = BulkClassSkipped
			}
		}
	}
	ev.uniqueness(rows)
	plan := &BulkPlan{RunHints: ev.runHints(), Rows: rows}
	for _, r := range rows {
		if r.Class == BulkClassChanged {
			r.Hints = ev.hints(r)
		}
		if r.Hints == nil {
			r.Hints = []string{}
		}
		switch r.Class {
		case BulkClassChanged:
			plan.Counts.Changed++
			if len(r.Hints) > 0 {
				plan.Counts.WithHint++
			}
		case BulkClassUnchangedCondition:
			plan.Counts.UnchangedCondition++
		case BulkClassUnchangedTarget:
			plan.Counts.UnchangedTarget++
		case BulkClassError:
			plan.Counts.Errors++
		}
	}
	return plan
}

func memberName(m *BulkMember) string {
	return strings.TrimSpace(strings.TrimSpace(m.Firstname) + " " + strings.TrimSpace(m.Lastname))
}

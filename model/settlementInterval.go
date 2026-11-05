package model

import (
	"fmt"
	"regexp"
	"strconv"
)

// The settlement intervals of base.eeg."settlementInterval".
const (
	SettlementMonthly  = "MONTHLY"
	SettlementQuarter  = "QUARTER"
	SettlementBiannual = "BIANNUAL"
	SettlementAnnual   = "ANNUAL"
)

// periodMonths is the length of one period of each interval, in months.
var periodMonths = map[string]int{SettlementMonthly: 1, SettlementQuarter: 3, SettlementBiannual: 6, SettlementAnnual: 12}

// IsSettlementInterval reports whether v is one of the four intervals.
func IsSettlementInterval(v string) bool {
	_, ok := periodMonths[v]
	return ok
}

// BillingState is billing's settlementState, passed on by web (konzept-abrechnungsintervall.md): the clearing period
// identifiers of the final runs with documents and of the running runs.
type BillingState struct {
	FinalPeriods   []string `json:"finalPeriods"`
	RunningPeriods []string `json:"runningPeriods"`
}

// SettlementIntervalDecision is the result of the change rule. Dates are ISO dates (2026-04-01); nil
// EffectiveFrom on an allowed change = applies at once.
type SettlementIntervalDecision struct {
	Allowed         bool    `json:"allowed"`
	EffectiveFrom   *string `json:"effectiveFrom"`
	LastFinalPeriod *string `json:"lastFinalPeriod"`
	Message         string  `json:"message"`
	EarliestFrom    *string `json:"earliestFrom"`
}

// clearingPeriodPattern is billing's ClearingPeriodMonths.IDENTIFIER.
var clearingPeriodPattern = regexp.MustCompile(`^Abr_(Y|YH|YQ|YM)-(\d{4})(?:-(\d{1,2}))?$`)

// identifierMonths is the length of one period of each identifier type, in months.
var identifierMonths = map[string]int{"Y": 12, "YH": 6, "YQ": 3, "YM": 1}

// clearingPeriod is the months an identifier covers, counted as year*12 + month-1.
type clearingPeriod struct {
	kind        string
	first, last int
}

// parseClearingPeriod mirrors billing's ClearingPeriodMonths.of: false for an identifier of another form.
func parseClearingPeriod(id string) (clearingPeriod, bool) {
	m := clearingPeriodPattern.FindStringSubmatch(id)
	if m == nil {
		return clearingPeriod{}, false
	}
	year, _ := strconv.Atoi(m[2])
	period := 0
	if m[3] != "" {
		period, _ = strconv.Atoi(m[3])
	}
	length := identifierMonths[m[1]]
	if m[1] == "Y" {
		period = 1
	} else if period < 1 || period > 12/length {
		return clearingPeriod{}, false
	}
	first := year*12 + (period-1)*length
	return clearingPeriod{kind: m[1], first: first, last: first + length - 1}, true
}

var monthNames = [12]string{"Jänner", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September",
	"Oktober", "November", "Dezember"}

func monthName(m int) string { return fmt.Sprintf("%s %d", monthNames[m%12], m/12) }
func isoDate(m int) *string  { s := fmt.Sprintf("%04d-%02d-01", m/12, m%12+1); return &s }
func longDate(m int) string  { return fmt.Sprintf("1. %s %d", monthNames[m%12], m/12) }

// monthRange names the months first..last of one year: "März 2026", "März bis Mai 2026". Every interval starts a
// period on 1 January, so a blocked range never crosses the year end.
func monthRange(first, last int) string {
	if first == last {
		return monthName(first)
	}
	return fmt.Sprintf("%s bis %s", monthNames[first%12], monthName(last))
}

// name is the period in words: "März 2026", "1. Quartal 2026", "1. Halbjahr 2026", "2026".
func (p clearingPeriod) name() string {
	year, index := p.first/12, p.first%12
	switch p.kind {
	case "YM":
		return monthName(p.first)
	case "YQ":
		return fmt.Sprintf("%d. Quartal %d", index/3+1, year)
	case "YH":
		return fmt.Sprintf("%d. Halbjahr %d", index/6+1, year)
	default:
		return strconv.Itoa(year)
	}
}

// DecideSettlementInterval applies the change rule of konzept-abrechnungsintervall.md (Tech Design, "Regel im
// backend") to a valid new interval. Without a final period the change applies at once (R5). Otherwise L is the last
// month of the final periods and S the month after it: allowed from S when S starts a period of the new interval
// (R1, R2), else blocked with the next start after L; also blocked while a run for a period ending in S or later is
// running (R3), without a final period while any run is running. Identifiers of another form are ignored, as in
// billing.
func DecideSettlementInterval(newInterval string, state BillingState) SettlementIntervalDecision {
	last, lastId := -1, ""
	for _, id := range state.FinalPeriods {
		if p, ok := parseClearingPeriod(id); ok && p.last > last {
			last, lastId = p.last, id
		}
	}
	if last < 0 {
		// a final run finishing after a change at once would overlap the first period of the new interval: any
		// running run blocks (R3)
		if p, ok := runningFrom(state, 0); ok {
			return SettlementIntervalDecision{Message: runningMessage(p)}
		}
		return SettlementIntervalDecision{Allowed: true,
			Message: "Das neue Intervall gilt sofort, es gibt noch keine endgültige Abrechnung."}
	}

	start := last + 1
	length := periodMonths[newInterval]
	d := SettlementIntervalDecision{LastFinalPeriod: &lastId}
	if start%length != 0 {
		earliest := (start/length + 1) * length
		d.EarliestFrom = isoDate(earliest)
		d.Message = fmt.Sprintf("Zuerst muss noch der Zeitraum %s im bisherigen Intervall abgerechnet werden. Ein Wechsel ist dann ab %s möglich.",
			monthRange(start, earliest-1), longDate(earliest))
		return d
	}
	if p, ok := runningFrom(state, start); ok {
		d.Message = runningMessage(p)
		return d
	}
	d.Allowed = true
	d.EffectiveFrom = isoDate(start)
	d.Message = fmt.Sprintf("Das neue Intervall gilt ab %s.", longDate(start))
	return d
}

// runningFrom returns the first running period that ends in month start or later.
func runningFrom(state BillingState, start int) (clearingPeriod, bool) {
	for _, id := range state.RunningPeriods {
		if p, ok := parseClearingPeriod(id); ok && p.last >= start {
			return p, true
		}
	}
	return clearingPeriod{}, false
}

func runningMessage(p clearingPeriod) string {
	return fmt.Sprintf("Für %s läuft gerade eine Berechnung. Bitte nach deren Ende erneut versuchen.", p.name())
}

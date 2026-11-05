package model

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func str(s string) *string { return &s }

func TestDecideSettlementInterval(t *testing.T) {
	cases := []struct {
		name     string
		interval string
		final    []string
		running  []string
		want     SettlementIntervalDecision
	}{
		{"no final period: at once (R5)", SettlementAnnual, nil, []string{},
			SettlementIntervalDecision{Allowed: true, Message: "Das neue Intervall gilt sofort, es gibt noch keine endgültige Abrechnung."}},
		{"no final period, a run running: blocked (R3)", SettlementAnnual, nil, []string{"Abr_YM-2026-4"},
			SettlementIntervalDecision{Message: "Für April 2026 läuft gerade eine Berechnung. Bitte nach deren Ende erneut versuchen."}},
		{"monthly to March, quarterly from April", SettlementQuarter, []string{"Abr_YM-2026-2", "Abr_YM-2026-3", "Abr_YM-2026-1"}, nil,
			SettlementIntervalDecision{Allowed: true, EffectiveFrom: str("2026-04-01"), LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Das neue Intervall gilt ab 1. April 2026."}},
		{"monthly to February, quarterly blocked until April (R2)", SettlementQuarter, []string{"Abr_YM-2026-1", "Abr_YM-2026-2"}, nil,
			SettlementIntervalDecision{EarliestFrom: str("2026-04-01"), LastFinalPeriod: str("Abr_YM-2026-2"),
				Message: "Zuerst muss noch der Zeitraum März 2026 im bisherigen Intervall abgerechnet werden. Ein Wechsel ist dann ab 1. April 2026 möglich."}},
		{"monthly to June, annual blocked until next January", SettlementAnnual, []string{"Abr_YM-2026-6"}, nil,
			SettlementIntervalDecision{EarliestFrom: str("2027-01-01"), LastFinalPeriod: str("Abr_YM-2026-6"),
				Message: "Zuerst muss noch der Zeitraum Juli bis Dezember 2026 im bisherigen Intervall abgerechnet werden. Ein Wechsel ist dann ab 1. Jänner 2027 möglich."}},
		{"monthly to November, biannual: months across the year end", SettlementBiannual, []string{"Abr_YM-2025-10"}, nil,
			SettlementIntervalDecision{EarliestFrom: str("2026-01-01"), LastFinalPeriod: str("Abr_YM-2025-10"),
				Message: "Zuerst muss noch der Zeitraum November bis Dezember 2025 im bisherigen Intervall abgerechnet werden. Ein Wechsel ist dann ab 1. Jänner 2026 möglich."}},
		{"quarterly to December, annual from January", SettlementAnnual, []string{"Abr_YQ-2025-4"}, nil,
			SettlementIntervalDecision{Allowed: true, EffectiveFrom: str("2026-01-01"), LastFinalPeriod: str("Abr_YQ-2025-4"),
				Message: "Das neue Intervall gilt ab 1. Jänner 2026."}},
		{"back to monthly after a quarter", SettlementMonthly, []string{"Abr_YQ-2026-1"}, nil,
			SettlementIntervalDecision{Allowed: true, EffectiveFrom: str("2026-04-01"), LastFinalPeriod: str("Abr_YQ-2026-1"),
				Message: "Das neue Intervall gilt ab 1. April 2026."}},
		{"half year to June, quarterly from July", SettlementQuarter, []string{"Abr_YH-2026-1"}, nil,
			SettlementIntervalDecision{Allowed: true, EffectiveFrom: str("2026-07-01"), LastFinalPeriod: str("Abr_YH-2026-1"),
				Message: "Das neue Intervall gilt ab 1. Juli 2026."}},
		{"running period from S on blocks (R3)", SettlementQuarter, []string{"Abr_YM-2026-3"}, []string{"Abr_YM-2026-4"},
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Für April 2026 läuft gerade eine Berechnung. Bitte nach deren Ende erneut versuchen."}},
		{"running quarter ending after S blocks, named as quarter", SettlementMonthly, []string{"Abr_YM-2026-3"}, []string{"Abr_YQ-2026-2"},
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Für 2. Quartal 2026 läuft gerade eine Berechnung. Bitte nach deren Ende erneut versuchen."}},
		{"running period before S does not block", SettlementQuarter, []string{"Abr_YM-2026-3"}, []string{"Abr_YM-2026-2", "Abr_YQ-2026-1"},
			SettlementIntervalDecision{Allowed: true, EffectiveFrom: str("2026-04-01"), LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Das neue Intervall gilt ab 1. April 2026."}},
		{"mixed types: year 2025 and February 2026, January a gap (rule 6)", SettlementQuarter, []string{"Abr_YM-2026-2", "Abr_Y-2025"}, nil,
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YM-2026-2"),
				Message: "Jänner 2026 ist noch nicht endgültig abgerechnet."}},
		{"mixed types: year 2025 to February 2026 without a gap, R2", SettlementQuarter, []string{"Abr_YM-2026-2", "Abr_Y-2025", "Abr_YM-2026-1"}, nil,
			SettlementIntervalDecision{EarliestFrom: str("2026-04-01"), LastFinalPeriod: str("Abr_YM-2026-2"),
				Message: "Zuerst muss noch der Zeitraum März 2026 im bisherigen Intervall abgerechnet werden. Ein Wechsel ist dann ab 1. April 2026 möglich."}},
		{"mixed types: year 2025 last, monthly from January", SettlementMonthly, []string{"Abr_YM-2025-2", "Abr_Y-2025"}, nil,
			SettlementIntervalDecision{Allowed: true, EffectiveFrom: str("2026-01-01"), LastFinalPeriod: str("Abr_Y-2025"),
				Message: "Das neue Intervall gilt ab 1. Jänner 2026."}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DecideSettlementInterval(c.interval, BillingState{FinalPeriods: c.final, RunningPeriods: c.running})
			assert.Equal(t, c.want, got)
		})
	}
}

// Rule 6 (eegfaktura-dev#54 #2): a month between the first month of the first final period and L without a final
// period blocks the change, before the R2 boundary; the message names the gap. Months before the first final period
// do not count.
func TestDecideSettlementIntervalGaps(t *testing.T) {
	cases := []struct {
		name     string
		interval string
		final    []string
		running  []string
		want     SettlementIntervalDecision
	}{
		{"January and March final, February never billed", SettlementQuarter, []string{"Abr_YM-2026-1", "Abr_YM-2026-3"}, nil,
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Februar 2026 ist noch nicht endgültig abgerechnet."}},
		{"two gaps in one year", SettlementQuarter, []string{"Abr_YM-2026-6", "Abr_YM-2026-1", "Abr_YM-2026-3", "Abr_YM-2026-4"}, nil,
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YM-2026-6"),
				Message: "Februar und Mai 2026 sind noch nicht endgültig abgerechnet."}},
		{"a range of months", SettlementMonthly, []string{"Abr_YQ-2026-1", "Abr_YQ-2026-4"}, nil,
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YQ-2026-4"),
				Message: "April bis September 2026 sind noch nicht endgültig abgerechnet."}},
		{"a range across the year end", SettlementQuarter, []string{"Abr_YM-2025-10", "Abr_YM-2026-2"}, nil,
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YM-2026-2"),
				Message: "November 2025 bis Jänner 2026 sind noch nicht endgültig abgerechnet."}},
		{"gaps in two years", SettlementQuarter, []string{"Abr_YM-2025-11", "Abr_YM-2026-1", "Abr_YM-2026-3"}, nil,
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Dezember 2025 und Februar 2026 sind noch nicht endgültig abgerechnet."}},
		{"three gaps", SettlementAnnual, []string{"Abr_YM-2026-1", "Abr_YM-2026-3", "Abr_YM-2026-5", "Abr_YM-2026-8"}, nil,
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YM-2026-8"),
				Message: "Februar, April und Juni bis Juli 2026 sind noch nicht endgültig abgerechnet."}},
		{"the gap before the R2 boundary", SettlementAnnual, []string{"Abr_YM-2026-1", "Abr_YM-2026-3"}, nil,
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Februar 2026 ist noch nicht endgültig abgerechnet."}},
		{"the gap before a running run (R3)", SettlementQuarter, []string{"Abr_YM-2026-1", "Abr_YM-2026-3"}, []string{"Abr_YM-2026-4"},
			SettlementIntervalDecision{LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Februar 2026 ist noch nicht endgültig abgerechnet."}},
		{"months before the first final period do not count", SettlementQuarter, []string{"Abr_YM-2026-3"}, nil,
			SettlementIntervalDecision{Allowed: true, EffectiveFrom: str("2026-04-01"), LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Das neue Intervall gilt ab 1. April 2026."}},
		{"overlapping types cover each other", SettlementMonthly, []string{"Abr_YQ-2026-1", "Abr_YM-2026-2", "Abr_YM-2026-4"}, nil,
			SettlementIntervalDecision{Allowed: true, EffectiveFrom: str("2026-05-01"), LastFinalPeriod: str("Abr_YM-2026-4"),
				Message: "Das neue Intervall gilt ab 1. Mai 2026."}},
		{"a quarter after months, no gap", SettlementAnnual, []string{"Abr_YM-2025-10", "Abr_YM-2025-11", "Abr_YM-2025-12", "Abr_YQ-2026-1"}, nil,
			SettlementIntervalDecision{EarliestFrom: str("2027-01-01"), LastFinalPeriod: str("Abr_YQ-2026-1"),
				Message: "Zuerst muss noch der Zeitraum April bis Dezember 2026 im bisherigen Intervall abgerechnet werden. Ein Wechsel ist dann ab 1. Jänner 2027 möglich."}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DecideSettlementInterval(c.interval, BillingState{FinalPeriods: c.final, RunningPeriods: c.running})
			assert.Equal(t, c.want, got)
		})
	}
}

// The identifier forms of billing's ClearingPeriodMonths.of.
func TestParseClearingPeriod(t *testing.T) {
	for id, want := range map[string][2]int{
		"Abr_Y-2025":     {2025 * 12, 2025*12 + 11},
		"Abr_Y-2025-7":   {2025 * 12, 2025*12 + 11}, // billing ignores the number of a year
		"Abr_YH-2026-2":  {2026*12 + 6, 2026*12 + 11},
		"Abr_YQ-2026-3":  {2026*12 + 6, 2026*12 + 8},
		"Abr_YM-2026-12": {2026*12 + 11, 2026*12 + 11},
	} {
		p, ok := parseClearingPeriod(id)
		assert.True(t, ok, id)
		assert.Equal(t, want, [2]int{p.first, p.last}, id)
	}
	for _, id := range []string{"", "Abr_YM-2026", "Abr_YM-2026-0", "Abr_YH-2026-3", "Abr_YQ-2026-5", "Abr_YM-26-1",
		" Abr_YM-2026-1", "Abr_YM-2026-1x", "Abr_YW-2026-1", "Abr_Y-9999", "Abr_Y-1999", "Abr_YM-2101-1"} {
		_, ok := parseClearingPeriod(id)
		assert.False(t, ok, id)
	}
}

// eegfaktura-dev#54 #9: an incomplete or unreadable billingState is refused instead of reading as "nothing billed".
func TestCheckBillingState(t *testing.T) {
	assert.EqualError(t, CheckBillingState(nil), "Der Abrechnungsstand (billingState) fehlt")
	for _, s := range []BillingState{{}, {FinalPeriods: []string{}}, {RunningPeriods: []string{}}} {
		assert.EqualError(t, CheckBillingState(&s), "Der Abrechnungsstand (billingState) ist unvollständig (finalPeriods, runningPeriods)")
	}
	for _, id := range []string{"Abr_YM-2026-13", "ABR_YM-2026-3", "Abr_YM-2026-3 ", "foo", "Abr_Y-9999", "Abr_YQ-1999-1", ""} {
		for _, s := range []BillingState{{FinalPeriods: []string{"Abr_YM-2026-1", id}, RunningPeriods: []string{}},
			{FinalPeriods: []string{}, RunningPeriods: []string{id}}} {
			assert.EqualError(t, CheckBillingState(&s), "Der Abrechnungsstand (billingState) enthält eine unlesbare Zeitraum-Kennung: "+strconv.Quote(id), id)
		}
	}
	// several: named, at most five, each cut to 40 characters
	long := strings.Repeat("x", 50)
	assert.EqualError(t, CheckBillingState(&BillingState{FinalPeriods: []string{"a", "Abr_YM-2026-1", long},
		RunningPeriods: []string{"b", "c", "d", "e"}}),
		`Der Abrechnungsstand (billingState) enthält unlesbare Zeitraum-Kennungen: "a", "`+strings.Repeat("x", 40)+`…", "b", "c", "d", …`)
	// the forms billing and web produce (web: createPeriodIdentifier, also a number after a year)
	assert.NoError(t, CheckBillingState(&BillingState{FinalPeriods: []string{}, RunningPeriods: []string{}}))
	assert.NoError(t, CheckBillingState(&BillingState{
		FinalPeriods:   []string{"Abr_YM-2026-3", "Abr_YM-2026-12", "Abr_YM-2026-03", "Abr_YQ-2026-1", "Abr_YH-2026-2", "Abr_Y-2025", "Abr_Y-2025-1"},
		RunningPeriods: []string{"Abr_YM-2000-1", "Abr_Y-2100"}}))
}

func TestIsSettlementInterval(t *testing.T) {
	for _, v := range []string{"MONTHLY", "QUARTER", "BIANNUAL", "ANNUAL"} {
		assert.True(t, IsSettlementInterval(v), v)
	}
	for _, v := range []string{"", "monthly", "YEARLY", "QUARTERLY"} {
		assert.False(t, IsSettlementInterval(v), v)
	}
}

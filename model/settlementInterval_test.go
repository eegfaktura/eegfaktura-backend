package model

import (
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
		{"no final period, a run running: blocked (R3)", SettlementAnnual, nil, []string{"kaputt", "Abr_YM-2026-4"},
			SettlementIntervalDecision{Message: "Für April 2026 läuft gerade eine Berechnung. Bitte nach deren Ende erneut versuchen."}},
		{"no final period, only unparseable running: at once", SettlementQuarter, []string{"Rechnung 3"}, []string{"kaputt"},
			SettlementIntervalDecision{Allowed: true, Message: "Das neue Intervall gilt sofort, es gibt noch keine endgültige Abrechnung."}},
		{"only identifiers of another form: at once", SettlementQuarter, []string{"Abr_YM-2026-13", "Abr_YQ-2026-5", "abr_YM-2026-3", "Rechnung 3"}, nil,
			SettlementIntervalDecision{Allowed: true, Message: "Das neue Intervall gilt sofort, es gibt noch keine endgültige Abrechnung."}},
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
		{"running period before S does not block", SettlementQuarter, []string{"Abr_YM-2026-3"}, []string{"Abr_YM-2026-2", "Abr_YQ-2026-1", "kaputt"},
			SettlementIntervalDecision{Allowed: true, EffectiveFrom: str("2026-04-01"), LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Das neue Intervall gilt ab 1. April 2026."}},
		{"unparseable identifiers ignored beside valid ones", SettlementQuarter, []string{"Abr_YM-2026-3", "Abr_YM-2027-13", "Abr_YX-2030"}, nil,
			SettlementIntervalDecision{Allowed: true, EffectiveFrom: str("2026-04-01"), LastFinalPeriod: str("Abr_YM-2026-3"),
				Message: "Das neue Intervall gilt ab 1. April 2026."}},
		{"mixed types: year 2025 and February 2026", SettlementQuarter, []string{"Abr_YM-2026-2", "Abr_Y-2025"}, nil,
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
		" Abr_YM-2026-1", "Abr_YM-2026-1x", "Abr_YW-2026-1"} {
		_, ok := parseClearingPeriod(id)
		assert.False(t, ok, id)
	}
}

func TestIsSettlementInterval(t *testing.T) {
	for _, v := range []string{"MONTHLY", "QUARTER", "BIANNUAL", "ANNUAL"} {
		assert.True(t, IsSettlementInterval(v), v)
	}
	for _, v := range []string{"", "monthly", "YEARLY", "QUARTERLY"} {
		assert.False(t, IsSettlementInterval(v), v)
	}
}

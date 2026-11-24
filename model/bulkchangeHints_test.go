package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
)

// Every hint in its own case, and only there.
func TestPlanBulk_hints(t *testing.T) {
	none := debit("NONE")
	cases := []struct {
		name   string
		rule   BulkRule
		member *BulkMember
		others []*BulkMember
		want   []string
	}{
		{"H1 no IBAN", debitToCore, bulkMember("1", none, func(m *BulkMember) { m.HasIban = false }), nil, []string{"H1"}},
		{"H2 reference ''", debitToCore, bulkMember("1", none, ref(str(""))), nil, []string{"H2"}},
		{"H2 reference NULL, no member number", debitToCore, bulkMember("1", none, ref(nil), func(m *BulkMember) { m.Number = null.String{} }), nil, []string{"H2"}},
		{"H3 no mandate date and no member since", debitToCore, bulkMember("1", none, func(m *BulkMember) { m.MandateDate, m.Since = null.String{}, null.String{} }), nil, []string{"H3"}},
		{"H4 invalid reference becomes effective", debitToCore, bulkMember("1", none, ref(str("M_1"))), nil, []string{"H4"}},
		{"H10 a reference with a blank becomes effective", debitToCore, bulkMember("1", none, ref(str("M 1"))), nil, []string{"H10"}},
		{"H10 a reference with a blank is written", copyRef, bulkMember("A 1", none), nil, []string{"H10"}},
		{"no H10 for a non-debit run on a blank reference", BulkRule{Field: BulkDebitType, Action: BulkSet, Value: "NONE", Condition: BulkAll},
			bulkMember("1", ref(str("M 1"))), nil, []string{}},
		{"H4 duplicate reference becomes effective", debitToCore, bulkMember("1", none, ref(str("m-9"))), []*BulkMember{bulkMember("9")}, []string{"H4"}},
		{"H5 B2B for a private member", BulkRule{Field: BulkDebitType, Action: BulkSet, Value: "B2B", Condition: BulkAll},
			bulkMember("1", none, func(m *BulkMember) { m.BusinessRole = "EEG_PRIVATE" }), nil, []string{"H5"}},
		{"H6 reference of a direct debit member changes", copyRef, bulkMember("1"), nil, []string{"H6"}},
		{"no H6 without direct debit", copyRef, bulkMember("1", none), nil, []string{}},
		{"no H6 when the effective reference stays", copyRef, bulkMember("1", ref(nil)), nil, []string{}},
		{"H9 no member tariff", BulkRule{Field: BulkParticipantTariff, Action: BulkClear, Condition: BulkAll},
			bulkMember("1", func(m *BulkMember) { m.TariffId = null.StringFrom(bulkTariffId) }), nil, []string{"H9"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := bulkPlan(t, c.rule, bulkSnap(append([]*BulkMember{c.member}, c.others...)...))
			r := bulkRowOf(t, p, c.member.Id)
			require.Equal(t, BulkClassChanged, r.Class, r.Error)
			assert.Equal(t, c.want, r.Hints)
			assert.Equal(t, []string{"L3"}, p.RunHints)
			if len(c.want) > 0 {
				assert.Equal(t, 1, p.Counts.WithHint)
			}
		})
	}

	t.Run("H7 and H8 joining fee", func(t *testing.T) {
		rule := BulkRule{Field: BulkParticipantTariff, Action: BulkSet, Value: bulkTariffId, Condition: BulkAll}
		other := &BulkTariff{Id: "other", Type: "VZP", Active: true, UseVat: true, Vat: 20}
		for name, c := range map[string]struct {
			fee  func(*BulkTariff)
			want []string
		}{
			"fee without VAT":     {func(t *BulkTariff) { t.JoiningFee = 50 }, []string{"H7"}},
			"fee with a used VAT": {func(t *BulkTariff) { t.JoiningFee, t.JoiningFeeUseVat, t.JoiningFeeVat = 50, true, 20 }, []string{"H7"}},
			"fee with a new VAT":  {func(t *BulkTariff) { t.JoiningFee, t.JoiningFeeUseVat, t.JoiningFeeVat = 50, true, 13 }, []string{"H7", "H8"}},
			"no fee":              {func(t *BulkTariff) { t.JoiningFeeUseVat, t.JoiningFeeVat = true, 13 }, []string{}},
		} {
			t.Run(name, func(t *testing.T) {
				s := bulkSnap(bulkMember("1"))
				s.Tariffs[bulkTariffId] = bulkTariff("EEG", c.fee)
				s.Tariffs["other"] = other
				assert.Equal(t, c.want, bulkRowOf(t, bulkPlan(t, rule, s), "p-1").Hints)
			})
		}
	})

	t.Run("H9 no meter tariff", func(t *testing.T) {
		s := bulkSnap(bulkMember("1"))
		s.Meters = []*BulkMeter{{ParticipantId: "p-1", MeteringPointId: "AT1", Direction: "CONSUMPTION", TariffId: null.StringFrom(bulkTariffId)}}
		p := bulkPlan(t, BulkRule{Field: BulkMeterTariff, Action: BulkClear, Condition: BulkAll, Direction: "CONSUMPTION"}, s)
		assert.Equal(t, []string{"H9"}, p.Rows[0].Hints)
	})
}

func TestPlanBulk_runHints(t *testing.T) {
	cases := map[string]struct {
		eeg  func(*BulkEeg)
		rule BulkRule
		want []string
	}{
		"L1 no BIC":                {func(e *BulkEeg) { e.HasBic = false }, debitToCore, []string{"L1", "L3"}},
		"L1 no IBAN":               {func(e *BulkEeg) { e.HasIban = false }, debitToCore, []string{"L1", "L3"}},
		"L2 no creditor id":        {func(e *BulkEeg) { e.CreditorNorm = "" }, debitToCore, []string{"L2", "L3"}},
		"no L1/L2 for NONE":        {func(e *BulkEeg) { e.HasBic, e.CreditorNorm = false, "" }, BulkRule{Field: BulkDebitType, Action: BulkSet, Value: "NONE", Condition: BulkAll}, []string{"L3"}},
		"no L1/L2 for a reference": {func(e *BulkEeg) { e.HasBic, e.CreditorNorm = false, "" }, copyRef, []string{"L3"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s := bulkSnap(bulkMember("1", debit("NONE")))
			c.eeg(s.Eeg)
			assert.Equal(t, c.want, bulkPlan(t, c.rule, s).RunHints)
		})
	}
}

package model

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bulkTariffId = "5f0e8a52-6c8e-4c39-9d1e-3b7f5c2a9e10"

func bulkCode(t *testing.T, err error) int {
	t.Helper()
	var ve *VfeegError
	require.True(t, errors.As(err, &ve), "want a VfeegError, got %v", err)
	return ve.Code
}

// The matrix of the Tech Design (A): allowed rules pass and are normalised.
func TestBulkRuleValidate_allowed(t *testing.T) {
	cases := []struct {
		name string
		rule BulkRule
		want BulkRule
	}{
		{"debit set equals", BulkRule{Field: BulkDebitType, Action: BulkSet, Value: " core", Condition: BulkEquals, ConditionValue: "none "},
			BulkRule{Field: BulkDebitType, Action: BulkSet, Value: "CORE", Condition: BulkEquals, ConditionValue: "NONE"}},
		{"debit clear", BulkRule{Field: BulkDebitType, Action: BulkClear, Condition: BulkAll},
			BulkRule{Field: BulkDebitType, Action: BulkClear, Condition: BulkAll}},
		{"reference copy with text", BulkRule{Field: BulkMandateReference, Action: BulkCopy, Source: BulkSourceParticipantNumber, Prefix: "Lastschrift-", Suffix: "/A", Condition: BulkEmpty},
			BulkRule{Field: BulkMandateReference, Action: BulkCopy, Source: BulkSourceParticipantNumber, Prefix: "Lastschrift-", Suffix: "/A", Condition: BulkEmpty}},
		{"reference clear equals text", BulkRule{Field: BulkMandateReference, Action: BulkClear, Condition: BulkEquals, ConditionValue: "Alt-1"},
			BulkRule{Field: BulkMandateReference, Action: BulkClear, Condition: BulkEquals, ConditionValue: "Alt-1"}},
		{"date set equals", BulkRule{Field: BulkMandateDate, Action: BulkSet, Value: "2026-01-01", Condition: BulkEquals, ConditionValue: "2025-12-31"},
			BulkRule{Field: BulkMandateDate, Action: BulkSet, Value: "2026-01-01", Condition: BulkEquals, ConditionValue: "2025-12-31"}},
		{"future date passes the matrix (F3 per hit)", BulkRule{Field: BulkMandateDate, Action: BulkSet, Value: "2999-01-01", Condition: BulkAll},
			BulkRule{Field: BulkMandateDate, Action: BulkSet, Value: "2999-01-01", Condition: BulkAll}},
		{"member tariff", BulkRule{Field: BulkParticipantTariff, Action: BulkSet, Value: strings.ToUpper(bulkTariffId), Condition: BulkEmpty},
			BulkRule{Field: BulkParticipantTariff, Action: BulkSet, Value: bulkTariffId, Condition: BulkEmpty}},
		{"meter tariff", BulkRule{Field: BulkMeterTariff, Action: BulkClear, Condition: BulkEquals, ConditionValue: bulkTariffId, Direction: "generation"},
			BulkRule{Field: BulkMeterTariff, Action: BulkClear, Condition: BulkEquals, ConditionValue: bulkTariffId, Direction: "GENERATION"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := c.rule
			require.NoError(t, r.Validate())
			assert.Equal(t, c.want, r)
		})
	}
}

// Everything outside the matrix is refused (400, code 6100), also fields outside the list.
func TestBulkRuleValidate_refused(t *testing.T) {
	ok := func(f BulkField, a BulkAction) BulkRule { return BulkRule{Field: f, Action: a, Condition: BulkAll} }
	with := func(r BulkRule, mod func(*BulkRule)) BulkRule { mod(&r); return r }
	cases := map[string]BulkRule{
		"field partFact":             ok("partFact", BulkSet),
		"field allocationFactor":     ok("allocationFactor", BulkSet),
		"field iban":                 ok("iban", BulkSet),
		"field participantSince":     ok("participantSince", BulkSet),
		"field tenant":               ok("tenant", BulkSet),
		"field empty":                ok("", BulkSet),
		"reference set":              with(ok(BulkMandateReference, BulkSet), func(r *BulkRule) { r.Value = "X" }),
		"debit copy":                 ok(BulkDebitType, BulkCopy),
		"unknown action":             ok(BulkDebitType, "DELETE"),
		"debit unknown value":        with(ok(BulkDebitType, BulkSet), func(r *BulkRule) { r.Value = "SEPA" }),
		"debit set without value":    ok(BulkDebitType, BulkSet),
		"value on clear":             with(ok(BulkDebitType, BulkClear), func(r *BulkRule) { r.Value = "CORE" }),
		"copy without source":        ok(BulkMandateReference, BulkCopy),
		"copy other source":          with(ok(BulkMandateReference, BulkCopy), func(r *BulkRule) { r.Source = "IBAN" }),
		"prefix on clear":            with(ok(BulkMandateReference, BulkClear), func(r *BulkRule) { r.Prefix = "X-" }),
		"suffix on debit":            with(ok(BulkDebitType, BulkClear), func(r *BulkRule) { r.Suffix = "-X" }),
		"source on clear":            with(ok(BulkMandateReference, BulkClear), func(r *BulkRule) { r.Source = BulkSourceParticipantNumber }),
		"date not iso":               with(ok(BulkMandateDate, BulkSet), func(r *BulkRule) { r.Value = "01.01.2026" }),
		"tariff not uuid":            with(ok(BulkParticipantTariff, BulkSet), func(r *BulkRule) { r.Value = "Tarif A" }),
		"meter without direction":    ok(BulkMeterTariff, BulkClear),
		"meter unknown direction":    with(ok(BulkMeterTariff, BulkClear), func(r *BulkRule) { r.Direction = "UNKNOWN" }),
		"direction on member tariff": with(ok(BulkParticipantTariff, BulkClear), func(r *BulkRule) { r.Direction = "CONSUMPTION" }),
		"unknown condition":          with(ok(BulkDebitType, BulkClear), func(r *BulkRule) { r.Condition = "SOME" }),
		"no condition":               with(ok(BulkDebitType, BulkClear), func(r *BulkRule) { r.Condition = "" }),
		"conditionValue on ALL":      with(ok(BulkDebitType, BulkClear), func(r *BulkRule) { r.ConditionValue = "CORE" }),
		"equals without value":       with(ok(BulkDebitType, BulkClear), func(r *BulkRule) { r.Condition = BulkEquals }),
		"equals reference blank":     with(ok(BulkMandateReference, BulkClear), func(r *BulkRule) { r.Condition, r.ConditionValue = BulkEquals, "  " }),
		"equals debit unknown":       with(ok(BulkDebitType, BulkClear), func(r *BulkRule) { r.Condition, r.ConditionValue = BulkEquals, "SEPA" }),
		"equals date not iso":        with(ok(BulkMandateDate, BulkClear), func(r *BulkRule) { r.Condition, r.ConditionValue = BulkEquals, "gestern" }),
		"equals tariff not uuid":     with(ok(BulkMeterTariff, BulkClear), func(r *BulkRule) { r.Direction, r.Condition, r.ConditionValue = "CONSUMPTION", BulkEquals, "x" }),
	}
	for name, r := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, 6100, bulkCode(t, r.Validate()))
		})
	}
}

// The SEPA mandate id rule of web (MandateReference.util.ts), with the examples of its tests.
func TestValidMandateReference(t *testing.T) {
	for ref, want := range map[string]bool{
		"4711": true, "EEG-001/A": true, "Lastschrift-135": true, "a+?/-:().,'b": true, strings.Repeat("1", 35): true,
		"": false, "EEG 001": false, "M_12": false, "Mü-1": false, strings.Repeat("1", 36): false, " 4711": false,
	} {
		assert.Equal(t, want, ValidMandateReference(ref), ref)
	}
}

// Apply items: at least one, well-formed ids, metering point exactly for METER_TARIFF, no duplicates (6101).
func TestBulkApplyRequestValidateItems(t *testing.T) {
	const pid = "ea9942da-0000-4000-8000-000000000001"
	member := &BulkRule{Field: BulkDebitType}
	meter := &BulkRule{Field: BulkMeterTariff}
	item := func(p, m string) *BulkApplyItem { return &BulkApplyItem{ParticipantId: p, MeteringPointId: m} }

	ok := BulkApplyRequest{Rule: member, Items: []*BulkApplyItem{item(strings.ToUpper(pid), "")}}
	require.NoError(t, ok.ValidateItems())
	assert.Equal(t, pid, ok.Items[0].ParticipantId, "lower-cased")

	cases := map[string]struct {
		req  BulkApplyRequest
		code int
	}{
		"no items":               {BulkApplyRequest{Rule: member}, 6100},
		"null item":              {BulkApplyRequest{Rule: member, Items: []*BulkApplyItem{nil}}, 6100},
		"bad participant id":     {BulkApplyRequest{Rule: member, Items: []*BulkApplyItem{item("x", "")}}, 6100},
		"meter on member rule":   {BulkApplyRequest{Rule: member, Items: []*BulkApplyItem{item(pid, "AT1")}}, 6100},
		"no meter on meter rule": {BulkApplyRequest{Rule: meter, Items: []*BulkApplyItem{item(pid, "")}}, 6100},
		"duplicate member":       {BulkApplyRequest{Rule: member, Items: []*BulkApplyItem{item(pid, ""), item(strings.ToUpper(pid), "")}}, 6101},
		"duplicate meter":        {BulkApplyRequest{Rule: meter, Items: []*BulkApplyItem{item(pid, "AT1"), item(pid, "AT1")}}, 6101},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, c.code, bulkCode(t, c.req.ValidateItems()))
		})
	}
	two := BulkApplyRequest{Rule: meter, Items: []*BulkApplyItem{item(pid, "AT1"), item(pid, "AT2")}}
	assert.NoError(t, two.ValidateItems(), "two meters of one member")
}

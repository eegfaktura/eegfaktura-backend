package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
)

func fileRule(field BulkField, pairs ...string) BulkRule {
	r := BulkRule{Field: field, Action: BulkFromFile, Condition: BulkAll}
	for i := 0; i+1 < len(pairs); i += 2 {
		r.Values = append(r.Values, &BulkFileValue{ParticipantNumber: pairs[i], Value: pairs[i+1]})
	}
	return r
}

func number(v string) func(*BulkMember) {
	return func(m *BulkMember) { m.Number = null.StringFrom(v) }
}

func TestBulkRuleValidate_fromFile(t *testing.T) {
	ok := fileRule(BulkMandateReference, " 7 ", " STERN-MIT0007 ")
	require.NoError(t, ok.Validate())
	assert.Equal(t, "7", ok.Values[0].ParticipantNumber, "trimmed")
	assert.Equal(t, "STERN-MIT0007", ok.Values[0].Value, "trimmed")
	date := fileRule(BulkMandateDate, "7", "2024-01-01")
	assert.NoError(t, date.Validate())

	tooMany := fileRule(BulkMandateReference)
	for i := 0; i <= BulkMaxFileValues; i++ {
		tooMany.Values = append(tooMany.Values, &BulkFileValue{ParticipantNumber: fmt.Sprint(i), Value: "R"})
	}
	long := make([]byte, 101)
	for i := range long {
		long[i] = 'x'
	}
	for name, r := range map[string]BulkRule{
		"debit type":           fileRule(BulkDebitType, "7", "CORE"),
		"tariff":               fileRule(BulkParticipantTariff, "7", "x"),
		"without values":       {Field: BulkMandateReference, Action: BulkFromFile, Condition: BulkAll},
		"values with COPY":     {Field: BulkMandateReference, Action: BulkCopy, Source: BulkSourceParticipantNumber, Condition: BulkAll, Values: []*BulkFileValue{{ParticipantNumber: "1", Value: "R"}}},
		"empty value":          fileRule(BulkMandateReference, "7", " "),
		"empty number":         fileRule(BulkMandateReference, "", "R"),
		"value too long":       fileRule(BulkMandateReference, "7", string(long)),
		"more than the max":    tooMany,
		"nil entry":            {Field: BulkMandateReference, Action: BulkFromFile, Condition: BulkAll, Values: []*BulkFileValue{nil}},
		"value with FROM_FILE": {Field: BulkMandateDate, Action: BulkFromFile, Value: "2024-01-01", Condition: BulkAll, Values: []*BulkFileValue{{ParticipantNumber: "1", Value: "2024-01-01"}}},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Error(t, r.Validate())
		})
	}
}

// Matching: trimmed and case-insensitive, otherwise exact; members without a row stay unchanged.
func TestPlanBulk_fromFile_matching(t *testing.T) {
	s := bulkSnap(
		bulkMember("7", ref(nil)),
		bulkMember("187a", ref(nil)),
		bulkMember("8", ref(nil)),
		bulkMember("x", number(" "), ref(nil)),
	)
	p := bulkPlan(t, fileRule(BulkMandateReference,
		"7", "STERN-MIT0007", " 187A ", "STERN-MIT0187a", "0008", "STERN-MIT0008", "9", "STERN-MIT0009"), s)

	r7 := bulkRowOf(t, p, "p-7")
	assert.Equal(t, BulkClassChanged, r7.Class)
	assert.Equal(t, "STERN-MIT0007", *r7.NewValue)
	assert.Equal(t, BulkClassChanged, bulkRowOf(t, p, "p-187a").Class, "case and blanks do not count")
	r8 := bulkRowOf(t, p, "p-8")
	assert.Equal(t, BulkClassUnchangedNotInFile, r8.Class, "0008 is not 8")
	assert.Nil(t, r8.NewValue, "new value = old value")
	assert.Equal(t, BulkClassUnchangedNotInFile, bulkRowOf(t, p, "p-x").Class, "a member without number is never matched")
	assert.Equal(t, []string{"0008", "9"}, p.NotFound, "as written in the file, in file order")
	// H6 on both: the effective reference of a CORE member changes (member number → file value)
	assert.Equal(t, BulkCounts{Changed: 2, WithHint: 2, UnchangedNotInFile: 2}, p.Counts)
	assert.Equal(t, []string{"H6"}, r7.Hints)
}

func TestPlanBulk_fromFile_duplicates(t *testing.T) {
	s := bulkSnap(
		bulkMember("1", ref(nil)),
		bulkMember("2", ref(nil)),
		bulkMember("3a", number("3"), ref(nil)),
		bulkMember("3b", number(" 3 "), ref(nil)),
	)
	p := bulkPlan(t, fileRule(BulkMandateReference,
		"1", "R-1", "1", "r-1", // the same value twice (case does not count for a reference): one row counts
		"2", "R-2", "2", "R-22", // different values: F8
		"3", "R-3"), s) // number of two members: F7 for both

	assert.Equal(t, BulkClassChanged, bulkRowOf(t, p, "p-1").Class)
	r2 := bulkRowOf(t, p, "p-2")
	assert.Equal(t, BulkClassError, r2.Class)
	assert.Equal(t, "F8", r2.Error)
	assert.Nil(t, r2.NewValue, "an error keeps the old value")
	for _, id := range []string{"p-3a", "p-3b"} {
		r := bulkRowOf(t, p, id)
		assert.Equal(t, BulkClassError, r.Class, id)
		assert.Equal(t, "F7", r.Error, id)
	}
	assert.Empty(t, p.NotFound)
	assert.Equal(t, 3, p.Counts.Errors)
}

func TestPlanBulk_fromFile_referenceChecks(t *testing.T) {
	s := bulkSnap(
		bulkMember("1", ref(nil)),
		bulkMember("2", ref(nil)),
		bulkMember("3", ref(nil)),
		bulkMember("4", debit("NONE"), ref(nil)),
	)
	s.Foreign = []*BulkForeignRef{foreignRef("taken-elsewhere", "CORE")}
	p := bulkPlan(t, fileRule(BulkMandateReference,
		"1", "STERN-EEG eGen MIT0001", // blanks: F1
		"2", "SAME", "3", "same", // two hits with the same new value: F2 for both
		"4", "TAKEN-ELSEWHERE"), s) // NONE member: not counted for uniqueness

	r1 := bulkRowOf(t, p, "p-1")
	assert.Equal(t, "F1", r1.Error)
	assert.Equal(t, "STERN-EEG eGen MIT0001", *r1.NewValue, "the invalid value is shown")
	assert.Equal(t, "F2", bulkRowOf(t, p, "p-2").Error)
	assert.Equal(t, "F2", bulkRowOf(t, p, "p-3").Error)
	assert.Equal(t, BulkClassChanged, bulkRowOf(t, p, "p-4").Class)
}

func TestPlanBulk_fromFile_conditionAndTarget(t *testing.T) {
	s := bulkSnap(
		bulkMember("1", ref(nil)),
		bulkMember("2"), // reference "M-2"
		bulkMember("3"), // reference "M-3"
	)
	rule := fileRule(BulkMandateReference, "1", "R-1", "2", "R-2", "3", "m-3")
	rule.Condition = BulkEmpty
	p := bulkPlan(t, rule, s)
	assert.Equal(t, BulkClassChanged, bulkRowOf(t, p, "p-1").Class)
	assert.Equal(t, BulkClassUnchangedCondition, bulkRowOf(t, p, "p-2").Class, "only where empty")
	assert.Equal(t, BulkClassUnchangedTarget, bulkRowOf(t, p, "p-3").Class, "has the value already (case does not count)")
}

func TestPlanBulk_fromFile_mandateDate(t *testing.T) {
	s := bulkSnap(bulkMember("1"), bulkMember("2"), bulkMember("3"), bulkMember("4"))
	p := bulkPlan(t, fileRule(BulkMandateDate,
		"1", "2024-01-01", "2", "31.2.2024", "3", "2024-02-31", "4", "2026-11-25"), s)

	r1 := bulkRowOf(t, p, "p-1")
	assert.Equal(t, BulkClassChanged, r1.Class)
	assert.Equal(t, "01.01.2024", r1.NewText)
	for _, id := range []string{"p-2", "p-3", "p-4"} {
		r := bulkRowOf(t, p, id)
		assert.Equal(t, BulkClassError, r.Class, id)
		assert.Equal(t, "F3", r.Error, id)
	}
	assert.Equal(t, "31.2.2024", *bulkRowOf(t, p, "p-2").NewValue, "an unreadable cell is shown as sent")
}

// Apply plans again and writes only the requested hits whose values still match (as for the other actions).
func TestPlanBulk_fromFile_keepFilter(t *testing.T) {
	rule := fileRule(BulkMandateReference, "1", "R-1", "2", "R-2")
	require.NoError(t, rule.Validate())
	s := bulkSnap(bulkMember("1", ref(nil)), bulkMember("2", ref(nil)))
	p := PlanBulkChange(&rule, s, bulkToday, func(r *BulkRow) bool { return r.ParticipantId == "p-1" })
	assert.Equal(t, BulkClassChanged, bulkRowOf(t, p, "p-1").Class)
	assert.Equal(t, BulkClassSkipped, bulkRowOf(t, p, "p-2").Class)
	exp := "r-1"
	assert.True(t, rule.SameValue(&exp, bulkRowOf(t, p, "p-1").NewValue), "expected new value compared like the rule")
}

// The other actions keep their answer: no notFound, no counter for "not in the file".
func TestPlanBulk_otherActionsWithoutFileFields(t *testing.T) {
	p := bulkPlan(t, debitToCore, bulkSnap(bulkMember("1", debit("NONE"))))
	assert.Nil(t, p.NotFound)
	assert.Zero(t, p.Counts.UnchangedNotInFile)
}

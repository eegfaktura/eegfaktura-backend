package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/guregu/null.v4"
)

const bulkToday = "2026-11-24"

// bulkMember is a member that triggers no hint: CORE, IBAN, valid unique reference "M-<nr>", mandate date, business.
func bulkMember(nr string, mods ...func(*BulkMember)) *BulkMember {
	m := &BulkMember{Id: "p-" + nr, Number: null.StringFrom(nr), Since: null.StringFrom("2025-01-01"), Firstname: "Vor",
		Lastname: "Name " + nr, BusinessRole: "EEG_BUSINESS", BankId: null.StringFrom("b-" + nr), HasIban: true,
		MandateRef: null.StringFrom("M-" + nr), MandateDate: null.StringFrom("2025-01-01"), DebitType: null.StringFrom("CORE")}
	for _, mod := range mods {
		mod(m)
	}
	return m
}

func debit(v string) func(*BulkMember) {
	return func(m *BulkMember) { m.DebitType = null.StringFrom(v) }
}
func ref(v *string) func(*BulkMember) {
	return func(m *BulkMember) { m.MandateRef = null.StringFromPtr(v) }
}

func bulkSnap(members ...*BulkMember) *BulkSnapshot {
	return &BulkSnapshot{Eeg: &BulkEeg{Tenant: "TE000001", Name: "EEG", RcNumber: "RC000001", CreditorNorm: "AT12ZZZ0001",
		HasIban: true, HasBic: true}, Members: members, Tariffs: map[string]*BulkTariff{}, ForeignRefs: map[string]bool{}}
}

func bulkPlan(t *testing.T, rule BulkRule, s *BulkSnapshot) *BulkPlan {
	t.Helper()
	require.NoError(t, rule.Validate())
	return PlanBulkChange(&rule, s, bulkToday, nil)
}

func bulkRowOf(t *testing.T, p *BulkPlan, participantId string) *BulkRow {
	t.Helper()
	for _, r := range p.Rows {
		if r.ParticipantId == participantId {
			return r
		}
	}
	t.Fatalf("no row of %s", participantId)
	return nil
}

var (
	debitToCore = BulkRule{Field: BulkDebitType, Action: BulkSet, Value: "CORE", Condition: BulkAll}
	copyRef     = BulkRule{Field: BulkMandateReference, Action: BulkCopy, Source: BulkSourceParticipantNumber, Condition: BulkAll}
)

// "Einzugsart = CORE, nur wo gleich NONE": NONE becomes CORE; empty and B2B are "condition not met", CORE "target".
func TestPlanBulk_debitWhereNone(t *testing.T) {
	p := bulkPlan(t, BulkRule{Field: BulkDebitType, Action: BulkSet, Value: "CORE", Condition: BulkEquals, ConditionValue: "NONE"},
		bulkSnap(bulkMember("1", debit("NONE")), bulkMember("2", debit("")), bulkMember("3", debit("B2B")), bulkMember("4")))
	assert.Equal(t, BulkCounts{Changed: 1, UnchangedCondition: 2, UnchangedTarget: 1}, p.Counts)
	r := bulkRowOf(t, p, "p-1")
	assert.Equal(t, BulkClassChanged, r.Class)
	assert.Equal(t, "Kein SEPA", r.OldText)
	assert.Equal(t, "CORE", r.NewText)
	assert.Equal(t, []string{}, r.Hints)
	assert.Equal(t, []string{"L3"}, p.RunHints)
}

// "Einzugsart = CORE, nur wo leer": NULL, "" and blanks become CORE, NONE stays.
func TestPlanBulk_debitWhereEmpty(t *testing.T) {
	null1 := bulkMember("1", func(m *BulkMember) { m.DebitType = null.String{} })
	p := bulkPlan(t, BulkRule{Field: BulkDebitType, Action: BulkSet, Value: "CORE", Condition: BulkEmpty},
		bulkSnap(null1, bulkMember("2", debit("")), bulkMember("3", debit("  ")), bulkMember("4", debit("NONE"))))
	assert.Equal(t, BulkCounts{Changed: 3, UnchangedCondition: 1}, p.Counts)
	assert.Equal(t, "nicht gewählt", bulkRowOf(t, p, "p-1").OldText)
}

// Copy with text before and after: "Lastschrift-" + 135 → "Lastschrift-135".
func TestPlanBulk_copyWithPrefixSuffix(t *testing.T) {
	rule := copyRef
	rule.Prefix, rule.Suffix = "Lastschrift-", "/A"
	p := bulkPlan(t, rule, bulkSnap(bulkMember("135")))
	r := bulkRowOf(t, p, "p-135")
	assert.Equal(t, BulkClassChanged, r.Class)
	assert.Equal(t, "Lastschrift-135/A", *r.NewValue)
	assert.Equal(t, "M-135", *r.OldValue)
}

func TestPlanBulk_F1_invalidReference(t *testing.T) {
	long := copyRef
	long.Prefix = strings.Repeat("L", 33) // 33 + "135" = 36 characters
	p := bulkPlan(t, long, bulkSnap(bulkMember("135")))
	assert.Equal(t, "F1", bulkRowOf(t, p, "p-135").Error)
	assert.Equal(t, 1, p.Counts.Errors)

	exact := copyRef
	exact.Prefix = strings.Repeat("L", 32) // 35 characters: valid
	assert.Equal(t, BulkClassChanged, bulkRowOf(t, bulkPlan(t, exact, bulkSnap(bulkMember("135"))), "p-135").Class)

	p = bulkPlan(t, copyRef, bulkSnap(bulkMember("EEG 001"), bulkMember("Mü-1")))
	assert.Equal(t, "F1", bulkRowOf(t, p, "p-EEG 001").Error)
	assert.Equal(t, "F1", bulkRowOf(t, p, "p-Mü-1").Error)

	clear := BulkRule{Field: BulkMandateReference, Action: BulkClear, Condition: BulkAll}
	p = bulkPlan(t, clear, bulkSnap(bulkMember("M_12")))
	assert.Equal(t, "F1", bulkRowOf(t, p, "p-M_12").Error, "clearing makes the invalid member number effective")
}

func TestPlanBulk_F2_duplicateReference(t *testing.T) {
	t.Run("stored reference of another member, case-insensitive", func(t *testing.T) {
		other := bulkMember("9", ref(str("x-1")))
		rule := copyRef
		rule.Prefix = "X-"
		rule.Condition, rule.ConditionValue = BulkEquals, "M-1"
		p := bulkPlan(t, rule, bulkSnap(bulkMember("1"), other))
		assert.Equal(t, "F2", bulkRowOf(t, p, "p-1").Error)
		assert.Equal(t, BulkClassUnchangedCondition, bulkRowOf(t, p, "p-9").Class)
	})
	t.Run("reference of another community with the same creditor id", func(t *testing.T) {
		s := bulkSnap(bulkMember("1"))
		s.ForeignRefs["1"] = true
		assert.Equal(t, "F2", bulkRowOf(t, bulkPlan(t, copyRef, s), "p-1").Error)
	})
	t.Run("two hits of the run with the same new value are both errors", func(t *testing.T) {
		p := bulkPlan(t, copyRef, bulkSnap(bulkMember("7"), bulkMember(" 7 ", func(m *BulkMember) { m.Id = "p-7b" })))
		assert.Equal(t, "F2", bulkRowOf(t, p, "p-7").Error)
		assert.Equal(t, "F2", bulkRowOf(t, p, "p-7b").Error)
		assert.Equal(t, 2, p.Counts.Errors)
	})
	t.Run("members without direct debit do not count", func(t *testing.T) {
		s := bulkSnap(bulkMember("1", debit("NONE")), bulkMember("2", debit("NONE"), ref(str("1"))))
		s.ForeignRefs["1"] = true
		p := bulkPlan(t, copyRef, s)
		assert.Equal(t, BulkClassChanged, bulkRowOf(t, p, "p-1").Class)
	})
	t.Run("a hit that keeps its old value collides again", func(t *testing.T) {
		// A: M-1 → 1 is taken abroad (F2) and keeps M-1; C: OLD-C → M-1 then collides with A's kept value
		a := bulkMember("1", ref(str("M-1")))
		c := bulkMember("M-1", func(m *BulkMember) { m.Id = "p-c"; m.MandateRef = null.StringFrom("OLD-C") })
		s := bulkSnap(a, c)
		s.ForeignRefs["1"] = true
		p := bulkPlan(t, copyRef, s)
		assert.Equal(t, "F2", bulkRowOf(t, p, "p-1").Error, "foreign")
		assert.Equal(t, "F2", bulkRowOf(t, p, "p-c").Error, "collides with A's kept M-1")
	})
}

func TestPlanBulk_F3_futureDate(t *testing.T) {
	p := bulkPlan(t, BulkRule{Field: BulkMandateDate, Action: BulkSet, Value: "2026-11-25", Condition: BulkAll}, bulkSnap(bulkMember("1")))
	assert.Equal(t, "F3", bulkRowOf(t, p, "p-1").Error)
	p = bulkPlan(t, BulkRule{Field: BulkMandateDate, Action: BulkSet, Value: bulkToday, Condition: BulkAll}, bulkSnap(bulkMember("1")))
	r := bulkRowOf(t, p, "p-1")
	assert.Equal(t, BulkClassChanged, r.Class)
	assert.Equal(t, "24.11.2026", r.NewText)
}

func bulkTariff(typ string, mods ...func(*BulkTariff)) *BulkTariff {
	t := &BulkTariff{Id: bulkTariffId, Name: "Tarif " + typ, Type: typ, Active: true}
	for _, mod := range mods {
		mod(t)
	}
	return t
}

func TestPlanBulk_F4_tariffDoesNotFit(t *testing.T) {
	member := BulkRule{Field: BulkParticipantTariff, Action: BulkSet, Value: bulkTariffId, Condition: BulkAll}
	for name, tariff := range map[string]*BulkTariff{
		"VZP": bulkTariff("VZP"), "EZP": bulkTariff("EZP"), "AKONTO": bulkTariff("AKONTO"), "unknown or foreign": nil,
		"archived": bulkTariff("EEG", func(t *BulkTariff) { t.Active = false }),
	} {
		t.Run("member tariff "+name, func(t *testing.T) {
			s := bulkSnap(bulkMember("1"))
			if tariff != nil {
				s.Tariffs[bulkTariffId] = tariff
			}
			assert.Equal(t, "F4", bulkRowOf(t, bulkPlan(t, member, s), "p-1").Error)
		})
	}
	t.Run("member tariff EEG fits", func(t *testing.T) {
		s := bulkSnap(bulkMember("1"))
		s.Tariffs[bulkTariffId] = bulkTariff("EEG")
		r := bulkRowOf(t, bulkPlan(t, member, s), "p-1")
		assert.Equal(t, BulkClassChanged, r.Class)
		assert.Equal(t, "Tarif EEG", r.NewText)
		assert.Equal(t, "kein Tarif", r.OldText)
		assert.Equal(t, []string{}, r.Hints)
	})
	for _, c := range []struct{ dir, typ, want string }{
		{"CONSUMPTION", "EZP", "F4"}, {"GENERATION", "VZP", "F4"}, {"CONSUMPTION", "VZP", ""}, {"GENERATION", "EZP", ""},
	} {
		t.Run("meter tariff "+c.typ+" on "+c.dir, func(t *testing.T) {
			s := bulkSnap(bulkMember("1"))
			s.Tariffs[bulkTariffId] = bulkTariff(c.typ)
			s.Meters = []*BulkMeter{{ParticipantId: "p-1", MeteringPointId: "AT1", Direction: c.dir}}
			p := bulkPlan(t, BulkRule{Field: BulkMeterTariff, Action: BulkSet, Value: bulkTariffId, Condition: BulkAll, Direction: c.dir}, s)
			require.Len(t, p.Rows, 1)
			assert.Equal(t, c.want, p.Rows[0].Error)
			assert.Equal(t, "AT1", p.Rows[0].MeteringPointId)
		})
	}
}

func TestPlanBulk_F5_emptySource(t *testing.T) {
	p := bulkPlan(t, copyRef, bulkSnap(bulkMember("1", func(m *BulkMember) { m.Number = null.StringFrom("  ") })))
	r := bulkRowOf(t, p, "p-1")
	assert.Equal(t, "F5", r.Error)
	assert.Equal(t, "M-1", *r.NewValue, "the target stays")
}

func TestPlanBulk_F6_noBankAccount(t *testing.T) {
	noBank := func(m *BulkMember) {
		m.BankId, m.HasIban, m.MandateRef, m.MandateDate, m.DebitType = null.String{}, false, null.String{}, null.String{}, null.String{}
	}
	p := bulkPlan(t, debitToCore, bulkSnap(bulkMember("1", noBank)))
	assert.Equal(t, "F6", bulkRowOf(t, p, "p-1").Error)
	// clearing what is not there changes nothing: no error
	p = bulkPlan(t, BulkRule{Field: BulkMandateDate, Action: BulkClear, Condition: BulkAll}, bulkSnap(bulkMember("1", noBank)))
	assert.Equal(t, BulkClassUnchangedTarget, bulkRowOf(t, p, "p-1").Class)
}

// ” is not NULL: clearing ” stores NULL (the member number applies), NULL is the target already.
func TestPlanBulk_emptyStringVersusNull(t *testing.T) {
	clear := BulkRule{Field: BulkMandateReference, Action: BulkClear, Condition: BulkAll}
	p := bulkPlan(t, clear, bulkSnap(bulkMember("1", ref(str(""))), bulkMember("2", ref(nil))))
	r1, r2 := bulkRowOf(t, p, "p-1"), bulkRowOf(t, p, "p-2")
	assert.Equal(t, BulkClassChanged, r1.Class)
	assert.Nil(t, r1.NewValue)
	assert.Equal(t, "leer", r1.OldText)
	assert.Equal(t, "leer (Mitgliedsnummer gilt)", r1.NewText)
	assert.Equal(t, BulkClassUnchangedTarget, r2.Class)

	empty := copyRef
	empty.Condition = BulkEmpty
	p = bulkPlan(t, empty, bulkSnap(bulkMember("1", ref(str(" "))), bulkMember("2", ref(nil)), bulkMember("3")))
	assert.Equal(t, BulkCounts{Changed: 2, WithHint: 1, UnchangedCondition: 1}, p.Counts, "\" \" → 1 changes the effective reference (H6)")
}

// Only current meters of the rule's direction are hits.
func TestPlanBulk_meterDirection(t *testing.T) {
	s := bulkSnap(bulkMember("1"))
	s.Meters = []*BulkMeter{{ParticipantId: "p-1", MeteringPointId: "AT1", Direction: "CONSUMPTION"},
		{ParticipantId: "p-1", MeteringPointId: "AT2", Direction: "GENERATION"}}
	p := bulkPlan(t, BulkRule{Field: BulkMeterTariff, Action: BulkClear, Condition: BulkAll, Direction: "GENERATION"}, s)
	require.Len(t, p.Rows, 1)
	assert.Equal(t, "AT2", p.Rows[0].MeteringPointId)
	assert.Equal(t, "Name 1", strings.TrimPrefix(p.Rows[0].Name, "Vor "))
}

// Apply plans only what it writes: a hit left out keeps its value, so a requested hit that collides with it is F2.
func TestPlanBulk_keepFilter(t *testing.T) {
	rule := copyRef
	rule.Prefix = "X-"
	a := bulkMember("5")
	b := bulkMember("6", ref(str("X-5")))
	preview := bulkPlan(t, rule, bulkSnap(a, b))
	assert.Equal(t, 2, preview.Counts.Changed, "B moves to X-6, A may take X-5")

	applied := PlanBulkChange(&rule, bulkSnap(a, b), bulkToday, func(r *BulkRow) bool { return r.ParticipantId == "p-5" })
	assert.Equal(t, "F2", bulkRowOf(t, applied, "p-5").Error)
	assert.Equal(t, BulkClassSkipped, bulkRowOf(t, applied, "p-6").Class)
	assert.Equal(t, 0, applied.Counts.Changed)
}

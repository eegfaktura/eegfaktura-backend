package model

import (
	"strings"
	"time"

	"gopkg.in/guregu/null.v4"
)

// Values of the bulk change: stored value, condition, new value and row-level errors F1 and F3–F6, plain texts.
// Comparison rule (Fachliche Logik 4): trimmed, case-insensitive; NULL, "" and blanks are empty. The effective
// value follows base.billing_masterdata: the fallback (member number, member since) only for NULL.

type bulkEvaluator struct {
	rule  *BulkRule
	s     *BulkSnapshot
	today string
	// tariffError: F4 for SET on a tariff field when the tariff does not fit (the same for every hit).
	tariffError string
	// refCounts: DEBIT_TYPE only, the normalised effective references of direct debit members after the run.
	refCounts map[string]int
	// foreign: normalised effective references of the direct debit members of the other communities (Foreign).
	foreign map[string]bool
}

func newBulkEvaluator(rule *BulkRule, s *BulkSnapshot, today string) *bulkEvaluator {
	e := &bulkEvaluator{rule: rule, s: s, today: today, foreign: map[string]bool{}}
	for _, f := range s.Foreign {
		if k := bulkNormRef(f.Ref.String); k != "" && isDirectDebit(f.DebitType) {
			e.foreign[k] = true
		}
	}
	if rule.Action == BulkSet && (rule.Field == BulkParticipantTariff || rule.Field == BulkMeterTariff) {
		want := string(EEG)
		if rule.Field == BulkMeterTariff {
			want = string(VZP)
			if rule.Direction == string(GENERATOR) {
				want = string(EZP)
			}
		}
		if t := s.Tariffs[rule.Value]; t == nil || !t.Active || t.Type != want {
			e.tariffError = "F4"
		}
	}
	return e
}

func (e *bulkEvaluator) bankField() bool {
	f := e.rule.Field
	return f == BulkDebitType || f == BulkMandateReference || f == BulkMandateDate
}

func bulkIsEmpty(v null.String) bool { return !v.Valid || strings.TrimSpace(v.String) == "" }

func bulkNormRef(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// norm is the comparison form of a non-empty value of the rule's field.
func (e *bulkEvaluator) norm(s string) string {
	switch e.rule.Field {
	case BulkParticipantTariff, BulkMeterTariff:
		return strings.ToLower(strings.TrimSpace(s))
	case BulkMandateDate:
		return strings.TrimSpace(s)
	}
	return bulkNormRef(s)
}

func (e *bulkEvaluator) stored(m *BulkMember, mt *BulkMeter) null.String {
	switch e.rule.Field {
	case BulkDebitType:
		return m.DebitType
	case BulkMandateReference:
		return m.MandateRef
	case BulkMandateDate:
		return m.MandateDate
	case BulkParticipantTariff:
		return m.TariffId
	}
	return mt.TariffId
}

func (e *bulkEvaluator) conditionMet(old null.String) bool {
	switch e.rule.Condition {
	case BulkEmpty:
		return bulkIsEmpty(old)
	case BulkEquals:
		return !bulkIsEmpty(old) && e.norm(old.String) == e.norm(e.rule.ConditionValue)
	}
	return true
}

// newValue is the target value and a row-level error (F1, F3, F4, F5).
func (e *bulkEvaluator) newValue(m *BulkMember) (null.String, string) {
	r := e.rule
	switch {
	case r.Action == BulkCopy:
		if bulkIsEmpty(m.Number) {
			return null.String{}, "F5"
		}
		// the member number as stored, like COALESCE(mandate_reference, "participantNumber") in billing_masterdata
		v := r.Prefix + m.Number.String + r.Suffix
		if !ValidMandateReference(v) {
			return null.StringFrom(v), "F1"
		}
		return null.StringFrom(v), ""
	case r.Action == BulkClear:
		// the member number becomes the effective reference
		if r.Field == BulkMandateReference && !bulkIsEmpty(m.Number) && !ValidMandateReference(m.Number.String) {
			return null.String{}, "F1"
		}
		return null.String{}, ""
	case r.Field == BulkMandateDate && r.Value > e.today:
		return null.StringFrom(r.Value), "F3"
	}
	return null.StringFrom(r.Value), e.tariffError
}

// same reports whether the stored value already is the target. A mandate reference "" is not NULL: only NULL
// lets the member number apply, so clearing a "" changes it.
func (e *bulkEvaluator) same(old, nv null.String) bool {
	if e.rule.Field == BulkMandateReference && old.Valid != nv.Valid {
		return false
	}
	if bulkIsEmpty(old) || bulkIsEmpty(nv) {
		return bulkIsEmpty(old) && bulkIsEmpty(nv)
	}
	return e.norm(old.String) == e.norm(nv.String)
}

// SameValue compares an expected value of an apply request with the planned one, normalised like the rule
// (trimmed, case-insensitive for direct debit type and reference, tariff id lower-cased, empty = null; E5).
func (r *BulkRule) SameValue(a, b *string) bool {
	e := &bulkEvaluator{rule: r}
	norm := func(p *string) string {
		if p == nil || strings.TrimSpace(*p) == "" {
			return ""
		}
		return e.norm(*p)
	}
	return norm(a) == norm(b)
}

// row classifies one hit (Fachliche Logik 5, 7).
func (e *bulkEvaluator) row(m *BulkMember, mt *BulkMeter) *BulkRow {
	old := e.stored(m, mt)
	r := &BulkRow{ParticipantId: m.Id, ParticipantNumber: m.Number.String, Name: memberName(m), BankId: m.BankId.String,
		member: m, meter: mt, OldValue: old.Ptr(), OldText: e.text(old)}
	if mt != nil {
		r.MeteringPointId = mt.MeteringPointId
	}
	setNew := func(v null.String) { r.NewValue, r.NewText = v.Ptr(), e.text(v) }
	nv, code := e.newValue(m)
	setNew(nv)
	switch {
	case code != "F5" && e.same(old, nv):
		// before the condition: "CORE where NONE" counts a CORE member as "has the target already"
		r.Class = BulkClassUnchangedTarget
	case !e.conditionMet(old):
		setNew(old)
		r.Class = BulkClassUnchangedCondition
	case code == "F5":
		setNew(old) // the target stays
		r.Class, r.Error = BulkClassError, code
	case e.bankField() && !m.BankId.Valid:
		r.Class, r.Error = BulkClassError, "F6" // no bank account row is created
	case code != "":
		r.Class, r.Error = BulkClassError, code
	default:
		r.Class = BulkClassChanged
	}
	return r
}

// text is the plain text of a value for the preview and the CSV.
func (e *bulkEvaluator) text(v null.String) string {
	switch e.rule.Field {
	case BulkDebitType:
		switch t := bulkNormRef(v.String); {
		case bulkIsEmpty(v):
			return "nicht gewählt"
		case t == "NONE":
			return "Kein SEPA"
		case t == "CORE" || t == "B2B":
			return t
		}
		return v.String
	case BulkMandateReference:
		if !v.Valid {
			return "leer (Mitgliedsnummer gilt)"
		}
		if bulkIsEmpty(v) {
			return "leer"
		}
		return v.String
	case BulkMandateDate:
		if bulkIsEmpty(v) {
			return "leer (Mitglied seit gilt)"
		}
		if d, err := time.Parse(time.DateOnly, v.String); err == nil {
			return d.Format("02.01.2006")
		}
		return v.String
	}
	if bulkIsEmpty(v) {
		return "kein Tarif"
	}
	if t := e.s.Tariffs[e.norm(v.String)]; t != nil {
		return t.Name
	}
	return v.String
}

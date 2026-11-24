package model

import (
	"math"
	"strings"

	"gopkg.in/guregu/null.v4"
)

// Uniqueness of the mandate reference (Fachliche Logik 6, F2/H4) and the hints H1–H10, L1–L3 (Fachliche Logik 8).

func isDirectDebit(v null.String) bool {
	t := bulkNormRef(v.String)
	return t == "CORE" || t == "B2B"
}

// effectiveRef is the reference billing takes: the stored one, the member number only for NULL.
func effectiveRef(ref, number null.String) string {
	if ref.Valid {
		return ref.String
	}
	return number.String
}

// post returns the value of the field after the run: the new value for a hit that changes, the stored otherwise.
func (e *bulkEvaluator) post(r *BulkRow, field BulkField, stored null.String) null.String {
	if e.rule.Field == field && r.Class == BulkClassChanged {
		return null.StringFromPtr(r.NewValue)
	}
	return stored
}

func (e *bulkEvaluator) postDebit(r *BulkRow) null.String {
	return e.post(r, BulkDebitType, r.member.DebitType)
}

func (e *bulkEvaluator) postRef(r *BulkRow) string {
	return effectiveRef(e.post(r, BulkMandateReference, r.member.MandateRef), r.member.Number)
}

// refCountsAfter counts the normalised effective references of the members with direct debit after the run.
func (e *bulkEvaluator) refCountsAfter(rows []*BulkRow) map[string]int {
	counts := map[string]int{}
	for _, r := range rows {
		if isDirectDebit(e.postDebit(r)) {
			if k := bulkNormRef(e.postRef(r)); k != "" {
				counts[k]++
			}
		}
	}
	return counts
}

func (e *bulkEvaluator) taken(k string, counts map[string]int) bool {
	return counts[k] > 1 || e.foreign[k]
}

// uniqueness marks a changing mandate reference that would be used twice under the creditor id as F2 — both hits
// when two hits of the run get the same value, whatever their direct debit type (Fachliche Logik 6: a run that sets
// the references before the direct debit type must not leave duplicates behind). A hit marked F2 keeps its old
// value, which may collide with another hit: repeated until nothing changes. For DEBIT_TYPE it only keeps the counts
// for H4.
func (e *bulkEvaluator) uniqueness(rows []*BulkRow) {
	switch e.rule.Field {
	case BulkDebitType:
		e.refCounts = e.refCountsAfter(rows)
	case BulkMandateReference:
		for marked := true; marked; {
			marked = false
			counts := e.refCountsAfter(rows)
			inRun := map[string]int{} // new values of the hits that change
			for _, r := range rows {
				if r.Class == BulkClassChanged {
					inRun[bulkNormRef(e.postRef(r))]++
				}
			}
			for _, r := range rows {
				if r.Class != BulkClassChanged {
					continue
				}
				k := bulkNormRef(e.postRef(r))
				if k != "" && (inRun[k] > 1 || isDirectDebit(r.member.DebitType) && e.taken(k, counts)) {
					r.Class, r.Error, marked = BulkClassError, "F2", true
				}
			}
		}
	}
}

// hints of a hit that changes.
func (e *bulkEvaluator) hints(r *BulkRow) []string {
	var h []string
	add := func(cond bool, code string) {
		if cond {
			h = append(h, code)
		}
	}
	m, f := r.member, e.rule.Field
	switch f {
	case BulkDebitType, BulkMandateReference, BulkMandateDate:
		debit := isDirectDebit(e.postDebit(r))
		ref := e.postRef(r)
		add(debit && !m.HasIban, "H1")
		add(debit && strings.TrimSpace(ref) == "", "H2")
		add(debit && bulkIsEmpty(e.post(r, BulkMandateDate, m.MandateDate)) && bulkIsEmpty(m.Since), "H3")
		if f == BulkDebitType {
			k := bulkNormRef(ref)
			add(debit && !isDirectDebit(m.DebitType) && k != "" &&
				(!ValidMandateReference(ref) || e.taken(k, e.refCounts)), "H4")
			add(bulkNormRef(e.postDebit(r).String) == "B2B" && strings.EqualFold(strings.TrimSpace(m.BusinessRole), "EEG_PRIVATE"), "H5")
		}
		add(f == BulkMandateReference && debit && bulkNormRef(effectiveRef(m.MandateRef, m.Number)) != bulkNormRef(ref), "H6")
		// a blank in the reference the run writes or makes effective: allowed, but not every bank takes it
		add(strings.Contains(ref, " ") && (f == BulkMandateReference || f == BulkDebitType && debit), "H10")
	case BulkParticipantTariff:
		if t := e.s.Tariffs[e.rule.Value]; e.rule.Action == BulkSet && t != nil {
			add(t.JoiningFee > 0, "H7")
			add(e.joiningFeeVatUnusual(t), "H8")
		}
	}
	add(e.rule.Action == BulkClear && (f == BulkParticipantTariff || f == BulkMeterTariff), "H9")
	return h
}

// joiningFeeVatUnusual: the joining fee's VAT rate is above 0 % and no active tariff of the community uses it (as
// web joiningFeeVatUnusual, src/util/JoiningFee.util.ts): an invoice can hold at most two rates above 0 %.
func (e *bulkEvaluator) joiningFeeVatUnusual(t *BulkTariff) bool {
	if t.JoiningFee <= 0 || !t.JoiningFeeUseVat || t.JoiningFeeVat <= 0 {
		return false
	}
	for _, o := range e.s.Tariffs {
		if !o.Active {
			continue
		}
		if o.UseVat && math.Abs(o.Vat-t.JoiningFeeVat) < 1e-9 {
			return false
		}
		if o.Type == string(EZP) && o.UseMeterFee && o.MeterVat.Valid && math.Abs(o.MeterVat.Float64-t.JoiningFeeVat) < 1e-9 {
			return false
		}
	}
	return true
}

// runHints L1–L3 of the run.
func (e *bulkEvaluator) runHints() []string {
	var h []string
	r, eeg := e.rule, e.s.Eeg
	if r.Field == BulkDebitType && r.Action == BulkSet && (r.Value == "CORE" || r.Value == "B2B") {
		if !eeg.HasIban || !eeg.HasBic {
			h = append(h, "L1")
		}
		if eeg.CreditorNorm == "" {
			h = append(h, "L2")
		}
	}
	return append(h, "L3")
}

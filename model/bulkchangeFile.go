package model

import (
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/guregu/null.v4"
)

// Value from file (konzept-massenaenderungen.md, Teil 2, Fachliche Logik 16–21, Tech Design G): the rule carries one
// value per member number; admin reads the file in the browser and sends only these pairs. A member is matched by
// its number, trimmed and case-insensitive, otherwise exactly ("0007" is not "7").

// BulkMaxFileValues bounds the values of one FROM_FILE rule.
const BulkMaxFileValues = 5000

// bulkMaxFileText bounds a member number and a value of the file.
const bulkMaxFileText = 100

// bulkMinDate: an earlier mandate date is F3 (a text cell "0000-01-01" parses, but Postgres has no year 0).
const bulkMinDate = "1900-01-01"

// BulkFileValue is one row of the file: member number and the new value of the rule's field.
type BulkFileValue struct {
	ParticipantNumber string `json:"participantNumber"`
	Value             string `json:"value"`
}

func bulkNormNumber(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// validateValues checks the values of a FROM_FILE rule (Validate has made sure there are some) and trims them. An
// empty cell is not sent by admin: an empty number or value is a broken request (400), not "clear the field".
func (r *BulkRule) validateValues() error {
	if len(r.Values) > BulkMaxFileValues {
		return errBulkRule("höchstens %d Werte je Lauf", BulkMaxFileValues)
	}
	for _, v := range r.Values {
		if v == nil {
			return errBulkRule("leerer Eintrag in values")
		}
		v.ParticipantNumber, v.Value = strings.TrimSpace(v.ParticipantNumber), strings.TrimSpace(v.Value)
		if v.ParticipantNumber == "" || v.Value == "" {
			return errBulkRule("Mitgliedsnummer und Wert dürfen nicht leer sein")
		}
		if utf8.RuneCountInString(v.ParticipantNumber) > bulkMaxFileText || utf8.RuneCountInString(v.Value) > bulkMaxFileText {
			return errBulkRule("Mitgliedsnummer oder Wert länger als %d Zeichen", bulkMaxFileText)
		}
	}
	return nil
}

// bulkFile is the file of a FROM_FILE rule, indexed by the normalised member number.
type bulkFile struct {
	first    []*BulkFileValue  // the first row of each number, in file order
	values   map[string]string // normalised number → value of its first row
	conflict map[string]bool   // the number has different values in the file (F8)
	members  map[string]int    // members of the community with the number (more than one: F7)
}

// newBulkFile indexes the values; same reports whether two values of one number are the same (normalised like the
// rule's field).
func newBulkFile(values []*BulkFileValue, members []*BulkMember, same func(a, b string) bool) *bulkFile {
	f := &bulkFile{values: map[string]string{}, conflict: map[string]bool{}, members: map[string]int{}}
	for _, v := range values {
		k := bulkNormNumber(v.ParticipantNumber)
		if old, seen := f.values[k]; !seen {
			f.values[k] = v.Value
			f.first = append(f.first, v)
		} else if !same(old, v.Value) {
			f.conflict[k] = true
		}
	}
	for _, m := range members {
		if !bulkIsEmpty(m.Number) {
			f.members[bulkNormNumber(m.Number.String)]++
		}
	}
	return f
}

// inFile reports whether the member has a value in the file (a member without number never has).
func (f *bulkFile) inFile(m *BulkMember) bool {
	if bulkIsEmpty(m.Number) {
		return false
	}
	_, ok := f.values[bulkNormNumber(m.Number.String)]
	return ok
}

// notFound lists the numbers of the file without a member in the community, as written in the file.
func (f *bulkFile) notFound() []string {
	var list []string
	for _, v := range f.first {
		if f.members[bulkNormNumber(v.ParticipantNumber)] == 0 {
			list = append(list, v.ParticipantNumber)
		}
	}
	return list
}

// fileValue is the new value of a member in the file and its row error: F7 (the number belongs to several members;
// the value is not applied), F8 (different values in the file; neither applied), F1 (reference invalid), F3 (date
// unreadable, in the future or before 1900). Like the other actions, the error counts only when the condition holds
// and the member does not have the value already (row).
func (e *bulkEvaluator) fileValue(m *BulkMember) (null.String, string) {
	k := bulkNormNumber(m.Number.String)
	v := e.file.values[k]
	code := ""
	switch {
	case e.file.members[k] > 1:
		return null.String{}, "F7"
	case e.file.conflict[k]:
		return null.String{}, "F8"
	case e.rule.Field == BulkMandateReference && !ValidMandateReference(v):
		code = "F1"
	case e.rule.Field == BulkMandateDate:
		if _, err := time.Parse(time.DateOnly, v); err != nil || v > e.today || v < bulkMinDate {
			code = "F3"
		}
	}
	return null.StringFrom(v), code
}

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

// bulkNotInFile is the internal state of a member without a value in the file (class UNCHANGED_NOT_IN_FILE).
const bulkNotInFile = "NOT_IN_FILE"

// BulkFileValue is one row of the file: member number and the new value of the rule's field.
type BulkFileValue struct {
	ParticipantNumber string `json:"participantNumber"`
	Value             string `json:"value"`
}

func bulkNormNumber(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// validateValues checks the values of a FROM_FILE rule and trims them. An empty cell is not sent by admin: an
// empty number or value is a broken request (400), not "clear the field".
func (r *BulkRule) validateValues() error {
	if len(r.Values) == 0 {
		return errBulkRule("values fehlt")
	}
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
	values   map[string]string
	conflict map[string]bool // the number has different values in the file (F8)
	members  map[string]int  // members of the community with the number (more than one: F7)
	order    []string        // normalised numbers in file order
	spelling map[string]string
}

// newBulkFile indexes the values; same reports whether two values of one number are the same (normalised like the
// rule's field).
func newBulkFile(values []*BulkFileValue, members []*BulkMember, same func(a, b string) bool) *bulkFile {
	f := &bulkFile{values: map[string]string{}, conflict: map[string]bool{}, members: map[string]int{},
		spelling: map[string]string{}}
	for _, v := range values {
		k := bulkNormNumber(v.ParticipantNumber)
		old, seen := f.values[k]
		switch {
		case !seen:
			f.values[k], f.spelling[k] = v.Value, v.ParticipantNumber
			f.order = append(f.order, k)
		case !same(old, v.Value):
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

// lookup is the value of the member and its state: bulkNotInFile, F7 (number of several members), F8 (different
// values in the file) or "" (the value applies).
func (f *bulkFile) lookup(m *BulkMember) (string, string) {
	if bulkIsEmpty(m.Number) {
		return "", bulkNotInFile
	}
	k := bulkNormNumber(m.Number.String)
	v, ok := f.values[k]
	switch {
	case !ok:
		return "", bulkNotInFile
	case f.members[k] > 1:
		return "", "F7"
	case f.conflict[k]:
		return "", "F8"
	}
	return v, ""
}

// notFound lists the numbers of the file without a member in the community, as written in the file.
func (f *bulkFile) notFound() []string {
	list := []string{}
	for _, k := range f.order {
		if f.members[k] == 0 {
			list = append(list, f.spelling[k])
		}
	}
	return list
}

// fileValue is the new value of a member whose file value applies, with F1 (reference) or F3 (date).
func (e *bulkEvaluator) fileValue(v string) (null.String, string) {
	if e.rule.Field == BulkMandateReference {
		if !ValidMandateReference(v) {
			return null.StringFrom(v), "F1"
		}
		return null.StringFrom(v), ""
	}
	if _, err := time.Parse(time.DateOnly, v); err != nil || v > e.today {
		return null.StringFrom(v), "F3"
	}
	return null.StringFrom(v), ""
}

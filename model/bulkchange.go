package model

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pborman/uuid"
)

// Bulk change of the support (konzept-massenaenderungen.md): one rule on one community, preview and apply.
// The rule is a closed set: every field maps to one fixed column in the DAO, nothing of the request becomes a
// column name. This file holds the rule, its matrix and the request types; bulkchangePlan*.go the planner.

type BulkField string

const (
	BulkDebitType         BulkField = "DEBIT_TYPE"
	BulkMandateReference  BulkField = "MANDATE_REFERENCE"
	BulkMandateDate       BulkField = "MANDATE_DATE"
	BulkParticipantTariff BulkField = "PARTICIPANT_TARIFF"
	BulkMeterTariff       BulkField = "METER_TARIFF"
)

type BulkAction string

const (
	BulkSet   BulkAction = "SET"
	BulkClear BulkAction = "CLEAR"
	BulkCopy  BulkAction = "COPY"
	// BulkFromFile: one value per member number from a file (Teil 2, bulkchangeFile.go).
	BulkFromFile BulkAction = "FROM_FILE"
)

type BulkCondition string

const (
	BulkAll    BulkCondition = "ALL"
	BulkEmpty  BulkCondition = "EMPTY"
	BulkEquals BulkCondition = "EQUALS"
)

// BulkSourceParticipantNumber is the only source of COPY.
const BulkSourceParticipantNumber = "PARTICIPANT_NUMBER"

// BulkRule is the rule of preview and apply. An empty string means "not given"; Validate refuses every value
// the matrix does not allow for the field and action, and normalises the given ones.
type BulkRule struct {
	Field          BulkField     `json:"field"`
	Action         BulkAction    `json:"action"`
	Value          string        `json:"value,omitempty"`
	Source         string        `json:"source,omitempty"`
	Prefix         string        `json:"prefix,omitempty"`
	Suffix         string        `json:"suffix,omitempty"`
	Condition      BulkCondition `json:"condition"`
	ConditionValue string        `json:"conditionValue,omitempty"`
	Direction      string        `json:"direction,omitempty"`
	// Values: FROM_FILE only, one value per member number.
	Values []*BulkFileValue `json:"values,omitempty"`
}

// BulkPreviewRequest is the body of POST /bulkchange/preview.
type BulkPreviewRequest struct {
	Rule *BulkRule `json:"rule"`
}

// BulkApplyItem is one changed hit of the preview, sent back to apply.
type BulkApplyItem struct {
	ParticipantId   string  `json:"participantId"`
	MeteringPointId string  `json:"meteringPointId,omitempty"`
	ExpectedOld     *string `json:"expectedOld"`
	ExpectedNew     *string `json:"expectedNew"`
}

// BulkApplyRequest is the body of POST /bulkchange/apply.
type BulkApplyRequest struct {
	Rule  *BulkRule        `json:"rule"`
	Items []*BulkApplyItem `json:"items"`
}

// BulkTariffOption is one entry of GET /bulkchange/tariffs: an active tariff of a type the rules can set.
type BulkTariffOption struct {
	Id         string  `json:"id" db:"id"`
	Name       string  `json:"name" db:"name"`
	Type       string  `json:"type" db:"type"`
	JoiningFee float64 `json:"joiningFee" db:"joining_fee"`
}

// BulkCommunity is one entry of GET /bulkchange/communities.
type BulkCommunity struct {
	Tenant   string `json:"tenant" db:"tenant"`
	Name     string `json:"name" db:"name"`
	RcNumber string `json:"rcNumber" db:"rcNumber"`
}

// sepaMandateIdRe is the SEPA mandate id (MndtId): 1–35 characters of the restricted SEPA set. A blank is part of
// that set and allowed inside, not at the start or end (decision 24.11.2026: a community's bank takes references
// with blanks; the web form accepts them too). The preview marks a reference with a blank (H10), since not every
// bank takes it. web suggests the member number only without blanks (src/util/MandateReference.util.ts).
var sepaMandateIdRe = regexp.MustCompile(`^[A-Za-z0-9+?/\-:().,']([A-Za-z0-9+?/\-:().,' ]{0,33}[A-Za-z0-9+?/\-:().,'])?$`)

// ValidMandateReference reports whether ref is a valid SEPA mandate id.
func ValidMandateReference(ref string) bool {
	return sepaMandateIdRe.MatchString(ref)
}

var bulkDebitTypes = map[string]bool{"CORE": true, "B2B": true, "NONE": true}

func errBulkRule(format string, args ...interface{}) error {
	return ErrBulkRule(fmt.Errorf("Regel ungültig: "+format, args...))
}

// Validate checks the rule against the matrix of the Tech Design (A) and normalises its values: direct debit type
// upper-cased, tariff ids lower-cased, the direction upper-cased. Every violation is ErrBulkRule (400).
func (r *BulkRule) Validate() error {
	setOnly := func(name, v string, allowed bool) error {
		if v != "" && !allowed {
			return errBulkRule("%s ist für %s/%s nicht erlaubt", name, r.Field, r.Action)
		}
		return nil
	}
	var actions []BulkAction
	switch r.Field {
	case BulkDebitType, BulkParticipantTariff, BulkMeterTariff:
		actions = []BulkAction{BulkSet, BulkClear}
	case BulkMandateDate:
		actions = []BulkAction{BulkSet, BulkClear, BulkFromFile}
	case BulkMandateReference:
		actions = []BulkAction{BulkCopy, BulkClear, BulkFromFile}
	default:
		return errBulkRule("Feld %q ist nicht änderbar", r.Field)
	}
	if !slices.Contains(actions, r.Action) {
		return errBulkRule("Aktion %q ist für %s nicht erlaubt", r.Action, r.Field)
	}
	for _, err := range []error{
		setOnly("value", r.Value, r.Action == BulkSet),
		setOnly("source", r.Source, r.Action == BulkCopy),
		setOnly("prefix", r.Prefix, r.Action == BulkCopy),
		setOnly("suffix", r.Suffix, r.Action == BulkCopy),
		setOnly("direction", r.Direction, r.Field == BulkMeterTariff),
		setOnly("conditionValue", r.ConditionValue, r.Condition == BulkEquals),
	} {
		if err != nil {
			return err
		}
	}
	if (len(r.Values) > 0) != (r.Action == BulkFromFile) {
		return errBulkRule("values nur und immer bei %s", BulkFromFile)
	}
	if r.Action == BulkFromFile {
		if err := r.validateValues(); err != nil {
			return err
		}
	}
	if r.Action == BulkCopy && r.Source != BulkSourceParticipantNumber {
		return errBulkRule("source muss %s sein", BulkSourceParticipantNumber)
	}
	if r.Field == BulkMeterTariff {
		r.Direction = strings.ToUpper(strings.TrimSpace(r.Direction))
		if r.Direction != string(CONSUMPTION) && r.Direction != string(GENERATOR) {
			return errBulkRule("direction muss %s oder %s sein", CONSUMPTION, GENERATOR)
		}
	}
	if r.Action == BulkSet {
		v, err := r.normaliseValue(r.Value)
		if err != nil {
			return err
		}
		r.Value = v
	}
	switch r.Condition {
	case BulkAll, BulkEmpty:
	case BulkEquals:
		if r.Field == BulkMandateReference {
			if strings.TrimSpace(r.ConditionValue) == "" {
				return errBulkRule("conditionValue fehlt")
			}
			return nil
		}
		v, err := r.normaliseValue(r.ConditionValue)
		if err != nil {
			return err
		}
		r.ConditionValue = v
	default:
		return errBulkRule("Bedingung %q ist unbekannt", r.Condition)
	}
	return nil
}

// normaliseValue checks a value or condition value of the field's type (not the mandate reference, which has
// no value to set).
func (r *BulkRule) normaliseValue(v string) (string, error) {
	switch r.Field {
	case BulkDebitType:
		t := strings.ToUpper(strings.TrimSpace(v))
		if !bulkDebitTypes[t] {
			return "", errBulkRule("Einzugsart muss CORE, B2B oder NONE sein")
		}
		return t, nil
	case BulkMandateDate:
		t := strings.TrimSpace(v)
		if _, err := time.Parse(time.DateOnly, t); err != nil {
			return "", errBulkRule("Datum muss JJJJ-MM-TT sein")
		}
		if t < bulkMinDate {
			return "", errBulkRule("Datum vor 1900")
		}
		return t, nil
	case BulkParticipantTariff, BulkMeterTariff:
		id := uuid.Parse(strings.TrimSpace(v))
		if id == nil {
			return "", errBulkRule("Tarif-ID ungültig")
		}
		return id.String(), nil
	}
	return "", errBulkRule("Wert ist für %s nicht erlaubt", r.Field)
}

// BulkItemKey identifies a hit: member id, and for the metering point tariff the metering point.
func BulkItemKey(participantId, meteringPointId string) string {
	return strings.ToLower(participantId) + "|" + meteringPointId
}

// ValidateItems checks the hits of an apply request: at least one, ids well-formed, a metering point exactly for
// METER_TARIFF, no hit twice (ErrBulkDuplicateItem). The participant ids are lower-cased. Whether the ids belong
// to the community is checked against the database (ErrBulkStalePreview).
func (r *BulkApplyRequest) ValidateItems() error {
	if len(r.Items) == 0 {
		return errBulkRule("keine Treffer zum Anwenden")
	}
	seen := make(map[string]bool, len(r.Items))
	for _, it := range r.Items {
		if it == nil {
			return errBulkRule("leerer Treffer")
		}
		id := uuid.Parse(it.ParticipantId)
		if id == nil {
			return errBulkRule("participantId ungültig")
		}
		it.ParticipantId = id.String()
		if (it.MeteringPointId != "") != (r.Rule.Field == BulkMeterTariff) {
			return errBulkRule("meteringPointId nur und immer bei %s", BulkMeterTariff)
		}
		key := BulkItemKey(it.ParticipantId, it.MeteringPointId)
		if seen[key] {
			return ErrBulkDuplicateItem(errors.New("Ein Treffer ist doppelt in der Anfrage."))
		}
		seen[key] = true
	}
	return nil
}

package model

import "testing"

func TestAllowedUpdateColumn_MeteringPoint(t *testing.T) {
	cases := []struct {
		field  string
		wantOK bool
		wantDB string
	}{
		{"equipmentName", true, "equipmentName"},
		{"inverterid", true, "inverterid"},
		{"tariff_id", true, "tariff_id"},
		{"transformer", true, "transformer"},
		// skipupdate / not updatable
		{"meteringPoint", false, ""},  // db metering_point_id, skipupdate
		{"participantId", false, ""},  // skipupdate
		{"participantState", false, ""}, // db:"-" via State, skipupdate
		// injected / unknown
		{`equipmentName"=(SELECT 1`, false, ""},
		{"evil", false, ""},
		{"", false, ""},
	}
	for _, c := range cases {
		gotDB, gotOK := AllowedUpdateColumn(MeteringPoint{}, c.field)
		if gotOK != c.wantOK || gotDB != c.wantDB {
			t.Errorf("AllowedUpdateColumn(%q) = (%q, %v), want (%q, %v)", c.field, gotDB, gotOK, c.wantDB, c.wantOK)
		}
	}
}

func TestAllowedUpdateColumn_EegProtectsFields(t *testing.T) {
	// tenant (db "tenant"), rcNumber and online are skipupdate → never settable.
	for _, f := range []string{"id", "rcNumber", "online", "createdAt"} {
		if _, ok := AllowedUpdateColumn(Eeg{}, f); ok {
			t.Errorf("Eeg field %q must not be updatable", f)
		}
	}
	// ordinary fields stay updatable.
	for _, f := range []string{"name", "description", "allocationMode"} {
		if _, ok := AllowedUpdateColumn(Eeg{}, f); !ok {
			t.Errorf("Eeg field %q should be updatable", f)
		}
	}
}

func TestIsAllowedParticipantUpdatePath(t *testing.T) {
	allowed := []string{
		"businessRole",
		"firstname",
		"contact.email",
		"contact.phone",
		"billingAddress.street",
		"residentAddress.city",
		"accountInfo.iban",
	}
	for _, p := range allowed {
		if !IsAllowedParticipantUpdatePath(p) {
			t.Errorf("path %q should be allowed", p)
		}
	}
	denied := []string{
		"id",                       // skipupdate
		"tenant",                   // not a field
		"contact.evil",             // unknown sub-field
		"unknownGroup.email",       // unknown group
		"accountInfo.type",         // not a BankInfo field
		`firstname"=(SELECT 1 /*`,  // injection
		"a.b.c",                    // too many segments
		"",
	}
	for _, p := range denied {
		if IsAllowedParticipantUpdatePath(p) {
			t.Errorf("path %q should be denied", p)
		}
	}
}

func TestSkipUpdateJSONKeys_Eeg(t *testing.T) {
	keys := SkipUpdateJSONKeys(Eeg{})
	set := map[string]bool{}
	for _, k := range keys {
		set[k] = true
	}
	for _, want := range []string{"id", "rcNumber", "online", "createdAt"} {
		if !set[want] {
			t.Errorf("skipupdate key %q missing from %v", want, keys)
		}
	}
	// updatable fields must NOT appear
	for _, no := range []string{"name", "description", "settlementInterval"} {
		if set[no] {
			t.Errorf("field %q must not be in skipupdate set", no)
		}
	}
}

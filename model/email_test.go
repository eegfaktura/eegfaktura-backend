package model

import "testing"

func TestNormalizeEmailList(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "a@x.at", "a@x.at"},
		{"leading space", " a@x.at", "a@x.at"},
		{"trailing space", "a@x.at ", "a@x.at"},
		{"nbsp", " a@x.at ", "a@x.at"},
		{"multi with spaces", "a@x.at; b@y.at", "a@x.at;b@y.at"},
		{"empty part dropped", "a@x.at;;b@y.at", "a@x.at;b@y.at"},
		{"only whitespace", "  ", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeEmailList(tt.in); got != tt.want {
				t.Errorf("NormalizeEmailList(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateEmailList(t *testing.T) {
	tests := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"plain valid", "a@x.at", "a@x.at", false},
		{"heals outer whitespace", " a@x.at ", "a@x.at", false},
		{"heals nbsp", " a@x.at", "a@x.at", false},
		{"multi healed", " a@x.at ; b@y.at ", "a@x.at;b@y.at", false},
		{"modern gtld", "a@eeg.energy", "a@eeg.energy", false},
		{"uppercase ok", "A@X.AT", "A@X.AT", false},
		{"empty is valid (no address)", "", "", false},
		{"whitespace-only is valid (no address)", " ", "", false},
		{"garbage", "x", "", true},
		{"umlaut local part", "hedwig.schön@x.at", "", true},
		{"inner whitespace", "a b@x.at", "", true},
		{"one bad part rejects all", "a@x.at;x", "", true},
		{"missing tld", "a@x", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateEmailList(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateEmailList(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ValidateEmailList(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

package model

import (
	"fmt"
	"regexp"
	"strings"
)

// emailPartRe is the shared address rule used across the eegfaktura
// suite (backend, eda-xp, billing, web): after trimming, exactly one
// address per part with an ASCII local part and a TLD of at least two
// letters — no TLD allowlist. Applied per ';'-separated part.
var emailPartRe = regexp.MustCompile(`^(?i)[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$`)

// NormalizeEmailList trims outer unicode whitespace (incl. NBSP) around
// each ';'-separated part, drops empty parts and re-joins with ';'
// (no spaces) — the canonical storage and wire format. Returns "" when
// nothing remains.
func NormalizeEmailList(email string) string {
	parts := strings.Split(email, ";")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			kept = append(kept, t)
		}
	}
	return strings.Join(kept, ";")
}

// ValidateEmailList normalizes the given address list and checks every
// part against the shared rule. An empty result (no address at all) is
// valid — a member without an e-mail is not an error. On success the
// normalized list is returned; on failure the offending part is named
// so tenant admins can correct the member data.
func ValidateEmailList(email string) (string, error) {
	normalized := NormalizeEmailList(email)
	if normalized == "" {
		return "", nil
	}
	for _, p := range strings.Split(normalized, ";") {
		if !emailPartRe.MatchString(p) {
			return "", fmt.Errorf("invalid email (%s)", p)
		}
	}
	return normalized, nil
}

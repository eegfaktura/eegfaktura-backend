package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMemberActivity(t *testing.T) {
	mp := func(s StatusType, p ProcessStatusType) *MeteringPoint { return &MeteringPoint{Status: s, ProcessState: p} }

	assert.Equal(t, S_ACTIVE, MemberActivity([]*MeteringPoint{mp(S_INACTIVE, INACTIVE), mp(S_ACTIVE, ACTIVE)}))
	// left: all metering points inactive
	assert.Equal(t, S_INACTIVE, MemberActivity([]*MeteringPoint{mp(S_INACTIVE, INACTIVE), mp(S_INACTIVE, REVOKED)}))
	// only rejected counts as inactive (decision 20.10.2026)
	assert.Equal(t, S_INACTIVE, MemberActivity([]*MeteringPoint{mp(S_INIT, REJECTED), mp(S_INIT, INVALID)}))
	// being registered, or no metering point yet: pending
	assert.Equal(t, StatusType(PENDING), MemberActivity([]*MeteringPoint{mp(S_INACTIVE, INACTIVE), mp(S_INIT, PENDING)}))
	assert.Equal(t, StatusType(PENDING), MemberActivity([]*MeteringPoint{mp(S_INIT, NEW)}))
	assert.Equal(t, StatusType(PENDING), MemberActivity(nil))
}

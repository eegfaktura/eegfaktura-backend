package eda

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Re-review backend#107 (2): an active metering point stays ACTIVE only if ALL codes of the rejection
// concern the request (model.REJECTED_KEEP_ACTIVE_CODES). The stored code decides it in the DAO, so a
// code about the metering point itself wins, in either order.
func TestRejectionCode(t *testing.T) {
	invalid := func(codes ...int16) int16 { return rejectionCode(intersectCodes(REJECTED_INVALID_CODES, codes), codes) }

	assert.Equal(t, int16(57), invalid(188, 57))
	assert.Equal(t, int16(57), invalid(57, 188))
	assert.Equal(t, int16(188), invalid(188))
	assert.Equal(t, int16(188), invalid(188, 173))
	assert.Equal(t, int16(56), invalid(56, 57), "first code as before")
	// a code outside REJECTED_INVALID_CODES also counts as about the metering point
	assert.Equal(t, int16(99), invalid(188, 99))

	// REJECTED (no INVALID code): candidates are all codes
	assert.Equal(t, int16(99), rejectionCode([]int16{183, 99}, []int16{183, 99}))
	assert.Equal(t, int16(183), rejectionCode([]int16{183}, []int16{183}))
}

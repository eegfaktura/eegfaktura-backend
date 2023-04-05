package database

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetMeteringPoint(t *testing.T) {
	eeg, err := GetEeg("RC100181")
	assert.NoError(t, err)

	assert.NotEmpty(t, eeg)
	fmt.Printf("EEG: %+v\n", eeg)
}

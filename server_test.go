package main

import (
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// 261008-ca1: without LOG_LEVEL, or with an unknown value, the backend logs at info, not debug.
func TestLogLevelDefaultsToInfo(t *testing.T) {
	ll, err := logLevel("", false)
	assert.NoError(t, err)
	assert.Equal(t, log.InfoLevel, ll)

	ll, err = logLevel("verbose", true)
	assert.Error(t, err)
	assert.Equal(t, log.InfoLevel, ll)

	ll, err = logLevel("debug", true)
	assert.NoError(t, err)
	assert.Equal(t, log.DebugLevel, ll)
}

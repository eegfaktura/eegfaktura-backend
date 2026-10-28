package services

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSendTimeout(t *testing.T) {
	logo := &Attachment{Filecontent: bytes.NewBuffer(make([]byte, 10*1024))}
	assert.Equal(t, 5*time.Second, sendTimeout([]*Attachment{logo}, nil))
	pdf := &Attachment{Filecontent: bytes.NewBuffer(make([]byte, 3*1024*1024))}
	assert.Equal(t, 30*time.Second, sendTimeout([]*Attachment{logo}, pdf))
	assert.Equal(t, 5*time.Second, sendTimeout(nil, nil))
}

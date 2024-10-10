package config

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReadActivationMailTemplateConfig(t *testing.T) {
	config, err := ReadActivationMailTemplateConfig("./activation-mail-templates.toml")
	assert.NoError(t, err)
	fmt.Printf("CONFIG: %+v\n", config)
}

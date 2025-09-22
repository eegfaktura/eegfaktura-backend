package config

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReadActivationMailTemplateConfig(t *testing.T) {
	config, err := ReadActivationMailTemplateConfig("../public/templates/activation-mail-template.toml")
	assert.NoError(t, err)
	assert.Equal(t, "AktivierungsEmail-template.html", config.TemplateFile)
	require.Equal(t, 1, len(config.InlinePictures))
	assert.Equal(t, "Logo_Faktura.png", config.InlinePictures[0].Filepath)
}

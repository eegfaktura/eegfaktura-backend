package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadActivationMailTemplateConfig(t *testing.T) {
	config, err := ReadActivationMailTemplateConfig(os.DirFS("../public/templates"), "activation-mail-template.toml")
	assert.NoError(t, err)
	assert.Equal(t, "AktivierungsEmail-template.html", config.TemplateFile)
	require.Equal(t, 1, len(config.InlinePictures))
	assert.Equal(t, "eegfaktura-logo.png", config.InlinePictures[0].Filepath)
}

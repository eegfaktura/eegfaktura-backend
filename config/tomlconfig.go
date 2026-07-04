package config

import (
	"fmt"
	"io/fs"

	"at.ourproject/vfeeg-backend/model"
	"github.com/BurntSushi/toml"
)

// ReadActivationMailTemplateConfig reads the mail template descriptor named
// name from fsys — a per-tenant or global templates dir on the data volume, or
// the templates embedded in the binary.
func ReadActivationMailTemplateConfig(fsys fs.FS, name string) (*model.ActivationMailTemplate, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("Config file is missing: %s", name)
	}

	var config model.ActivationMailTemplate
	if _, err := toml.Decode(string(data), &config); err != nil {
		return nil, fmt.Errorf("Config file is not able to parse: %s", name)
	}
	return &config, nil
}

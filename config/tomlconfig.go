package config

import (
	"at.ourproject/vfeeg-backend/model"
	"errors"
	"fmt"
	"github.com/BurntSushi/toml"
	"os"
)

// Reads info from config file
func ReadActivationMailTemplateConfig(configFile string) (*model.ActivationMailTemplate, error) {
	_, err := os.Stat(configFile)
	if err != nil {
		fmt.Printf("Error: %+v\n", err)
		return nil, errors.New(fmt.Sprintf("Config file is missing: %s", configFile))
	}

	var config model.ActivationMailTemplate
	if _, err := toml.DecodeFile(configFile, &config); err != nil {
		fmt.Printf("Error: %+v\n", err)
		return nil, errors.New(fmt.Sprintf("Config file is not able to parse: %s", configFile))
	}
	return &config, nil
}

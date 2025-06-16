package cmd

import (
	"at.ourproject/vfeeg-backend/config"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

var (
	configPath string
)

func init() {
	RootCmd.PersistentFlags().StringVar(&configPath, "configPath", ".",
		"Config Path. (required)")
}

func validateRootCmdArgs(cmd *cobra.Command, args []string) error {
	if strings.HasPrefix(cmd.Use, "help ") { // No need to validate if it is help
		return nil
	}
	if configPath == "" {
		return errors.New("--config path not specified")
	}

	config.ReadConfig(configPath)

	return nil
}

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:               "vfeeg-backend",
	Short:             "VFEEG Backend process CLI",
	PersistentPreRunE: validateRootCmdArgs,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

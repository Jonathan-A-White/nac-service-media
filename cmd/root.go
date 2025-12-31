package cmd

import (
	"fmt"
	"os"

	"nac-service-media/infrastructure/config"

	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "nac-service-media",
	Short: "Automate church service media processing and distribution",
	Long: `nac-service-media automates the workflow of processing and distributing
church service recordings:

  - Trim video by start/end timestamps
  - Extract audio as MP3
  - Upload to Google Drive with sharing
  - Send email notifications with links

Example:
  nac-service-media process --source recording.mp4 --start 00:05:30 --end 01:15:00`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./config/config.yaml)")
}

func initConfig() {
	if cfgFile == "" {
		cfgFile = "config/config.yaml"
	}

	var err error
	cfg, err = config.Load(cfgFile)
	if err != nil {
		// Config file is optional for some commands (like help)
		// Commands that need config will check and error appropriately
		cfg = nil
	}
}

// GetConfig returns the loaded configuration
func GetConfig() *config.Config {
	return cfg
}

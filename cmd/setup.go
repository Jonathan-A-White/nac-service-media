package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"nac-service-media/infrastructure/config"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

// Prompter interface for interactive prompts (allows mocking in tests)
type Prompter interface {
	Input(message string, defaultValue string) (string, error)
	Confirm(message string, defaultValue bool) (bool, error)
}

// SurveyPrompter implements Prompter using the survey library
type SurveyPrompter struct{}

func (p *SurveyPrompter) Input(message string, defaultValue string) (string, error) {
	result := ""
	prompt := &survey.Input{
		Message: message,
		Default: defaultValue,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return result, nil
}

func (p *SurveyPrompter) Confirm(message string, defaultValue bool) (bool, error) {
	result := defaultValue
	prompt := &survey.Confirm{
		Message: message,
		Default: defaultValue,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return false, err
	}
	return result, nil
}

// DefaultPrompter is the prompter used in production
var DefaultPrompter Prompter = &SurveyPrompter{}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Create configuration file interactively",
	Long: `Prompts for configuration values and creates config.yaml.

This command guides you through setting up your configuration file
with all necessary paths, Google Drive settings, and email recipients.`,
	RunE: runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	return RunSetupWithPrompter(DefaultPrompter, "config/config.yaml")
}

// RunSetupWithPrompter runs the setup with a given prompter (for testing)
func RunSetupWithPrompter(prompter Prompter, configPath string) error {
	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil {
		overwrite, err := prompter.Confirm("config.yaml already exists. Overwrite?", false)
		if err != nil {
			return fmt.Errorf("prompt cancelled")
		}
		if !overwrite {
			fmt.Println("Setup cancelled.")
			return nil
		}
	}

	fmt.Println("Welcome to nac-service-media setup!")
	fmt.Println()

	cfg := &config.Config{}

	// Paths section
	if err := promptPaths(prompter, cfg); err != nil {
		return err
	}

	// Audio section
	if err := promptAudio(prompter, cfg); err != nil {
		return err
	}

	// Google section
	if err := promptGoogle(prompter, cfg); err != nil {
		return err
	}

	// Email section
	if err := promptEmail(prompter, cfg); err != nil {
		return err
	}

	// Ensure config directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Save configuration
	if err := config.Save(cfg, configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println()
	fmt.Printf("Configuration saved to %s\n", configPath)
	return nil
}

func promptPaths(prompter Prompter, cfg *config.Config) error {
	source, err := prompter.Input("Where does OBS save recordings?", "")
	if err != nil {
		return fmt.Errorf("prompt cancelled")
	}
	if source == "" {
		return fmt.Errorf("source directory is required")
	}
	cfg.Paths.SourceDirectory = source

	trimmed, err := prompter.Input("Where should trimmed videos go?", "")
	if err != nil {
		return fmt.Errorf("prompt cancelled")
	}
	if trimmed == "" {
		return fmt.Errorf("trimmed directory is required")
	}
	cfg.Paths.TrimmedDirectory = trimmed

	audio, err := prompter.Input("Where should audio files go?", "")
	if err != nil {
		return fmt.Errorf("prompt cancelled")
	}
	if audio == "" {
		return fmt.Errorf("audio directory is required")
	}
	cfg.Paths.AudioDirectory = audio

	return nil
}

func promptAudio(prompter Prompter, cfg *config.Config) error {
	bitrate, err := prompter.Input("Audio bitrate for mp3 extraction?", "192k")
	if err != nil {
		return fmt.Errorf("prompt cancelled")
	}
	if bitrate == "" {
		bitrate = "192k"
	}
	cfg.Audio.Bitrate = bitrate
	return nil
}

func promptGoogle(prompter Prompter, cfg *config.Config) error {
	credentials, err := prompter.Input("Path to Google credentials file?", "credentials.json")
	if err != nil {
		return fmt.Errorf("prompt cancelled")
	}
	if credentials == "" {
		credentials = "credentials.json"
	}
	cfg.Google.CredentialsFile = credentials

	folder, err := prompter.Input("Google Drive folder ID for Services?", "")
	if err != nil {
		return fmt.Errorf("prompt cancelled")
	}
	if folder == "" {
		return fmt.Errorf("folder ID is required")
	}
	cfg.Google.ServicesFolderID = folder

	return nil
}

func promptEmail(prompter Prompter, cfg *config.Config) error {
	// From details
	fromName, err := prompter.Input("Display name for outgoing emails?", "")
	if err != nil {
		return fmt.Errorf("prompt cancelled")
	}
	if fromName == "" {
		return fmt.Errorf("from name is required")
	}
	cfg.Email.FromName = fromName

	fromAddress, err := prompter.Input("Gmail address to send from?", "")
	if err != nil {
		return fmt.Errorf("prompt cancelled")
	}
	if fromAddress == "" {
		return fmt.Errorf("from address is required")
	}
	cfg.Email.FromAddress = fromAddress

	// Default CC recipients
	cfg.Email.DefaultCC = []config.RecipientConfig{}
	for {
		addCC, err := prompter.Confirm("Add a CC recipient?", false)
		if err != nil {
			return fmt.Errorf("prompt cancelled")
		}
		if !addCC {
			break
		}

		recipient, err := promptRecipientWithPrompter(prompter)
		if err != nil {
			return err
		}
		cfg.Email.DefaultCC = append(cfg.Email.DefaultCC, recipient)
	}

	// Quick-lookup recipients
	cfg.Email.Recipients = make(map[string]config.RecipientConfig)
	for {
		addRecipient, err := prompter.Confirm("Add a quick-lookup recipient?", false)
		if err != nil {
			return fmt.Errorf("prompt cancelled")
		}
		if !addRecipient {
			break
		}

		nickname, err := prompter.Input("  Nickname:", "")
		if err != nil {
			return fmt.Errorf("prompt cancelled")
		}
		if nickname == "" {
			return fmt.Errorf("nickname is required")
		}

		recipient, err := promptRecipientWithPrompter(prompter)
		if err != nil {
			return err
		}
		cfg.Email.Recipients[nickname] = recipient
	}

	return nil
}

func promptRecipientWithPrompter(prompter Prompter) (config.RecipientConfig, error) {
	name, err := prompter.Input("  Full name:", "")
	if err != nil {
		return config.RecipientConfig{}, fmt.Errorf("prompt cancelled")
	}
	if name == "" {
		return config.RecipientConfig{}, fmt.Errorf("name is required")
	}

	address, err := prompter.Input("  Email:", "")
	if err != nil {
		return config.RecipientConfig{}, fmt.Errorf("prompt cancelled")
	}
	if address == "" {
		return config.RecipientConfig{}, fmt.Errorf("email is required")
	}

	return config.RecipientConfig{
		Name:    name,
		Address: address,
	}, nil
}

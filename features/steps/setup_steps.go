//go:build integration

package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nac-service-media/cmd"
	"nac-service-media/infrastructure/config"

	"github.com/cucumber/godog"
)

type setupContext struct {
	tempDir          string
	configPath       string
	setupCancelled   bool
	originalContent  string
	promptResponses  map[string]string
	confirmResponses map[string]bool
	err              error
}

var SharedSetupContext = &setupContext{}

// MockPrompter implements cmd.Prompter for testing
type MockPrompter struct {
	inputResponses   []string
	confirmResponses []bool
	inputIndex       int
	confirmIndex     int
}

func NewMockPrompter(inputs []string, confirms []bool) *MockPrompter {
	return &MockPrompter{
		inputResponses:   inputs,
		confirmResponses: confirms,
	}
}

func (m *MockPrompter) Input(message string, defaultValue string) (string, error) {
	if m.inputIndex >= len(m.inputResponses) {
		if defaultValue != "" {
			return defaultValue, nil
		}
		return "", fmt.Errorf("no more input responses available for message: %s", message)
	}
	response := m.inputResponses[m.inputIndex]
	m.inputIndex++
	return response, nil
}

func (m *MockPrompter) Confirm(message string, defaultValue bool) (bool, error) {
	if m.confirmIndex >= len(m.confirmResponses) {
		return defaultValue, nil
	}
	response := m.confirmResponses[m.confirmIndex]
	m.confirmIndex++
	return response, nil
}

func InitializeSetupScenario(ctx *godog.ScenarioContext) {
	testCtx := SharedSetupContext

	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		// Create temp directory for each scenario
		tempDir, err := os.MkdirTemp("", "setup-test-*")
		if err != nil {
			return c, err
		}
		testCtx.tempDir = tempDir
		testCtx.configPath = filepath.Join(tempDir, "config", "config.yaml")
		testCtx.setupCancelled = false
		testCtx.originalContent = ""
		testCtx.promptResponses = make(map[string]string)
		testCtx.confirmResponses = make(map[string]bool)
		testCtx.err = nil
		return c, nil
	})

	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		// Cleanup temp directory
		if testCtx.tempDir != "" {
			os.RemoveAll(testCtx.tempDir)
		}
		SharedSetupContext = &setupContext{}
		return c, nil
	})

	ctx.Step(`^no config file exists for setup$`, testCtx.noConfigFileExistsForSetup)
	ctx.Step(`^a config file already exists for setup$`, testCtx.aConfigFileAlreadyExistsForSetup)
	ctx.Step(`^I run the setup command with inputs:$`, testCtx.iRunTheSetupCommandWithInputs)
	ctx.Step(`^I run the setup command with confirmation "([^"]*)"$`, testCtx.iRunTheSetupCommandWithConfirmation)
	ctx.Step(`^I run the setup command with confirmation "([^"]*)" and inputs:$`, testCtx.iRunTheSetupCommandWithConfirmationAndInputs)
	ctx.Step(`^a config file should exist$`, testCtx.aConfigFileShouldExist)
	ctx.Step(`^the config should have source_directory "([^"]*)"$`, testCtx.theConfigShouldHaveSourceDirectory)
	ctx.Step(`^the config should have trimmed_directory "([^"]*)"$`, testCtx.theConfigShouldHaveTrimmedDirectory)
	ctx.Step(`^the config should have audio_directory "([^"]*)"$`, testCtx.theConfigShouldHaveAudioDirectory)
	ctx.Step(`^the config should have services_folder_id "([^"]*)"$`, testCtx.theConfigShouldHaveServicesFolderId)
	ctx.Step(`^the config should have a CC recipient "([^"]*)"$`, testCtx.theConfigShouldHaveACCRecipient)
	ctx.Step(`^the config should have a quick-lookup recipient "([^"]*)"$`, testCtx.theConfigShouldHaveAQuickLookupRecipient)
	ctx.Step(`^the setup should be cancelled$`, testCtx.theSetupShouldBeCancelled)
	ctx.Step(`^the existing config should be unchanged$`, testCtx.theExistingConfigShouldBeUnchanged)
}

func (s *setupContext) noConfigFileExistsForSetup() error {
	// Just ensure the config path directory exists but no config file
	configDir := filepath.Dir(s.configPath)
	return os.MkdirAll(configDir, 0755)
}

func (s *setupContext) aConfigFileAlreadyExistsForSetup() error {
	// Create the config file with some content
	configDir := filepath.Dir(s.configPath)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	content := `paths:
  source_directory: "/original/source"
  trimmed_directory: "/original/trimmed"
  audio_directory: "/original/audio"
audio:
  bitrate: "192k"
google:
  credentials_file: "original-creds.json"
  services_folder_id: "original-folder-id"
email:
  from_name: "Original Church"
  from_address: "original@example.com"
`
	s.originalContent = content
	return os.WriteFile(s.configPath, []byte(content), 0644)
}

func (s *setupContext) iRunTheSetupCommandWithInputs(table *godog.Table) error {
	inputs, confirms := parseInputTable(table)
	prompter := NewMockPrompter(inputs, confirms)

	s.err = cmd.RunSetupWithPrompter(prompter, s.configPath)
	if s.err != nil {
		return fmt.Errorf("setup command failed: %w", s.err)
	}
	return nil
}

func (s *setupContext) iRunTheSetupCommandWithConfirmation(confirmation string) error {
	confirm := strings.ToLower(confirmation) == "y"
	prompter := NewMockPrompter([]string{}, []bool{confirm})

	s.err = cmd.RunSetupWithPrompter(prompter, s.configPath)
	if !confirm {
		s.setupCancelled = true
	}
	return nil
}

func (s *setupContext) iRunTheSetupCommandWithConfirmationAndInputs(confirmation string, table *godog.Table) error {
	confirm := strings.ToLower(confirmation) == "y"
	inputs, confirms := parseInputTable(table)

	// Prepend the overwrite confirmation
	allConfirms := append([]bool{confirm}, confirms...)
	prompter := NewMockPrompter(inputs, allConfirms)

	s.err = cmd.RunSetupWithPrompter(prompter, s.configPath)
	if s.err != nil {
		return fmt.Errorf("setup command failed: %w", s.err)
	}
	return nil
}

func parseInputTable(table *godog.Table) ([]string, []bool) {
	var inputs []string
	var confirms []bool

	for i, row := range table.Rows {
		if i == 0 {
			continue // Skip header row
		}
		prompt := strings.ToLower(row.Cells[0].Value)
		value := row.Cells[1].Value

		// Check if this is a confirm prompt (starts with "Add")
		if strings.HasPrefix(prompt, "add") {
			confirms = append(confirms, strings.ToLower(value) == "y")
		} else {
			inputs = append(inputs, value)
		}
	}

	return inputs, confirms
}

func (s *setupContext) aConfigFileShouldExist() error {
	if _, err := os.Stat(s.configPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist at %s", s.configPath)
	}
	return nil
}

func (s *setupContext) theConfigShouldHaveSourceDirectory(expected string) error {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg.Paths.SourceDirectory != expected {
		return fmt.Errorf("expected source_directory %q, got %q", expected, cfg.Paths.SourceDirectory)
	}
	return nil
}

func (s *setupContext) theConfigShouldHaveTrimmedDirectory(expected string) error {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg.Paths.TrimmedDirectory != expected {
		return fmt.Errorf("expected trimmed_directory %q, got %q", expected, cfg.Paths.TrimmedDirectory)
	}
	return nil
}

func (s *setupContext) theConfigShouldHaveAudioDirectory(expected string) error {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg.Paths.AudioDirectory != expected {
		return fmt.Errorf("expected audio_directory %q, got %q", expected, cfg.Paths.AudioDirectory)
	}
	return nil
}

func (s *setupContext) theConfigShouldHaveServicesFolderId(expected string) error {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg.Google.ServicesFolderID != expected {
		return fmt.Errorf("expected services_folder_id %q, got %q", expected, cfg.Google.ServicesFolderID)
	}
	return nil
}

func (s *setupContext) theConfigShouldHaveACCRecipient(expectedName string) error {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	for _, cc := range cfg.Email.DefaultCC {
		if cc.Name == expectedName {
			return nil
		}
	}
	return fmt.Errorf("CC recipient %q not found in %v", expectedName, cfg.Email.DefaultCC)
}

func (s *setupContext) theConfigShouldHaveAQuickLookupRecipient(nickname string) error {
	cfg, err := config.Load(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if _, ok := cfg.Email.Recipients[nickname]; !ok {
		return fmt.Errorf("quick-lookup recipient %q not found in %v", nickname, cfg.Email.Recipients)
	}
	return nil
}

func (s *setupContext) theSetupShouldBeCancelled() error {
	if !s.setupCancelled {
		return fmt.Errorf("expected setup to be cancelled")
	}
	return nil
}

func (s *setupContext) theExistingConfigShouldBeUnchanged() error {
	content, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}
	if string(content) != s.originalContent {
		return fmt.Errorf("config content was changed")
	}
	return nil
}

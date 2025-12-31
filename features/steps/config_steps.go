//go:build integration

package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"nac-service-media/infrastructure/config"

	"github.com/cucumber/godog"
)

type configContext struct {
	configPath string
	cfg        *config.Config
	loadErr    error
}

// SharedConfigContext is reset before each scenario via After hook
var SharedConfigContext = &configContext{}

func InitializeConfigScenario(ctx *godog.ScenarioContext) {
	testCtx := SharedConfigContext

	// Reset context after each scenario
	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		SharedConfigContext = &configContext{}
		return c, nil
	})

	ctx.Step(`^a configuration file exists at "([^"]*)"$`, testCtx.aConfigurationFileExistsAt)
	ctx.Step(`^no configuration file exists at "([^"]*)"$`, testCtx.noConfigurationFileExistsAt)
	ctx.Step(`^I load the configuration$`, testCtx.iLoadTheConfiguration)
	ctx.Step(`^I attempt to load the configuration$`, testCtx.iAttemptToLoadTheConfiguration)
	ctx.Step(`^the trimmed directory should be "([^"]*)"$`, testCtx.theTrimmedDirectoryShouldBe)
	ctx.Step(`^the audio directory should be "([^"]*)"$`, testCtx.theAudioDirectoryShouldBe)
	ctx.Step(`^the Google services folder ID should be "([^"]*)"$`, testCtx.theGoogleServicesFolderIDShouldBe)
	ctx.Step(`^I should receive an error about missing configuration$`, testCtx.iShouldReceiveAnErrorAboutMissingConfiguration)
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find project root (no go.mod found)")
		}
		dir = parent
	}
}

func (c *configContext) aConfigurationFileExistsAt(path string) error {
	root, err := findProjectRoot()
	if err != nil {
		return err
	}
	c.configPath = filepath.Join(root, path)

	// Verify file actually exists
	if _, err := os.Stat(c.configPath); err != nil {
		return fmt.Errorf("expected config file at %s but it does not exist: %w", c.configPath, err)
	}
	return nil
}

func (c *configContext) noConfigurationFileExistsAt(path string) error {
	root, err := findProjectRoot()
	if err != nil {
		return err
	}
	c.configPath = filepath.Join(root, path)
	return nil
}

func (c *configContext) iLoadTheConfiguration() error {
	cfg, err := config.Load(c.configPath)
	if err != nil {
		return fmt.Errorf("unexpected error loading config: %w", err)
	}
	c.cfg = cfg
	return nil
}

func (c *configContext) iAttemptToLoadTheConfiguration() error {
	cfg, err := config.Load(c.configPath)
	c.cfg = cfg
	c.loadErr = err
	return nil
}

func (c *configContext) theTrimmedDirectoryShouldBe(expected string) error {
	if c.cfg == nil {
		return fmt.Errorf("config was not loaded")
	}
	if c.cfg.Paths.TrimmedDirectory != expected {
		return fmt.Errorf("expected trimmed directory %q, got %q", expected, c.cfg.Paths.TrimmedDirectory)
	}
	return nil
}

func (c *configContext) theAudioDirectoryShouldBe(expected string) error {
	if c.cfg == nil {
		return fmt.Errorf("config was not loaded")
	}
	if c.cfg.Paths.AudioDirectory != expected {
		return fmt.Errorf("expected audio directory %q, got %q", expected, c.cfg.Paths.AudioDirectory)
	}
	return nil
}

func (c *configContext) theGoogleServicesFolderIDShouldBe(expected string) error {
	if c.cfg == nil {
		return fmt.Errorf("config was not loaded")
	}
	if c.cfg.Google.ServicesFolderID != expected {
		return fmt.Errorf("expected services folder ID %q, got %q", expected, c.cfg.Google.ServicesFolderID)
	}
	return nil
}

func (c *configContext) iShouldReceiveAnErrorAboutMissingConfiguration() error {
	if c.loadErr == nil {
		return fmt.Errorf("expected an error but got none")
	}
	return nil
}

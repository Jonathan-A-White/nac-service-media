//go:build integration

package steps

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"nac-service-media/cmd"
	"nac-service-media/infrastructure/config"

	"github.com/cucumber/godog"
)

type configCrudContext struct {
	tempDir    string
	configPath string
	config     *config.Config
	output     *bytes.Buffer
	err        error
}

var SharedConfigCrudContext = &configCrudContext{}

func InitializeConfigCrudScenario(ctx *godog.ScenarioContext) {
	testCtx := SharedConfigCrudContext

	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		// Create temp directory for each scenario
		tempDir, err := os.MkdirTemp("", "config-crud-test-*")
		if err != nil {
			return c, err
		}
		testCtx.tempDir = tempDir
		testCtx.configPath = filepath.Join(tempDir, "config.yaml")
		testCtx.output = &bytes.Buffer{}
		testCtx.err = nil
		testCtx.config = nil
		return c, nil
	})

	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		// Cleanup temp directory
		if testCtx.tempDir != "" {
			os.RemoveAll(testCtx.tempDir)
		}
		SharedConfigCrudContext = &configCrudContext{}
		return c, nil
	})

	// Background
	ctx.Step(`^a config file exists with initial data$`, testCtx.aConfigFileExistsWithInitialData)

	// Minister steps
	ctx.Step(`^I run config add minister with key "([^"]*)" and name "([^"]*)"$`, testCtx.iRunConfigAddMinister)
	ctx.Step(`^minister "([^"]*)" exists with name "([^"]*)"$`, testCtx.ministerExistsWithName)
	ctx.Step(`^I run config list ministers$`, testCtx.iRunConfigListMinisters)
	ctx.Step(`^I run config remove minister "([^"]*)"$`, testCtx.iRunConfigRemoveMinister)
	ctx.Step(`^I run config update minister "([^"]*)" with name "([^"]*)"$`, testCtx.iRunConfigUpdateMinister)
	ctx.Step(`^the config should contain minister "([^"]*)" with name "([^"]*)"$`, testCtx.theConfigShouldContainMinister)
	ctx.Step(`^the config should not contain minister "([^"]*)"$`, testCtx.theConfigShouldNotContainMinister)

	// Recipient steps
	ctx.Step(`^I run config add recipient with key "([^"]*)" name "([^"]*)" and email "([^"]*)"$`, testCtx.iRunConfigAddRecipient)
	ctx.Step(`^recipient "([^"]*)" exists with name "([^"]*)" and email "([^"]*)"$`, testCtx.recipientExistsWithNameAndEmail)
	ctx.Step(`^I run config list recipients$`, testCtx.iRunConfigListRecipients)
	ctx.Step(`^I run config remove recipient "([^"]*)"$`, testCtx.iRunConfigRemoveRecipient)
	ctx.Step(`^I run config update recipient "([^"]*)" with email "([^"]*)"$`, testCtx.iRunConfigUpdateRecipientEmail)
	ctx.Step(`^I run config update recipient "([^"]*)" with name "([^"]*)" and email "([^"]*)"$`, testCtx.iRunConfigUpdateRecipientNameAndEmail)
	ctx.Step(`^the config should contain recipient "([^"]*)" with name "([^"]*)" and email "([^"]*)"$`, testCtx.theConfigShouldContainRecipient)
	ctx.Step(`^the config should not contain recipient "([^"]*)"$`, testCtx.theConfigShouldNotContainRecipient)

	// CC steps
	ctx.Step(`^I run config add cc with key "([^"]*)" name "([^"]*)" and email "([^"]*)"$`, testCtx.iRunConfigAddCC)
	ctx.Step(`^cc exists with name "([^"]*)" and email "([^"]*)"$`, testCtx.ccExistsWithNameAndEmail)
	ctx.Step(`^I run config list ccs$`, testCtx.iRunConfigListCCs)
	ctx.Step(`^I run config remove cc "([^"]*)"$`, testCtx.iRunConfigRemoveCC)
	ctx.Step(`^I run config update cc "([^"]*)" with email "([^"]*)"$`, testCtx.iRunConfigUpdateCCEmail)
	ctx.Step(`^the config should contain cc with name "([^"]*)" and email "([^"]*)"$`, testCtx.theConfigShouldContainCC)
	ctx.Step(`^the config should not contain cc with name "([^"]*)"$`, testCtx.theConfigShouldNotContainCC)

	// Sender steps
	ctx.Step(`^I run config add sender with key "([^"]*)" and name "([^"]*)"$`, testCtx.iRunConfigAddSender)
	ctx.Step(`^sender "([^"]*)" exists with name "([^"]*)"$`, testCtx.senderExistsWithName)
	ctx.Step(`^I run config list senders$`, testCtx.iRunConfigListSenders)
	ctx.Step(`^I run config remove sender "([^"]*)"$`, testCtx.iRunConfigRemoveSender)
	ctx.Step(`^I run config update sender "([^"]*)" with name "([^"]*)"$`, testCtx.iRunConfigUpdateSender)
	ctx.Step(`^the config should contain sender "([^"]*)" with name "([^"]*)"$`, testCtx.theConfigShouldContainSender)
	ctx.Step(`^the config should not contain sender "([^"]*)"$`, testCtx.theConfigShouldNotContainSender)

	// Common assertions
	ctx.Step(`^the command should succeed$`, testCtx.theCommandShouldSucceed)
	ctx.Step(`^the command should fail with "([^"]*)"$`, testCtx.theCommandShouldFailWith)
	ctx.Step(`^the output should contain "([^"]*)"$`, testCtx.theOutputShouldContain)
}

func (c *configCrudContext) loadConfig() error {
	cfg, err := config.Load(c.configPath)
	if err != nil {
		return err
	}
	c.config = cfg
	return nil
}

func (c *configCrudContext) saveConfig() error {
	return config.Save(c.config, c.configPath)
}

// --- Background ---

func (c *configCrudContext) aConfigFileExistsWithInitialData() error {
	c.config = &config.Config{
		Paths: config.PathsConfig{
			SourceDirectory:  "/source",
			TrimmedDirectory: "/trimmed",
			AudioDirectory:   "/audio",
		},
		Audio: config.AudioConfig{
			Bitrate: "192k",
		},
		Google: config.GoogleConfig{
			CredentialsFile:  "credentials.json",
			ServicesFolderID: "folder-id",
		},
		Email: config.EmailConfig{
			FromName:    "Test Church",
			FromAddress: "test@example.com",
			DefaultCC:   []config.RecipientConfig{},
			Recipients:  make(map[string]config.RecipientConfig),
		},
		Ministers: make(map[string]config.MinisterConfig),
	}
	return c.saveConfig()
}

// --- Minister steps ---

func (c *configCrudContext) iRunConfigAddMinister(key, name string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigAddWithDependencies(c.config, c.configPath, "minister", key, name, "", c.output)
	return nil
}

func (c *configCrudContext) ministerExistsWithName(key, name string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	if c.config.Ministers == nil {
		c.config.Ministers = make(map[string]config.MinisterConfig)
	}
	c.config.Ministers[strings.ToLower(key)] = config.MinisterConfig{Name: name}
	return c.saveConfig()
}

func (c *configCrudContext) iRunConfigListMinisters() error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigListWithDependencies(c.config, c.configPath, "ministers", c.output)
	return nil
}

func (c *configCrudContext) iRunConfigRemoveMinister(key string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigRemoveWithDependencies(c.config, c.configPath, "minister", key, c.output)
	return nil
}

func (c *configCrudContext) iRunConfigUpdateMinister(key, name string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigUpdateWithDependencies(c.config, c.configPath, "minister", key, name, "", c.output)
	return nil
}

func (c *configCrudContext) theConfigShouldContainMinister(key, name string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	key = strings.ToLower(key)
	m, exists := c.config.Ministers[key]
	if !exists {
		return fmt.Errorf("minister %q not found in config", key)
	}
	if m.Name != name {
		return fmt.Errorf("expected minister %q to have name %q, got %q", key, name, m.Name)
	}
	return nil
}

func (c *configCrudContext) theConfigShouldNotContainMinister(key string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	key = strings.ToLower(key)
	if _, exists := c.config.Ministers[key]; exists {
		return fmt.Errorf("minister %q should not exist in config", key)
	}
	return nil
}

// --- Recipient steps ---

func (c *configCrudContext) iRunConfigAddRecipient(key, name, email string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigAddWithDependencies(c.config, c.configPath, "recipient", key, name, email, c.output)
	return nil
}

func (c *configCrudContext) recipientExistsWithNameAndEmail(key, name, email string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	if c.config.Email.Recipients == nil {
		c.config.Email.Recipients = make(map[string]config.RecipientConfig)
	}
	c.config.Email.Recipients[strings.ToLower(key)] = config.RecipientConfig{Name: name, Address: email}
	return c.saveConfig()
}

func (c *configCrudContext) iRunConfigListRecipients() error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigListWithDependencies(c.config, c.configPath, "recipients", c.output)
	return nil
}

func (c *configCrudContext) iRunConfigRemoveRecipient(key string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigRemoveWithDependencies(c.config, c.configPath, "recipient", key, c.output)
	return nil
}

func (c *configCrudContext) iRunConfigUpdateRecipientEmail(key, email string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigUpdateWithDependencies(c.config, c.configPath, "recipient", key, "", email, c.output)
	return nil
}

func (c *configCrudContext) iRunConfigUpdateRecipientNameAndEmail(key, name, email string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigUpdateWithDependencies(c.config, c.configPath, "recipient", key, name, email, c.output)
	return nil
}

func (c *configCrudContext) theConfigShouldContainRecipient(key, name, email string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	key = strings.ToLower(key)
	r, exists := c.config.Email.Recipients[key]
	if !exists {
		return fmt.Errorf("recipient %q not found in config", key)
	}
	if r.Name != name {
		return fmt.Errorf("expected recipient %q to have name %q, got %q", key, name, r.Name)
	}
	if r.Address != email {
		return fmt.Errorf("expected recipient %q to have email %q, got %q", key, email, r.Address)
	}
	return nil
}

func (c *configCrudContext) theConfigShouldNotContainRecipient(key string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	key = strings.ToLower(key)
	if _, exists := c.config.Email.Recipients[key]; exists {
		return fmt.Errorf("recipient %q should not exist in config", key)
	}
	return nil
}

// --- CC steps ---

func (c *configCrudContext) iRunConfigAddCC(key, name, email string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigAddWithDependencies(c.config, c.configPath, "cc", key, name, email, c.output)
	return nil
}

func (c *configCrudContext) ccExistsWithNameAndEmail(name, email string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.config.Email.DefaultCC = append(c.config.Email.DefaultCC, config.RecipientConfig{
		Name:    name,
		Address: email,
	})
	return c.saveConfig()
}

func (c *configCrudContext) iRunConfigListCCs() error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigListWithDependencies(c.config, c.configPath, "ccs", c.output)
	return nil
}

func (c *configCrudContext) iRunConfigRemoveCC(key string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigRemoveWithDependencies(c.config, c.configPath, "cc", key, c.output)
	return nil
}

func (c *configCrudContext) iRunConfigUpdateCCEmail(key, email string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigUpdateWithDependencies(c.config, c.configPath, "cc", key, "", email, c.output)
	return nil
}

func (c *configCrudContext) theConfigShouldContainCC(name, email string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	for _, cc := range c.config.Email.DefaultCC {
		if cc.Name == name && cc.Address == email {
			return nil
		}
	}
	return fmt.Errorf("cc with name %q and email %q not found in config", name, email)
}

func (c *configCrudContext) theConfigShouldNotContainCC(name string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	for _, cc := range c.config.Email.DefaultCC {
		if cc.Name == name {
			return fmt.Errorf("cc with name %q should not exist in config", name)
		}
	}
	return nil
}

// --- Common assertions ---

func (c *configCrudContext) theCommandShouldSucceed() error {
	if c.err != nil {
		return fmt.Errorf("expected command to succeed but got error: %v\nOutput: %s", c.err, c.output.String())
	}
	return nil
}

func (c *configCrudContext) theCommandShouldFailWith(expectedError string) error {
	if c.err == nil {
		return fmt.Errorf("expected command to fail with %q but it succeeded\nOutput: %s", expectedError, c.output.String())
	}
	errStr := strings.ToLower(c.err.Error())
	expected := strings.ToLower(expectedError)
	if !strings.Contains(errStr, expected) {
		return fmt.Errorf("expected error to contain %q but got %q", expectedError, c.err.Error())
	}
	return nil
}

func (c *configCrudContext) theOutputShouldContain(expected string) error {
	output := c.output.String()
	if !strings.Contains(output, expected) {
		return fmt.Errorf("expected output to contain %q but got:\n%s", expected, output)
	}
	return nil
}

// --- Sender steps ---

func (c *configCrudContext) iRunConfigAddSender(key, name string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigAddWithDependencies(c.config, c.configPath, "sender", key, name, "", c.output)
	return nil
}

func (c *configCrudContext) senderExistsWithName(key, name string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	if c.config.Senders.Senders == nil {
		c.config.Senders.Senders = make(map[string]config.SenderConfig)
	}
	c.config.Senders.Senders[strings.ToLower(key)] = config.SenderConfig{Name: name}
	return c.saveConfig()
}

func (c *configCrudContext) iRunConfigListSenders() error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigListWithDependencies(c.config, c.configPath, "senders", c.output)
	return nil
}

func (c *configCrudContext) iRunConfigRemoveSender(key string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigRemoveWithDependencies(c.config, c.configPath, "sender", key, c.output)
	return nil
}

func (c *configCrudContext) iRunConfigUpdateSender(key, name string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	c.output.Reset()
	c.err = cmd.RunConfigUpdateWithDependencies(c.config, c.configPath, "sender", key, name, "", c.output)
	return nil
}

func (c *configCrudContext) theConfigShouldContainSender(key, name string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	key = strings.ToLower(key)
	s, exists := c.config.Senders.Senders[key]
	if !exists {
		return fmt.Errorf("sender %q not found in config", key)
	}
	if s.Name != name {
		return fmt.Errorf("expected sender %q to have name %q, got %q", key, name, s.Name)
	}
	return nil
}

func (c *configCrudContext) theConfigShouldNotContainSender(key string) error {
	if err := c.loadConfig(); err != nil {
		return err
	}
	key = strings.ToLower(key)
	if _, exists := c.config.Senders.Senders[key]; exists {
		return fmt.Errorf("sender %q should not exist in config", key)
	}
	return nil
}

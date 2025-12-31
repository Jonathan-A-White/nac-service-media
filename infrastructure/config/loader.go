package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration
type Config struct {
	Paths  PathsConfig  `yaml:"paths"`
	Audio  AudioConfig  `yaml:"audio"`
	Google GoogleConfig `yaml:"google"`
	Email  EmailConfig  `yaml:"email"`
}

// PathsConfig contains directory paths for media processing
type PathsConfig struct {
	SourceDirectory  string `yaml:"source_directory"`
	TrimmedDirectory string `yaml:"trimmed_directory"`
	AudioDirectory   string `yaml:"audio_directory"`
}

// AudioConfig contains audio extraction settings
type AudioConfig struct {
	Bitrate string `yaml:"bitrate"`
}

// GoogleConfig contains Google API settings
type GoogleConfig struct {
	CredentialsFile  string `yaml:"credentials_file"`
	ServicesFolderID string `yaml:"services_folder_id"`
}

// EmailConfig contains email notification settings
type EmailConfig struct {
	FromName    string                     `yaml:"from_name"`
	FromAddress string                     `yaml:"from_address"`
	DefaultCC   []RecipientConfig          `yaml:"default_cc"`
	Recipients  map[string]RecipientConfig `yaml:"recipients"`
}

// RecipientConfig represents an email recipient
type RecipientConfig struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
}

// Load reads and parses the configuration from the specified YAML file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

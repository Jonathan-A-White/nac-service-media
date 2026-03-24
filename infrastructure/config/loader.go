package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration
type Config struct {
	Paths     PathsConfig               `yaml:"paths"`
	Audio     AudioConfig               `yaml:"audio"`
	Google    GoogleConfig              `yaml:"google"`
	Email     EmailConfig               `yaml:"email"`
	Ministers map[string]MinisterConfig `yaml:"ministers,omitempty"`
	Senders   SendersConfig             `yaml:"senders,omitempty"`
	Detection DetectionConfig           `yaml:"detection,omitempty"`
}

// DetectionConfig contains settings for automatic timestamp detection
type DetectionConfig struct {
	Enabled           bool                      `yaml:"enabled"`
	TemplatesDir      string                    `yaml:"templates_dir"`
	AudioTemplatesDir string                    `yaml:"audio_templates_dir"`
	Thresholds        DetectionThresholdsConfig `yaml:"thresholds"`
	SearchRange       SearchRangeConfig         `yaml:"search_range"`
}

// DetectionThresholdsConfig contains detection threshold settings
type DetectionThresholdsConfig struct {
	MatchScore        float64 `yaml:"match_score"`
	CoarseStepSeconds int     `yaml:"coarse_step_seconds"`
	AmenMatchScore    float64 `yaml:"amen_match_score"`
}

// SearchRangeConfig contains the video time range to search for cross lighting
type SearchRangeConfig struct {
	StartMinutes              int `yaml:"start_minutes"`
	EndMinutes                int `yaml:"end_minutes"`
	AmenStartOffsetMinutes    int `yaml:"amen_start_offset_minutes"`
	AmenSearchDurationMinutes int `yaml:"amen_search_duration_minutes"`
}

// SendersConfig contains sender configuration with default sender
type SendersConfig struct {
	DefaultSender string                  `yaml:"default_sender"`
	Senders       map[string]SenderConfig `yaml:"senders,omitempty"`
}

// SenderConfig represents a sender's information
type SenderConfig struct {
	Name string `yaml:"name"`
}

// MinisterConfig represents a minister's information
type MinisterConfig struct {
	Name string `yaml:"name"`
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
	TokenFile        string `yaml:"token_file"`
	GmailTokenFile   string `yaml:"gmail_token_file"`
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

// Load reads and parses the configuration from the specified YAML file.
// Relative paths for Google credentials and token files are converted to
// absolute paths (relative to CWD at load time), so tokens are always
// saved and loaded from the same location.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Convert relative paths to absolute so tokens are always found
	cfg.Google.CredentialsFile = toAbsPath(cfg.Google.CredentialsFile)
	cfg.Google.TokenFile = toAbsPath(cfg.Google.TokenFile)
	if cfg.Google.GmailTokenFile == "" {
		cfg.Google.GmailTokenFile = toAbsPath("gmail_token.json")
	} else {
		cfg.Google.GmailTokenFile = toAbsPath(cfg.Google.GmailTokenFile)
	}

	return &cfg, nil
}

// toAbsPath converts a relative path to absolute using the current working directory.
// Already-absolute paths are returned unchanged. Empty paths are returned as-is.
func toAbsPath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}

// Save writes the configuration to the specified YAML file
func Save(cfg *Config, path string) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

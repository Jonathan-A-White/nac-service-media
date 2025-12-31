package config

import (
	"errors"
	"fmt"
	"strings"
)

// Errors for config management
var (
	ErrMinisterNotFound  = errors.New("minister not found")
	ErrRecipientNotFound = errors.New("recipient not found")
	ErrCCNotFound        = errors.New("cc not found")
	ErrSenderNotFound    = errors.New("sender not found")
	ErrDuplicateKey      = errors.New("key already exists")
	ErrInvalidEmail      = errors.New("invalid email format")
)

// ConfigManager provides CRUD operations for config entries
type ConfigManager struct {
	config     *Config
	configPath string
}

// NewConfigManager creates a new config manager
func NewConfigManager(cfg *Config, configPath string) *ConfigManager {
	return &ConfigManager{
		config:     cfg,
		configPath: configPath,
	}
}

// Minister represents a minister entry
type Minister struct {
	Key  string
	Name string
}

// Recipient represents a recipient entry (used for both recipients and CCs)
type Recipient struct {
	Key     string
	Name    string
	Address string
}

// Sender represents a sender entry
type Sender struct {
	Key  string
	Name string
}

// --- Minister CRUD ---

// AddMinister adds a new minister to config
func (m *ConfigManager) AddMinister(key, name string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	name = strings.TrimSpace(name)

	if key == "" {
		return fmt.Errorf("minister key is required")
	}
	if name == "" {
		return fmt.Errorf("minister name is required")
	}

	if m.config.Ministers == nil {
		m.config.Ministers = make(map[string]MinisterConfig)
	}

	if _, exists := m.config.Ministers[key]; exists {
		return fmt.Errorf("%w: minister %q", ErrDuplicateKey, key)
	}

	m.config.Ministers[key] = MinisterConfig{Name: name}
	return Save(m.config, m.configPath)
}

// ListMinisters returns all ministers
func (m *ConfigManager) ListMinisters() []Minister {
	result := make([]Minister, 0, len(m.config.Ministers))
	for key, mc := range m.config.Ministers {
		result = append(result, Minister{
			Key:  key,
			Name: mc.Name,
		})
	}
	return result
}

// GetMinister gets a minister by key (case-insensitive)
func (m *ConfigManager) GetMinister(key string) (Minister, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if mc, exists := m.config.Ministers[key]; exists {
		return Minister{Key: key, Name: mc.Name}, nil
	}
	return Minister{}, fmt.Errorf("%w: %q", ErrMinisterNotFound, key)
}

// RemoveMinister removes a minister by key
func (m *ConfigManager) RemoveMinister(key string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if _, exists := m.config.Ministers[key]; !exists {
		return fmt.Errorf("%w: %q", ErrMinisterNotFound, key)
	}

	delete(m.config.Ministers, key)
	return Save(m.config, m.configPath)
}

// UpdateMinister updates a minister's name
func (m *ConfigManager) UpdateMinister(key, name string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	name = strings.TrimSpace(name)

	if _, exists := m.config.Ministers[key]; !exists {
		return fmt.Errorf("%w: %q", ErrMinisterNotFound, key)
	}

	if name == "" {
		return fmt.Errorf("minister name is required")
	}

	m.config.Ministers[key] = MinisterConfig{Name: name}
	return Save(m.config, m.configPath)
}

// --- Recipient CRUD ---

// AddRecipient adds a new recipient to config
func (m *ConfigManager) AddRecipient(key, name, email string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)

	if key == "" {
		return fmt.Errorf("recipient key is required")
	}
	if name == "" {
		return fmt.Errorf("recipient name is required")
	}
	if !isValidEmail(email) {
		return fmt.Errorf("%w: %q", ErrInvalidEmail, email)
	}

	if m.config.Email.Recipients == nil {
		m.config.Email.Recipients = make(map[string]RecipientConfig)
	}

	if _, exists := m.config.Email.Recipients[key]; exists {
		return fmt.Errorf("%w: recipient %q", ErrDuplicateKey, key)
	}

	m.config.Email.Recipients[key] = RecipientConfig{Name: name, Address: email}
	return Save(m.config, m.configPath)
}

// ListRecipients returns all recipients
func (m *ConfigManager) ListRecipients() []Recipient {
	result := make([]Recipient, 0, len(m.config.Email.Recipients))
	for key, rc := range m.config.Email.Recipients {
		result = append(result, Recipient{
			Key:     key,
			Name:    rc.Name,
			Address: rc.Address,
		})
	}
	return result
}

// GetRecipient gets a recipient by key (case-insensitive)
func (m *ConfigManager) GetRecipient(key string) (Recipient, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if rc, exists := m.config.Email.Recipients[key]; exists {
		return Recipient{Key: key, Name: rc.Name, Address: rc.Address}, nil
	}
	return Recipient{}, fmt.Errorf("%w: %q", ErrRecipientNotFound, key)
}

// RemoveRecipient removes a recipient by key
func (m *ConfigManager) RemoveRecipient(key string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if _, exists := m.config.Email.Recipients[key]; !exists {
		return fmt.Errorf("%w: %q", ErrRecipientNotFound, key)
	}

	delete(m.config.Email.Recipients, key)
	return Save(m.config, m.configPath)
}

// UpdateRecipient updates a recipient's name and/or email
func (m *ConfigManager) UpdateRecipient(key, name, email string) error {
	key = strings.ToLower(strings.TrimSpace(key))

	rc, exists := m.config.Email.Recipients[key]
	if !exists {
		return fmt.Errorf("%w: %q", ErrRecipientNotFound, key)
	}

	// Update only provided values
	if name = strings.TrimSpace(name); name != "" {
		rc.Name = name
	}
	if email = strings.TrimSpace(email); email != "" {
		if !isValidEmail(email) {
			return fmt.Errorf("%w: %q", ErrInvalidEmail, email)
		}
		rc.Address = email
	}

	m.config.Email.Recipients[key] = rc
	return Save(m.config, m.configPath)
}

// --- CC CRUD ---

// AddCC adds a new default CC recipient
func (m *ConfigManager) AddCC(key, name, email string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)

	if key == "" {
		return fmt.Errorf("cc key is required")
	}
	if name == "" {
		return fmt.Errorf("cc name is required")
	}
	if !isValidEmail(email) {
		return fmt.Errorf("%w: %q", ErrInvalidEmail, email)
	}

	// Check for duplicate key - we use a slice, so iterate
	for _, cc := range m.config.Email.DefaultCC {
		// Use name as a pseudo-key if it matches the key
		if strings.ToLower(cc.Name) == key || strings.ToLower(strings.Split(cc.Name, " ")[0]) == key {
			return fmt.Errorf("%w: cc %q", ErrDuplicateKey, key)
		}
	}

	// Store key in a way we can find it - prepend to name or use separate storage
	// For simplicity, we'll store the key as part of the lookup by converting default_cc to a map structure
	// But to maintain backwards compatibility with the existing array structure, we'll use a keyed approach
	// by making the DefaultCC a map-like structure in the yaml while keeping the RecipientConfig structure

	m.config.Email.DefaultCC = append(m.config.Email.DefaultCC, RecipientConfig{
		Name:    name,
		Address: email,
	})

	return Save(m.config, m.configPath)
}

// ListCCs returns all default CC recipients
func (m *ConfigManager) ListCCs() []Recipient {
	result := make([]Recipient, 0, len(m.config.Email.DefaultCC))
	for i, cc := range m.config.Email.DefaultCC {
		// Generate key from index or first name
		key := strings.ToLower(strings.Split(cc.Name, " ")[0])
		if key == "" {
			key = fmt.Sprintf("cc%d", i)
		}
		result = append(result, Recipient{
			Key:     key,
			Name:    cc.Name,
			Address: cc.Address,
		})
	}
	return result
}

// GetCC gets a CC by key (matches on name/first name, case-insensitive)
func (m *ConfigManager) GetCC(key string) (Recipient, int, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	for i, cc := range m.config.Email.DefaultCC {
		nameLower := strings.ToLower(cc.Name)
		firstName := strings.ToLower(strings.Split(cc.Name, " ")[0])
		if firstName == key || nameLower == key {
			return Recipient{Key: firstName, Name: cc.Name, Address: cc.Address}, i, nil
		}
	}
	return Recipient{}, -1, fmt.Errorf("%w: %q", ErrCCNotFound, key)
}

// RemoveCC removes a CC by key
func (m *ConfigManager) RemoveCC(key string) error {
	_, idx, err := m.GetCC(key)
	if err != nil {
		return err
	}

	m.config.Email.DefaultCC = append(
		m.config.Email.DefaultCC[:idx],
		m.config.Email.DefaultCC[idx+1:]...,
	)
	return Save(m.config, m.configPath)
}

// UpdateCC updates a CC's name and/or email
func (m *ConfigManager) UpdateCC(key, name, email string) error {
	_, idx, err := m.GetCC(key)
	if err != nil {
		return err
	}

	cc := m.config.Email.DefaultCC[idx]

	// Update only provided values
	if name = strings.TrimSpace(name); name != "" {
		cc.Name = name
	}
	if email = strings.TrimSpace(email); email != "" {
		if !isValidEmail(email) {
			return fmt.Errorf("%w: %q", ErrInvalidEmail, email)
		}
		cc.Address = email
	}

	m.config.Email.DefaultCC[idx] = cc
	return Save(m.config, m.configPath)
}

// isValidEmail performs basic email validation
func isValidEmail(email string) bool {
	if email == "" {
		return false
	}
	// Basic check: contains @ and at least one . after @
	atIdx := strings.Index(email, "@")
	if atIdx < 1 {
		return false
	}
	domain := email[atIdx+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	return true
}

// SuggestAddCommand returns the command to add a missing entry
func SuggestAddMinisterCommand(key string) string {
	return fmt.Sprintf(`nac-service-media config add minister --key %s --name "Minister Name"`, key)
}

func SuggestAddRecipientCommand(key string) string {
	return fmt.Sprintf(`nac-service-media config add recipient --key %s --name "Recipient Name" --email "email@example.com"`, key)
}

func SuggestAddCCCommand(key string) string {
	return fmt.Sprintf(`nac-service-media config add cc --key %s --name "CC Name" --email "email@example.com"`, key)
}

func SuggestAddSenderCommand(key string) string {
	return fmt.Sprintf(`nac-service-media config add sender --key %s --name "Sender Name"`, key)
}

// --- Sender CRUD ---

// AddSender adds a new sender to config
func (m *ConfigManager) AddSender(key, name string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	name = strings.TrimSpace(name)

	if key == "" {
		return fmt.Errorf("sender key is required")
	}
	if name == "" {
		return fmt.Errorf("sender name is required")
	}

	if m.config.Senders.Senders == nil {
		m.config.Senders.Senders = make(map[string]SenderConfig)
	}

	if _, exists := m.config.Senders.Senders[key]; exists {
		return fmt.Errorf("%w: sender %q", ErrDuplicateKey, key)
	}

	m.config.Senders.Senders[key] = SenderConfig{Name: name}
	return Save(m.config, m.configPath)
}

// ListSenders returns all senders
func (m *ConfigManager) ListSenders() []Sender {
	result := make([]Sender, 0, len(m.config.Senders.Senders))
	for key, sc := range m.config.Senders.Senders {
		result = append(result, Sender{
			Key:  key,
			Name: sc.Name,
		})
	}
	return result
}

// GetSender gets a sender by key (case-insensitive)
func (m *ConfigManager) GetSender(key string) (Sender, error) {
	key = strings.ToLower(strings.TrimSpace(key))
	if sc, exists := m.config.Senders.Senders[key]; exists {
		return Sender{Key: key, Name: sc.Name}, nil
	}
	return Sender{}, fmt.Errorf("%w: %q", ErrSenderNotFound, key)
}

// GetDefaultSender gets the default sender
func (m *ConfigManager) GetDefaultSender() (Sender, error) {
	if m.config.Senders.DefaultSender == "" {
		return Sender{}, fmt.Errorf("no default sender configured")
	}
	return m.GetSender(m.config.Senders.DefaultSender)
}

// RemoveSender removes a sender by key
func (m *ConfigManager) RemoveSender(key string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	if _, exists := m.config.Senders.Senders[key]; !exists {
		return fmt.Errorf("%w: %q", ErrSenderNotFound, key)
	}

	delete(m.config.Senders.Senders, key)
	return Save(m.config, m.configPath)
}

// UpdateSender updates a sender's name
func (m *ConfigManager) UpdateSender(key, name string) error {
	key = strings.ToLower(strings.TrimSpace(key))
	name = strings.TrimSpace(name)

	if _, exists := m.config.Senders.Senders[key]; !exists {
		return fmt.Errorf("%w: %q", ErrSenderNotFound, key)
	}

	if name == "" {
		return fmt.Errorf("sender name is required")
	}

	m.config.Senders.Senders[key] = SenderConfig{Name: name}
	return Save(m.config, m.configPath)
}

// SetDefaultSender sets the default sender key
func (m *ConfigManager) SetDefaultSender(key string) error {
	key = strings.ToLower(strings.TrimSpace(key))

	// Verify sender exists
	if _, exists := m.config.Senders.Senders[key]; !exists {
		return fmt.Errorf("%w: %q", ErrSenderNotFound, key)
	}

	m.config.Senders.DefaultSender = key
	return Save(m.config, m.configPath)
}

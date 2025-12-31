package config

import (
	"fmt"
	"strings"

	"nac-service-media/domain/notification"
)

// RecipientLookup provides methods to find and manage recipients
type RecipientLookup struct {
	config     *Config
	configPath string
}

// NewRecipientLookup creates a new recipient lookup from config
func NewRecipientLookup(cfg *Config, configPath string) *RecipientLookup {
	return &RecipientLookup{
		config:     cfg,
		configPath: configPath,
	}
}

// LookupRecipient finds recipients matching the query (first name, last name, full name, or key)
// Returns all matches - caller should handle ambiguity
func (r *RecipientLookup) LookupRecipient(query string) ([]notification.Recipient, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil, notification.ErrRecipientNotFound
	}

	var matches []notification.Recipient

	for key, rc := range r.config.Email.Recipients {
		keyLower := strings.ToLower(key)
		nameLower := strings.ToLower(rc.Name)
		nameParts := strings.Fields(nameLower)

		var firstName, lastName string
		if len(nameParts) > 0 {
			firstName = nameParts[0]
		}
		if len(nameParts) > 1 {
			lastName = nameParts[len(nameParts)-1]
		}

		// Match on: key, first name, last name, or full name
		if keyLower == query || firstName == query || lastName == query || nameLower == query {
			matches = append(matches, notification.Recipient{
				Name:    rc.Name,
				Address: rc.Address,
			})
		}
	}

	if len(matches) == 0 {
		return nil, notification.ErrRecipientNotFound
	}

	return matches, nil
}

// LookupRecipients looks up multiple recipients by query strings
// Supports comma-separated or multiple queries
func (r *RecipientLookup) LookupRecipients(queries []string) ([]notification.Recipient, error) {
	var allRecipients []notification.Recipient
	seen := make(map[string]bool) // Deduplicate by email

	for _, q := range queries {
		// Handle comma-separated values
		for _, query := range strings.Split(q, ",") {
			query = strings.TrimSpace(query)
			if query == "" {
				continue
			}

			matches, err := r.LookupRecipient(query)
			if err != nil {
				return nil, fmt.Errorf("recipient %q: %w", query, err)
			}

			if len(matches) > 1 {
				names := make([]string, len(matches))
				for i, m := range matches {
					names[i] = m.Name
				}
				return nil, fmt.Errorf("%w: %q matches %s - use last name to disambiguate",
					notification.ErrAmbiguousRecipient, query, strings.Join(names, ", "))
			}

			// Add if not already seen
			if !seen[matches[0].Address] {
				seen[matches[0].Address] = true
				allRecipients = append(allRecipients, matches[0])
			}
		}
	}

	if len(allRecipients) == 0 {
		return nil, notification.ErrRecipientNotFound
	}

	return allRecipients, nil
}

// GetDefaultCC returns the configured default CC recipients
func (r *RecipientLookup) GetDefaultCC() []notification.Recipient {
	cc := make([]notification.Recipient, len(r.config.Email.DefaultCC))
	for i, rc := range r.config.Email.DefaultCC {
		cc[i] = notification.Recipient{
			Name:    rc.Name,
			Address: rc.Address,
		}
	}
	return cc
}

// AddRecipient adds a new recipient to the config and saves it
func (r *RecipientLookup) AddRecipient(key, name, address string) error {
	if r.config.Email.Recipients == nil {
		r.config.Email.Recipients = make(map[string]RecipientConfig)
	}

	r.config.Email.Recipients[key] = RecipientConfig{
		Name:    name,
		Address: address,
	}

	return Save(r.config, r.configPath)
}

// ListRecipients returns all configured recipients
func (r *RecipientLookup) ListRecipients() []notification.Recipient {
	result := make([]notification.Recipient, 0, len(r.config.Email.Recipients))
	for _, rc := range r.config.Email.Recipients {
		result = append(result, notification.Recipient{
			Name:    rc.Name,
			Address: rc.Address,
		})
	}
	return result
}

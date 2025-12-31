package config

import (
	"errors"
	"testing"

	"nac-service-media/domain/notification"
)

func TestRecipientLookup_LookupRecipient(t *testing.T) {
	cfg := &Config{
		Email: EmailConfig{
			Recipients: map[string]RecipientConfig{
				"jonathan": {Name: "Jonathan White", Address: "jonathan@example.com"},
				"jane":     {Name: "Jane Doe", Address: "jane@example.com"},
				"john":     {Name: "John Smith", Address: "john@example.com"},
			},
		},
	}
	lookup := NewRecipientLookup(cfg, "")

	tests := []struct {
		name      string
		query     string
		wantName  string
		wantAddr  string
		wantErr   error
		wantCount int
	}{
		{
			name:      "lookup by key",
			query:     "jonathan",
			wantName:  "Jonathan White",
			wantAddr:  "jonathan@example.com",
			wantCount: 1,
		},
		{
			name:      "lookup by first name",
			query:     "Jane",
			wantName:  "Jane Doe",
			wantAddr:  "jane@example.com",
			wantCount: 1,
		},
		{
			name:      "lookup by last name",
			query:     "Smith",
			wantName:  "John Smith",
			wantAddr:  "john@example.com",
			wantCount: 1,
		},
		{
			name:      "lookup by full name",
			query:     "Jonathan White",
			wantName:  "Jonathan White",
			wantAddr:  "jonathan@example.com",
			wantCount: 1,
		},
		{
			name:      "case insensitive",
			query:     "JANE",
			wantName:  "Jane Doe",
			wantAddr:  "jane@example.com",
			wantCount: 1,
		},
		{
			name:    "not found",
			query:   "unknown",
			wantErr: notification.ErrRecipientNotFound,
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: notification.ErrRecipientNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := lookup.LookupRecipient(tt.query)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("LookupRecipient() error = %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("LookupRecipient() error = %v", err)
			}

			if len(matches) != tt.wantCount {
				t.Errorf("LookupRecipient() got %d matches, want %d", len(matches), tt.wantCount)
			}

			if tt.wantCount > 0 {
				if matches[0].Name != tt.wantName {
					t.Errorf("LookupRecipient() name = %q, want %q", matches[0].Name, tt.wantName)
				}
				if matches[0].Address != tt.wantAddr {
					t.Errorf("LookupRecipient() address = %q, want %q", matches[0].Address, tt.wantAddr)
				}
			}
		})
	}
}

func TestRecipientLookup_LookupRecipients_Multiple(t *testing.T) {
	cfg := &Config{
		Email: EmailConfig{
			Recipients: map[string]RecipientConfig{
				"jonathan": {Name: "Jonathan White", Address: "jonathan@example.com"},
				"jane":     {Name: "Jane Doe", Address: "jane@example.com"},
			},
		},
	}
	lookup := NewRecipientLookup(cfg, "")

	// Test multiple queries
	recipients, err := lookup.LookupRecipients([]string{"jonathan", "jane"})
	if err != nil {
		t.Fatalf("LookupRecipients() error = %v", err)
	}
	if len(recipients) != 2 {
		t.Errorf("LookupRecipients() got %d recipients, want 2", len(recipients))
	}

	// Test comma-separated
	recipients, err = lookup.LookupRecipients([]string{"jonathan, jane"})
	if err != nil {
		t.Fatalf("LookupRecipients() error = %v", err)
	}
	if len(recipients) != 2 {
		t.Errorf("LookupRecipients() got %d recipients, want 2", len(recipients))
	}

	// Test deduplication
	recipients, err = lookup.LookupRecipients([]string{"jonathan", "jonathan"})
	if err != nil {
		t.Fatalf("LookupRecipients() error = %v", err)
	}
	if len(recipients) != 1 {
		t.Errorf("LookupRecipients() should deduplicate, got %d", len(recipients))
	}
}

func TestRecipientLookup_AmbiguousRecipient(t *testing.T) {
	cfg := &Config{
		Email: EmailConfig{
			Recipients: map[string]RecipientConfig{
				"jane1": {Name: "Jane Doe", Address: "jane1@example.com"},
				"jane2": {Name: "Jane Smith", Address: "jane2@example.com"},
			},
		},
	}
	lookup := NewRecipientLookup(cfg, "")

	// Single lookup returns all matches
	matches, err := lookup.LookupRecipient("jane")
	if err != nil {
		t.Fatalf("LookupRecipient() error = %v", err)
	}
	if len(matches) != 2 {
		t.Errorf("LookupRecipient() should return 2 matches for ambiguous query, got %d", len(matches))
	}

	// LookupRecipients returns error for ambiguous
	_, err = lookup.LookupRecipients([]string{"jane"})
	if err == nil {
		t.Error("LookupRecipients() should return error for ambiguous query")
	}
	if !errors.Is(err, notification.ErrAmbiguousRecipient) {
		t.Errorf("LookupRecipients() error = %v, want ErrAmbiguousRecipient", err)
	}

	// Can disambiguate by last name
	recipients, err := lookup.LookupRecipients([]string{"smith"})
	if err != nil {
		t.Fatalf("LookupRecipients() error = %v", err)
	}
	if len(recipients) != 1 || recipients[0].Name != "Jane Smith" {
		t.Errorf("LookupRecipients() should disambiguate by last name")
	}
}

func TestRecipientLookup_GetDefaultCC(t *testing.T) {
	cfg := &Config{
		Email: EmailConfig{
			DefaultCC: []RecipientConfig{
				{Name: "Admin", Address: "admin@example.com"},
			},
		},
	}
	lookup := NewRecipientLookup(cfg, "")

	cc := lookup.GetDefaultCC()
	if len(cc) != 1 {
		t.Fatalf("GetDefaultCC() got %d, want 1", len(cc))
	}
	if cc[0].Name != "Admin" || cc[0].Address != "admin@example.com" {
		t.Errorf("GetDefaultCC() = %+v, unexpected", cc[0])
	}
}

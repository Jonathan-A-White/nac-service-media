package notification

import (
	"strings"
	"testing"
	"time"
)

func TestEmailTemplate_RenderSubject(t *testing.T) {
	data := TemplateData{
		ChurchName:    "White Plains",
		DateFormatted: "12/28/2025",
	}

	subject, err := DefaultTemplate.RenderSubject(data)
	if err != nil {
		t.Fatalf("RenderSubject() error = %v", err)
	}

	expected := "White Plains: Recording of Service on 12/28/2025"
	if subject != expected {
		t.Errorf("RenderSubject() = %q, want %q", subject, expected)
	}
}

func TestEmailTemplate_RenderPlainText(t *testing.T) {
	data := TemplateData{
		Greeting:     "Dear John,",
		MinisterName: "Pr. Smith",
		AudioURL:     "https://drive.google.com/file/d/abc/view",
		VideoURL:     "https://drive.google.com/file/d/xyz/view",
		SenderName:   "Jonathan",
	}

	body, err := DefaultTemplate.RenderPlainText(data)
	if err != nil {
		t.Fatalf("RenderPlainText() error = %v", err)
	}

	// Verify key content is present
	checks := []string{
		"Dear John,",
		"Pr. Smith",
		"https://drive.google.com/file/d/abc/view",
		"https://drive.google.com/file/d/xyz/view",
		"Thanks!\nJonathan",
	}

	for _, check := range checks {
		if !strings.Contains(body, check) {
			t.Errorf("RenderPlainText() missing %q in:\n%s", check, body)
		}
	}
}

func TestEmailTemplate_RenderPlainText_WithMinister(t *testing.T) {
	data := TemplateData{
		Greeting:     "Dear John,",
		ServiceRef:   "today's",
		MinisterName: "Pr. Smith",
		AudioURL:     "https://drive.google.com/file/d/abc/view",
		VideoURL:     "https://drive.google.com/file/d/xyz/view",
		SenderName:   "Jonathan",
	}

	body, err := DefaultTemplate.RenderPlainText(data)
	if err != nil {
		t.Fatalf("RenderPlainText() error = %v", err)
	}

	expected := "service with Pr. Smith."
	if !strings.Contains(body, expected) {
		t.Errorf("RenderPlainText() should contain %q when minister provided, got:\n%s", expected, body)
	}
}

func TestEmailTemplate_RenderPlainText_WithoutMinister(t *testing.T) {
	data := TemplateData{
		Greeting:     "Dear John,",
		ServiceRef:   "today's",
		MinisterName: "", // No minister
		AudioURL:     "https://drive.google.com/file/d/abc/view",
		VideoURL:     "https://drive.google.com/file/d/xyz/view",
		SenderName:   "Jonathan",
	}

	body, err := DefaultTemplate.RenderPlainText(data)
	if err != nil {
		t.Fatalf("RenderPlainText() error = %v", err)
	}

	// Should end with "service." not "service with ."
	if !strings.Contains(body, "today's service.") {
		t.Errorf("RenderPlainText() should contain \"today's service.\" when no minister, got:\n%s", body)
	}
	if strings.Contains(body, "service with") {
		t.Errorf("RenderPlainText() should not contain \"service with\" when no minister, got:\n%s", body)
	}
}

func TestEmailTemplate_RenderHTML(t *testing.T) {
	data := TemplateData{
		Greeting:     "Dear John,",
		MinisterName: "Pr. Smith",
		AudioURL:     "https://drive.google.com/file/d/abc/view",
		VideoURL:     "https://drive.google.com/file/d/xyz/view",
		SenderName:   "Jonathan",
	}

	body, err := DefaultTemplate.RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}

	// Verify HTML links are clickable
	if !strings.Contains(body, `<a href="https://drive.google.com/file/d/abc/view">audio</a>`) {
		t.Errorf("RenderHTML() missing audio link in:\n%s", body)
	}
	if !strings.Contains(body, `<a href="https://drive.google.com/file/d/xyz/view">video</a>`) {
		t.Errorf("RenderHTML() missing video link in:\n%s", body)
	}
}

func TestEmailTemplate_RenderHTML_WithMinister(t *testing.T) {
	data := TemplateData{
		Greeting:     "Dear John,",
		ServiceRef:   "today's",
		MinisterName: "Pr. Smith",
		AudioURL:     "https://drive.google.com/file/d/abc/view",
		VideoURL:     "https://drive.google.com/file/d/xyz/view",
		SenderName:   "Jonathan",
	}

	body, err := DefaultTemplate.RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}

	expected := "service with Pr. Smith."
	if !strings.Contains(body, expected) {
		t.Errorf("RenderHTML() should contain %q when minister provided, got:\n%s", expected, body)
	}
}

func TestEmailTemplate_RenderHTML_WithoutMinister(t *testing.T) {
	data := TemplateData{
		Greeting:     "Dear John,",
		ServiceRef:   "today's",
		MinisterName: "", // No minister
		AudioURL:     "https://drive.google.com/file/d/abc/view",
		VideoURL:     "https://drive.google.com/file/d/xyz/view",
		SenderName:   "Jonathan",
	}

	body, err := DefaultTemplate.RenderHTML(data)
	if err != nil {
		t.Fatalf("RenderHTML() error = %v", err)
	}

	// Should end with "service." not "service with ."
	if !strings.Contains(body, "today's service.") {
		t.Errorf("RenderHTML() should contain \"today's service.\" when no minister, got:\n%s", body)
	}
	if strings.Contains(body, "service with") {
		t.Errorf("RenderHTML() should not contain \"service with\" when no minister, got:\n%s", body)
	}
}

func TestFormatServiceRef(t *testing.T) {
	// Use a fixed "now" for testing
	sunday := time.Date(2025, 12, 28, 10, 0, 0, 0, time.Local) // Sunday service

	tests := []struct {
		name        string
		serviceDate time.Time
		now         time.Time
		want        string
	}{
		{
			name:        "same day (today)",
			serviceDate: sunday,
			now:         sunday,
			want:        "today's",
		},
		{
			name:        "next day (yesterday)",
			serviceDate: sunday,
			now:         sunday.AddDate(0, 0, 1), // Monday
			want:        "yesterday's",
		},
		{
			name:        "two days later",
			serviceDate: sunday,
			now:         sunday.AddDate(0, 0, 2), // Tuesday
			want:        "Sunday's",
		},
		{
			name:        "six days later (still Sunday's)",
			serviceDate: sunday,
			now:         sunday.AddDate(0, 0, 6),
			want:        "Sunday's",
		},
		{
			name:        "week later (explicit date)",
			serviceDate: sunday,
			now:         sunday.AddDate(0, 0, 7),
			want:        "the 12/28",
		},
		{
			name:        "two weeks later",
			serviceDate: sunday,
			now:         sunday.AddDate(0, 0, 14),
			want:        "the 12/28",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatServiceRef(tt.serviceDate, tt.now)
			if got != tt.want {
				t.Errorf("FormatServiceRef() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatGreeting(t *testing.T) {
	tests := []struct {
		name       string
		recipients []Recipient
		want       string
	}{
		{
			name:       "no recipients",
			recipients: nil,
			want:       "Hello,",
		},
		{
			name:       "one recipient",
			recipients: []Recipient{{Name: "John Doe", Address: "john@example.com"}},
			want:       "Dear John,",
		},
		{
			name:       "two recipients",
			recipients: []Recipient{{Name: "John Doe"}, {Name: "Jane Smith"}},
			want:       "Dear John & Jane,",
		},
		{
			name:       "three recipients",
			recipients: []Recipient{{Name: "John"}, {Name: "Jane"}, {Name: "Alice"}},
			want:       "Hey Everyone!",
		},
		{
			name:       "recipient with no name",
			recipients: []Recipient{{Address: "john@example.com"}},
			want:       "Dear Friend,",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatGreeting(tt.recipients)
			if got != tt.want {
				t.Errorf("FormatGreeting() = %q, want %q", got, tt.want)
			}
		})
	}
}

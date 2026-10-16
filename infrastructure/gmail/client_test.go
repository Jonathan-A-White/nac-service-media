package gmail

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"nac-service-media/domain/notification"

	"google.golang.org/api/gmail/v1"
)

// mockGmailService is a mock implementation for testing
type mockGmailService struct {
	sentMessages []*gmail.Message
	shouldFail   bool
	failError    error
}

func (m *mockGmailService) SendMessage(ctx context.Context, userID string, message *gmail.Message) (*gmail.Message, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	m.sentMessages = append(m.sentMessages, message)
	return &gmail.Message{Id: "test-message-id"}, nil
}

func TestClient_Send(t *testing.T) {
	mock := &mockGmailService{}
	from := notification.Recipient{Name: "Jonathan White", Address: "whiteplainsnac@gmail.com"}

	client := NewClient(from, WithGmailService(mock))

	req := &notification.EmailRequest{
		To:           []notification.Recipient{{Name: "John Doe", Address: "john@example.com"}},
		CC:           []notification.Recipient{{Name: "Jane Doe", Address: "jane@example.com"}},
		ServiceDate:  time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
		MinisterName: "Pr. Smith",
		AudioURL:     "https://drive.google.com/file/d/abc/view",
		VideoURL:     "https://drive.google.com/file/d/xyz/view",
		ChurchName:   "White Plains",
		SenderName:   "Jonathan",
	}

	err := client.Send(req)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if len(mock.sentMessages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(mock.sentMessages))
	}

	// Decode the raw message to verify content
	// The message is base64 URL encoded
	rawBytes, err := decodeBase64URL(mock.sentMessages[0].Raw)
	if err != nil {
		t.Fatalf("failed to decode message: %v", err)
	}
	raw := string(rawBytes)

	// Verify headers
	checks := []string{
		"From: Jonathan White <whiteplainsnac@gmail.com>",
		"To: John Doe <john@example.com>",
		"Cc: Jane Doe <jane@example.com>",
		"Subject: White Plains: Recording of Service on 12/28/2025",
		"Dear John,", // Single recipient greeting
		"Pr. Smith",
		"https://drive.google.com/file/d/abc/view",
		"https://drive.google.com/file/d/xyz/view",
		"Thanks!\nJonathan",
	}

	for _, check := range checks {
		if !strings.Contains(raw, check) {
			t.Errorf("message missing %q in:\n%s", check, raw)
		}
	}
}

func TestClient_Send_MultipleRecipients(t *testing.T) {
	mock := &mockGmailService{}
	from := notification.Recipient{Name: "Jonathan White", Address: "whiteplainsnac@gmail.com"}

	client := NewClient(from, WithGmailService(mock))

	req := &notification.EmailRequest{
		To: []notification.Recipient{
			{Name: "John Doe", Address: "john@example.com"},
			{Name: "Alice Smith", Address: "alice@example.com"},
		},
		ServiceDate:  time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
		MinisterName: "Pr. Henkel",
		AudioURL:     "https://drive.google.com/file/d/abc/view",
		VideoURL:     "https://drive.google.com/file/d/xyz/view",
		ChurchName:   "White Plains",
		SenderName:   "Jonathan",
	}

	err := client.Send(req)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	rawBytes, _ := decodeBase64URL(mock.sentMessages[0].Raw)
	raw := string(rawBytes)

	// Verify multiple recipients in To header
	if !strings.Contains(raw, "To: John Doe <john@example.com>, Alice Smith <alice@example.com>") {
		t.Errorf("message missing multiple recipients in To header:\n%s", raw)
	}

	// Greeting should use both recipients' names for two recipients
	if !strings.Contains(raw, "Dear John & Alice,") {
		t.Errorf("message should greet both recipients by name:\n%s", raw)
	}
}

func TestClient_Send_ValidationError(t *testing.T) {
	mock := &mockGmailService{}
	from := notification.Recipient{Name: "Jonathan White", Address: "whiteplainsnac@gmail.com"}

	client := NewClient(from, WithGmailService(mock))

	req := &notification.EmailRequest{
		// Missing To recipients
		ServiceDate:  time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
		MinisterName: "Pr. Smith",
		AudioURL:     "https://drive.google.com/file/d/abc/view",
	}

	err := client.Send(req)
	if err == nil {
		t.Fatal("Send() expected error for invalid request, got nil")
	}
	if !strings.Contains(err.Error(), "invalid email request") {
		t.Errorf("Send() error = %v, want invalid email request error", err)
	}
}

// decodeBase64URL decodes a base64 URL encoded string
func decodeBase64URL(s string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(s)
}

func TestClient_SendPlain(t *testing.T) {
	mock := &mockGmailService{}
	from := notification.Recipient{Name: "Jonathan White", Address: "whiteplainsnac@gmail.com"}
	client := NewClient(from, WithGmailService(mock))

	to := []notification.Recipient{{Name: "Jane Doe", Address: "jane@example.com"}}
	err := client.SendPlain(to, "Service recording NOT processed: 2026-10-11: cross not found", "What failed: cross not found\nVideo file: x.mp4")
	if err != nil {
		t.Fatalf("SendPlain() error = %v", err)
	}
	if len(mock.sentMessages) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(mock.sentMessages))
	}
	raw, err := decodeBase64URL(mock.sentMessages[0].Raw)
	if err != nil {
		t.Fatalf("failed to decode message: %v", err)
	}
	msg := string(raw)
	for _, want := range []string{
		"From: Jonathan White <whiteplainsnac@gmail.com>\r\n",
		"To: Jane Doe <jane@example.com>\r\n",
		"Subject: Service recording NOT processed: 2026-10-11: cross not found\r\n",
		"Content-Type: text/plain; charset=\"UTF-8\"",
		"What failed: cross not found\r\nVideo file: x.mp4",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q\n%s", want, msg)
		}
	}
}

func TestClient_SendPlain_NoRecipients(t *testing.T) {
	mock := &mockGmailService{}
	client := NewClient(notification.Recipient{Address: "a@example.com"}, WithGmailService(mock))
	if err := client.SendPlain(nil, "s", "b"); err != notification.ErrNoRecipients {
		t.Errorf("expected ErrNoRecipients, got %v", err)
	}
	if len(mock.sentMessages) != 0 {
		t.Error("nothing should be sent without recipients")
	}
}

func TestClient_SendPlain_SendFails(t *testing.T) {
	mock := &mockGmailService{shouldFail: true, failError: errors.New("token expired")}
	client := NewClient(notification.Recipient{Address: "a@example.com"}, WithGmailService(mock))
	err := client.SendPlain([]notification.Recipient{{Address: "j@example.com"}}, "s", "b")
	if !errors.Is(err, notification.ErrSendFailed) {
		t.Errorf("expected ErrSendFailed, got %v", err)
	}
}

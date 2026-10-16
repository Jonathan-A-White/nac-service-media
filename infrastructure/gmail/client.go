package gmail

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"strings"
	"time"

	"nac-service-media/domain/notification"

	"google.golang.org/api/gmail/v1"
)

// GmailService defines the interface for Gmail API operations
// This allows mocking the Gmail API in tests
type GmailService interface {
	SendMessage(ctx context.Context, userID string, message *gmail.Message) (*gmail.Message, error)
}

// GoogleGmailService is the production implementation using the Gmail API
type GoogleGmailService struct {
	service *gmail.Service
}

// SendMessage sends an email via Gmail API
func (s *GoogleGmailService) SendMessage(ctx context.Context, userID string, message *gmail.Message) (*gmail.Message, error) {
	return s.service.Users.Messages.Send(userID, message).Context(ctx).Do()
}

// Client implements notification.EmailSender using Gmail API
type Client struct {
	gmailService GmailService
	from         notification.Recipient
	template     notification.EmailTemplate
	authTimeout  time.Duration // bounds the browser sign-in wait; zero waits for ctx
}

// ClientOption is a functional option for configuring Client
type ClientOption func(*Client)

// WithGmailService sets a custom Gmail service (for testing)
func WithGmailService(svc GmailService) ClientOption {
	return func(c *Client) {
		c.gmailService = svc
	}
}

// WithTemplate sets a custom email template
func WithTemplate(tmpl notification.EmailTemplate) ClientOption {
	return func(c *Client) {
		c.template = tmpl
	}
}

// NewClient creates a new Gmail client
func NewClient(from notification.Recipient, opts ...ClientOption) *Client {
	c := &Client{
		from:     from,
		template: notification.DefaultTemplate,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Send sends an email using the Gmail API
func (c *Client) Send(req *notification.EmailRequest) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("invalid email request: %w", err)
	}

	// Build template data with dynamic greeting and service reference
	data := notification.TemplateData{
		Greeting:      notification.FormatGreeting(req.To),
		ChurchName:    req.ChurchName,
		DateFormatted: req.ServiceDate.Format("01/02/2006"),
		ServiceRef:    notification.FormatServiceRef(req.ServiceDate, time.Now()),
		MinisterName:  req.MinisterName,
		AudioURL:      req.AudioURL,
		VideoURL:      req.VideoURL,
		SenderName:    req.SenderName,
	}

	// Render templates
	subject, err := c.template.RenderSubject(data)
	if err != nil {
		return fmt.Errorf("failed to render subject: %w", err)
	}

	plainText, err := c.template.RenderPlainText(data)
	if err != nil {
		return fmt.Errorf("failed to render plain text: %w", err)
	}

	htmlBody, err := c.template.RenderHTML(data)
	if err != nil {
		return fmt.Errorf("failed to render HTML: %w", err)
	}

	// Build MIME message
	rawMessage := c.buildMIMEMessage(req, subject, plainText, htmlBody)

	// Encode for Gmail API
	message := &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString([]byte(rawMessage)),
	}

	// Send via Gmail API
	_, err = c.gmailService.SendMessage(context.Background(), "me", message)
	if err != nil {
		return fmt.Errorf("%w: %v", notification.ErrSendFailed, err)
	}

	return nil
}

// buildMIMEMessage builds a RFC 2822 MIME message
func (c *Client) buildMIMEMessage(req *notification.EmailRequest, subject, plainText, htmlBody string) string {
	var msg strings.Builder

	// Headers
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", c.from.Name, c.from.Address))

	// To recipients
	msg.WriteString(fmt.Sprintf("To: %s\r\n", formatAddressList(req.To)))

	// CC recipients
	if len(req.CC) > 0 {
		msg.WriteString(fmt.Sprintf("Cc: %s\r\n", formatAddressList(req.CC)))
	}

	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: multipart/alternative; boundary=\"boundary42\"\r\n\r\n")

	// Plain text part
	msg.WriteString("--boundary42\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
	msg.WriteString(plainText)
	msg.WriteString("\r\n\r\n")

	// HTML part
	msg.WriteString("--boundary42\r\n")
	msg.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n\r\n")
	msg.WriteString(htmlBody)
	msg.WriteString("\r\n\r\n")

	msg.WriteString("--boundary42--\r\n")

	return msg.String()
}

// SendPlain sends a plain-text email with the given subject and body, such as
// the notice that a process run failed.
func (c *Client) SendPlain(to []notification.Recipient, subject, body string) error {
	if len(to) == 0 {
		return notification.ErrNoRecipients
	}

	var msg strings.Builder
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", c.from.Name, c.from.Address))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", formatAddressList(to)))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", mime.QEncoding.Encode("UTF-8", subject)))
	msg.WriteString("MIME-Version: 1.0\r\n")
	msg.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
	msg.WriteString(strings.ReplaceAll(body, "\n", "\r\n"))
	msg.WriteString("\r\n")

	message := &gmail.Message{
		Raw: base64.URLEncoding.EncodeToString([]byte(msg.String())),
	}
	if _, err := c.gmailService.SendMessage(context.Background(), "me", message); err != nil {
		return fmt.Errorf("%w: %v", notification.ErrSendFailed, err)
	}
	return nil
}

// formatAddressList formats recipients for a To or Cc header
func formatAddressList(recipients []notification.Recipient) string {
	addrs := make([]string, len(recipients))
	for i, r := range recipients {
		if r.Name != "" {
			addrs[i] = fmt.Sprintf("%s <%s>", r.Name, r.Address)
		} else {
			addrs[i] = r.Address
		}
	}
	return strings.Join(addrs, ", ")
}

// Ensure Client implements notification.EmailSender
var _ notification.EmailSender = (*Client)(nil)

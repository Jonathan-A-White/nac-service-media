//go:build integration

package steps

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	appnotif "nac-service-media/application/notification"
	"nac-service-media/domain/notification"
	"nac-service-media/infrastructure/config"
	"nac-service-media/infrastructure/gmail"

	googlegmail "google.golang.org/api/gmail/v1"

	"github.com/cucumber/godog"
)

// mockGmailService is a mock implementation for email testing
type mockGmailService struct {
	sentMessages []*googlegmail.Message
	shouldFail   bool
	failError    error
}

func (m *mockGmailService) SendMessage(ctx context.Context, userID string, message *googlegmail.Message) (*googlegmail.Message, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	m.sentMessages = append(m.sentMessages, message)
	return &googlegmail.Message{Id: "test-message-id"}, nil
}

// emailContext holds test state for email scenarios
type emailContext struct {
	cfg           *config.Config
	mockService   *mockGmailService
	gmailClient   *gmail.Client
	service       *appnotif.Service
	err           error
	audioURL      string
	videoURL      string
	serviceDate   time.Time
	ministerName  string
	recipients    []notification.Recipient
	lookupResult  []notification.Recipient
	lookupErr     error
}

// SharedEmailContext is reset before each scenario
var SharedEmailContext *emailContext

func getEmailContext() *emailContext {
	return SharedEmailContext
}

func InitializeEmailScenario(ctx *godog.ScenarioContext) {
	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		SharedEmailContext = &emailContext{
			cfg: &config.Config{
				Email: config.EmailConfig{
					FromName:    "White Plains",
					FromAddress: "whiteplainsnac@gmail.com",
					Recipients:  make(map[string]config.RecipientConfig),
				},
			},
			mockService: &mockGmailService{},
		}
		return c, nil
	})

	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		SharedEmailContext = nil
		return c, nil
	})

	// Config steps
	ctx.Step(`^the email config has from name "([^"]*)" and address "([^"]*)"$`, theEmailConfigHasFromNameAndAddress)
	ctx.Step(`^valid Gmail credentials$`, validGmailCredentials)
	ctx.Step(`^I have a recipient "([^"]*)" with name "([^"]*)" and email "([^"]*)"$`, iHaveARecipient)
	ctx.Step(`^I have a default CC "([^"]*)"$`, iHaveADefaultCC)

	// Upload context steps
	ctx.Step(`^I have uploaded files with URLs:$`, iHaveUploadedFilesWithURLs)
	ctx.Step(`^the service date is "([^"]*)"$`, theServiceDateIs)
	ctx.Step(`^the minister was "([^"]*)"$`, theMinisterWas)

	// Action steps
	ctx.Step(`^I send notification to "([^"]*)"$`, iSendNotificationTo)
	ctx.Step(`^I lookup recipient "([^"]*)"$`, iLookupRecipient)

	// Assertion steps
	ctx.Step(`^an email should be sent$`, anEmailShouldBeSent)
	ctx.Step(`^the email should be sent to "([^"]*)"$`, theEmailShouldBeSentTo)
	ctx.Step(`^the subject should be "([^"]*)"$`, theSubjectShouldBe)
	ctx.Step(`^the body should contain "([^"]*)"$`, theBodyShouldContain)
	ctx.Step(`^the body should contain the audio URL$`, theBodyShouldContainTheAudioURL)
	ctx.Step(`^the body should contain the video URL$`, theBodyShouldContainTheVideoURL)
	ctx.Step(`^I should find "([^"]*)"$`, iShouldFind)
	ctx.Step(`^I should receive an error about unknown recipient$`, iShouldReceiveAnErrorAboutUnknownRecipient)
	ctx.Step(`^the email should CC "([^"]*)"$`, theEmailShouldCC)
	ctx.Step(`^the HTML body should contain clickable audio link$`, theHTMLBodyShouldContainClickableAudioLink)
	ctx.Step(`^the HTML body should contain clickable video link$`, theHTMLBodyShouldContainClickableVideoLink)
}

func theEmailConfigHasFromNameAndAddress(name, address string) error {
	e := getEmailContext()
	e.cfg.Email.FromName = name
	e.cfg.Email.FromAddress = address
	return nil
}

func validGmailCredentials() error {
	e := getEmailContext()

	from := notification.Recipient{
		Name:    e.cfg.Email.FromName,
		Address: e.cfg.Email.FromAddress,
	}

	e.gmailClient = gmail.NewClient(from, gmail.WithGmailService(e.mockService))
	e.service = appnotif.NewService(e.gmailClient, e.cfg.Email.FromName, "Jonathan")
	return nil
}

func iHaveARecipient(key, name, email string) error {
	e := getEmailContext()
	e.cfg.Email.Recipients[key] = config.RecipientConfig{
		Name:    name,
		Address: email,
	}
	return nil
}

func iHaveADefaultCC(ccStr string) error {
	e := getEmailContext()
	// Parse "Name <email>" format
	var name, address string
	if idx := strings.Index(ccStr, " <"); idx != -1 {
		name = ccStr[:idx]
		address = strings.TrimSuffix(ccStr[idx+2:], ">")
	} else {
		address = ccStr
	}
	e.cfg.Email.DefaultCC = append(e.cfg.Email.DefaultCC, config.RecipientConfig{
		Name:    name,
		Address: address,
	})
	return nil
}

func iHaveUploadedFilesWithURLs(table *godog.Table) error {
	e := getEmailContext()
	for _, row := range table.Rows[1:] { // Skip header
		fileType := row.Cells[0].Value
		url := row.Cells[1].Value
		switch fileType {
		case "audio":
			e.audioURL = url
		case "video":
			e.videoURL = url
		}
	}
	return nil
}

func theServiceDateIs(dateStr string) error {
	e := getEmailContext()
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return fmt.Errorf("invalid date format: %w", err)
	}
	e.serviceDate = date
	return nil
}

func theMinisterWas(minister string) error {
	e := getEmailContext()
	e.ministerName = minister
	return nil
}

func iSendNotificationTo(recipientQuery string) error {
	e := getEmailContext()

	lookup := config.NewRecipientLookup(e.cfg, "")
	recipients, err := lookup.LookupRecipients([]string{recipientQuery})
	if err != nil {
		e.err = err
		return nil
	}
	e.recipients = recipients

	ccRecipients := lookup.GetDefaultCC()

	err = e.service.Send(appnotif.SendRequest{
		To:           recipients,
		CC:           ccRecipients,
		ServiceDate:  e.serviceDate,
		MinisterName: e.ministerName,
		AudioURL:     e.audioURL,
		VideoURL:     e.videoURL,
	})
	e.err = err
	return nil
}

func iLookupRecipient(query string) error {
	e := getEmailContext()
	lookup := config.NewRecipientLookup(e.cfg, "")
	e.lookupResult, e.lookupErr = lookup.LookupRecipient(query)
	return nil
}

func anEmailShouldBeSent() error {
	e := getEmailContext()
	if e.err != nil {
		return fmt.Errorf("expected email to be sent, but got error: %v", e.err)
	}
	if len(e.mockService.sentMessages) == 0 {
		return fmt.Errorf("expected email to be sent, but none were sent")
	}
	return nil
}

func theEmailShouldBeSentTo(expectedTo string) error {
	e := getEmailContext()
	if len(e.mockService.sentMessages) == 0 {
		return fmt.Errorf("no email was sent")
	}

	raw, err := decodeMessage(e.mockService.sentMessages[0])
	if err != nil {
		return err
	}

	if !strings.Contains(raw, "To: "+expectedTo) {
		return fmt.Errorf("email To header doesn't contain %q in:\n%s", expectedTo, raw)
	}
	return nil
}

func theSubjectShouldBe(expectedSubject string) error {
	e := getEmailContext()
	if len(e.mockService.sentMessages) == 0 {
		return fmt.Errorf("no email was sent")
	}

	raw, err := decodeMessage(e.mockService.sentMessages[0])
	if err != nil {
		return err
	}

	if !strings.Contains(raw, "Subject: "+expectedSubject) {
		return fmt.Errorf("email subject doesn't match, expected %q in:\n%s", expectedSubject, raw)
	}
	return nil
}

func theBodyShouldContain(expected string) error {
	e := getEmailContext()
	if len(e.mockService.sentMessages) == 0 {
		return fmt.Errorf("no email was sent")
	}

	raw, err := decodeMessage(e.mockService.sentMessages[0])
	if err != nil {
		return err
	}

	if !strings.Contains(raw, expected) {
		return fmt.Errorf("email body doesn't contain %q in:\n%s", expected, raw)
	}
	return nil
}

func theBodyShouldContainTheAudioURL() error {
	e := getEmailContext()
	return theBodyShouldContain(e.audioURL)
}

func theBodyShouldContainTheVideoURL() error {
	e := getEmailContext()
	return theBodyShouldContain(e.videoURL)
}

func iShouldFind(expected string) error {
	e := getEmailContext()
	if e.lookupErr != nil {
		return fmt.Errorf("lookup error: %v", e.lookupErr)
	}
	if len(e.lookupResult) == 0 {
		return fmt.Errorf("no recipients found")
	}

	found := false
	for _, r := range e.lookupResult {
		formatted := fmt.Sprintf("%s <%s>", r.Name, r.Address)
		if formatted == expected {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("expected to find %q, got %+v", expected, e.lookupResult)
	}
	return nil
}

func iShouldReceiveAnErrorAboutUnknownRecipient() error {
	e := getEmailContext()
	if e.lookupErr == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(e.lookupErr.Error(), "not found") {
		return fmt.Errorf("expected 'not found' error, got: %v", e.lookupErr)
	}
	return nil
}

func theEmailShouldCC(expectedCC string) error {
	e := getEmailContext()
	if len(e.mockService.sentMessages) == 0 {
		return fmt.Errorf("no email was sent")
	}

	raw, err := decodeMessage(e.mockService.sentMessages[0])
	if err != nil {
		return err
	}

	if !strings.Contains(raw, "Cc:") || !strings.Contains(raw, expectedCC) {
		return fmt.Errorf("email CC doesn't contain %q in:\n%s", expectedCC, raw)
	}
	return nil
}

func theHTMLBodyShouldContainClickableAudioLink() error {
	e := getEmailContext()
	if len(e.mockService.sentMessages) == 0 {
		return fmt.Errorf("no email was sent")
	}

	raw, err := decodeMessage(e.mockService.sentMessages[0])
	if err != nil {
		return err
	}

	expected := fmt.Sprintf(`<a href="%s">audio</a>`, e.audioURL)
	if !strings.Contains(raw, expected) {
		return fmt.Errorf("HTML body doesn't contain clickable audio link %q in:\n%s", expected, raw)
	}
	return nil
}

func theHTMLBodyShouldContainClickableVideoLink() error {
	e := getEmailContext()
	if len(e.mockService.sentMessages) == 0 {
		return fmt.Errorf("no email was sent")
	}

	raw, err := decodeMessage(e.mockService.sentMessages[0])
	if err != nil {
		return err
	}

	expected := fmt.Sprintf(`<a href="%s">video</a>`, e.videoURL)
	if !strings.Contains(raw, expected) {
		return fmt.Errorf("HTML body doesn't contain clickable video link %q in:\n%s", expected, raw)
	}
	return nil
}

func decodeMessage(msg *googlegmail.Message) (string, error) {
	decoded, err := base64.URLEncoding.DecodeString(msg.Raw)
	if err != nil {
		return "", fmt.Errorf("failed to decode message: %w", err)
	}
	return string(decoded), nil
}

package notification

import (
	"bytes"
	"fmt"
	"text/template"
	"time"
)

// TemplateData contains all the fields available for email template rendering
type TemplateData struct {
	Greeting      string // Dynamic greeting based on recipient count
	ChurchName    string
	DateFormatted string // e.g., "12/28/2025"
	ServiceRef    string // "today's", "yesterday's", or "Sunday's" based on when email is sent
	MinisterName  string
	AudioURL      string
	VideoURL      string
	SenderName    string
}

// EmailTemplate contains the templates for rendering emails
type EmailTemplate struct {
	SubjectFormat string
	PlainText     string
	HTML          string
}

// DefaultTemplate is the standard email template for service recordings
var DefaultTemplate = EmailTemplate{
	SubjectFormat: "{{.ChurchName}}: Recording of Service on {{.DateFormatted}}",
	PlainText: `{{.Greeting}}

Here is the audio and video from {{.ServiceRef}} service with {{.MinisterName}}.

Audio: {{.AudioURL}}
Video: {{.VideoURL}}

Thanks!
~{{.SenderName}}`,
	HTML: `<div dir="ltr">{{.Greeting}}<br><br>
Here is the <a href="{{.AudioURL}}">audio</a> and <a href="{{.VideoURL}}">video</a> from {{.ServiceRef}} service with {{.MinisterName}}.<br><br>
Thanks!<br>
~{{.SenderName}}</div>`,
}

// FormatGreeting creates an appropriate greeting based on number of recipients
// 1 recipient: "Dear John,"
// 2 recipients: "Dear John & Jane,"
// 3+ recipients: "Hey Everyone!"
func FormatGreeting(recipients []Recipient) string {
	switch len(recipients) {
	case 0:
		return "Hello,"
	case 1:
		name := getFirstName(recipients[0].Name)
		return fmt.Sprintf("Dear %s,", name)
	case 2:
		name1 := getFirstName(recipients[0].Name)
		name2 := getFirstName(recipients[1].Name)
		return fmt.Sprintf("Dear %s & %s,", name1, name2)
	default:
		return "Hey Everyone!"
	}
}

// getFirstName extracts the first name from a full name
func getFirstName(fullName string) string {
	if fullName == "" {
		return "Friend"
	}
	// Split on space and take first part
	for i, c := range fullName {
		if c == ' ' {
			return fullName[:i]
		}
	}
	return fullName
}

// FormatServiceRef returns the appropriate reference to the service based on
// the service date relative to the current date:
// - Same day: "today's"
// - Yesterday: "yesterday's"
// - 2-6 days ago: "Sunday's" (assuming services are on Sunday)
// - 7+ days ago: "the 12/28" (explicit date reference)
func FormatServiceRef(serviceDate, now time.Time) string {
	// Normalize to date only (ignore time component)
	serviceDay := time.Date(serviceDate.Year(), serviceDate.Month(), serviceDate.Day(), 0, 0, 0, 0, time.Local)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	diff := today.Sub(serviceDay).Hours() / 24

	switch {
	case diff == 0:
		return "today's"
	case diff == 1:
		return "yesterday's"
	case diff < 7:
		return "Sunday's"
	default:
		// More than a week old - use explicit date
		return fmt.Sprintf("the %s", serviceDate.Format("1/2"))
	}
}

// RenderSubject renders the email subject using the template
func (t *EmailTemplate) RenderSubject(data TemplateData) (string, error) {
	return renderTemplate("subject", t.SubjectFormat, data)
}

// RenderPlainText renders the plain text email body
func (t *EmailTemplate) RenderPlainText(data TemplateData) (string, error) {
	return renderTemplate("plaintext", t.PlainText, data)
}

// RenderHTML renders the HTML email body
func (t *EmailTemplate) RenderHTML(data TemplateData) (string, error) {
	return renderTemplate("html", t.HTML, data)
}

func renderTemplate(name, tmplStr string, data TemplateData) (string, error) {
	tmpl, err := template.New(name).Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	appnotif "nac-service-media/application/notification"
	"nac-service-media/domain/notification"
	"nac-service-media/infrastructure/config"
	"nac-service-media/infrastructure/gmail"

	"github.com/spf13/cobra"
)

var (
	emailTo        []string
	emailDate      string
	emailMinister  string
	emailAudioURL  string
	emailVideoURL  string
	emailSenderKey string
)

var sendEmailCmd = &cobra.Command{
	Use:   "send-email",
	Short: "Send email notification with recording links",
	Long: `Send an email notification to recipients with links to the service recording.

Recipients can be specified by name (first name, last name, or full name) or by
their config key. Multiple recipients can be specified using multiple --to flags
or comma-separated values.

Examples:
  # Send to a single recipient
  nac-service-media send-email --to jonathan --date 2025-12-28 --minister "Pr. Henkel" \
    --audio-url "https://..." --video-url "https://..."

  # Send to multiple recipients
  nac-service-media send-email --to jonathan --to jane --date 2025-12-28 ...
  nac-service-media send-email --to "jonathan,jane" --date 2025-12-28 ...`,
	RunE: runSendEmail,
}

func init() {
	rootCmd.AddCommand(sendEmailCmd)
	sendEmailCmd.Flags().StringArrayVar(&emailTo, "to", nil, "Recipient(s) by name or config key (can be repeated or comma-separated)")
	sendEmailCmd.Flags().StringVar(&emailDate, "date", "", "Service date in YYYY-MM-DD format")
	sendEmailCmd.Flags().StringVar(&emailMinister, "minister", "", "Minister's name (e.g., 'Pr. Henkel')")
	sendEmailCmd.Flags().StringVar(&emailAudioURL, "audio-url", "", "Google Drive URL for audio file")
	sendEmailCmd.Flags().StringVar(&emailVideoURL, "video-url", "", "Google Drive URL for video file")
	sendEmailCmd.Flags().StringVar(&emailSenderKey, "sender", "", "Sender config key (defaults to config default_sender)")

	sendEmailCmd.MarkFlagRequired("to")
	sendEmailCmd.MarkFlagRequired("date")
	sendEmailCmd.MarkFlagRequired("minister")
}

func runSendEmail(cmd *cobra.Command, args []string) error {
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("configuration not loaded; ensure config/config.yaml exists")
	}

	// Parse service date
	serviceDate, err := time.Parse("2006-01-02", emailDate)
	if err != nil {
		return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
	}

	// Require at least one media URL
	if emailAudioURL == "" && emailVideoURL == "" {
		return fmt.Errorf("at least one of --audio-url or --video-url is required")
	}

	// Lookup recipients
	lookup := config.NewRecipientLookup(cfg, cfgFile)
	recipients, err := lookup.LookupRecipients(emailTo)
	if err != nil {
		return fmt.Errorf("failed to lookup recipients: %w", err)
	}

	// Get default CC
	ccRecipients := lookup.GetDefaultCC()

	// Lookup sender
	mgr := config.NewConfigManager(cfg, cfgFile)
	var senderName string
	if emailSenderKey != "" {
		sender, err := mgr.GetSender(emailSenderKey)
		if err != nil {
			return fmt.Errorf("sender '%s' not found in config\n\nTo fix this, run:\n  %s", emailSenderKey, config.SuggestAddSenderCommand(emailSenderKey))
		}
		senderName = sender.Name
	} else {
		sender, err := mgr.GetDefaultSender()
		if err != nil {
			return fmt.Errorf("no default sender configured. Either specify --sender or set senders.default_sender in config")
		}
		senderName = sender.Name
	}

	// Create Gmail client with OAuth
	ctx := cmd.Context()
	from := notification.Recipient{
		Name:    cfg.Email.FromName,
		Address: cfg.Email.FromAddress,
	}

	// Use a separate token file for Gmail (different scope than Drive)
	gmailTokenFile := "gmail_token.json"
	gmailClient, err := gmail.NewClientWithOAuth(ctx, gmail.OAuthConfig{
		CredentialsFile: cfg.Google.CredentialsFile,
		TokenFile:       gmailTokenFile,
	}, from)
	if err != nil {
		return fmt.Errorf("failed to create Gmail client: %w", err)
	}

	return RunSendEmailWithDependencies(
		ctx,
		gmailClient,
		cfg.Email.FromName, // Church name (used in subject)
		senderName,         // Sender name (for signature)
		recipients,
		ccRecipients,
		serviceDate,
		emailMinister,
		emailAudioURL,
		emailVideoURL,
		os.Stdout,
	)
}

// RunSendEmailWithDependencies runs the send-email command with injected dependencies (for testing)
func RunSendEmailWithDependencies(
	ctx context.Context,
	sender notification.EmailSender,
	churchName string,
	senderName string,
	recipients []notification.Recipient,
	ccRecipients []notification.Recipient,
	serviceDate time.Time,
	ministerName string,
	audioURL string,
	videoURL string,
	output io.Writer,
) error {
	service := appnotif.NewService(sender, churchName, senderName)

	// Display what we're about to send
	toNames := make([]string, len(recipients))
	for i, r := range recipients {
		toNames[i] = fmt.Sprintf("%s <%s>", r.Name, r.Address)
	}
	fmt.Fprintf(output, "Sending email to: %s\n", strings.Join(toNames, ", "))

	if len(ccRecipients) > 0 {
		ccNames := make([]string, len(ccRecipients))
		for i, r := range ccRecipients {
			ccNames[i] = fmt.Sprintf("%s <%s>", r.Name, r.Address)
		}
		fmt.Fprintf(output, "CC: %s\n", strings.Join(ccNames, ", "))
	}

	fmt.Fprintf(output, "Subject: %s: Recording of Service on %s\n", churchName, serviceDate.Format("01/02/2006"))
	fmt.Fprintf(output, "Minister: %s\n", ministerName)
	if audioURL != "" {
		fmt.Fprintf(output, "Audio URL: %s\n", audioURL)
	}
	if videoURL != "" {
		fmt.Fprintf(output, "Video URL: %s\n", videoURL)
	}
	fmt.Fprintln(output)

	// Send the email
	fmt.Fprintf(output, "Sending email...\n")
	err := service.Send(appnotif.SendRequest{
		To:           recipients,
		CC:           ccRecipients,
		ServiceDate:  serviceDate,
		MinisterName: ministerName,
		AudioURL:     audioURL,
		VideoURL:     videoURL,
	})
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	fmt.Fprintf(output, "Email sent successfully!\n")
	return nil
}

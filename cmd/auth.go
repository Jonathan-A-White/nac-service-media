package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"nac-service-media/domain/notification"
	"nac-service-media/infrastructure/drive"
	"nac-service-media/infrastructure/gmail"

	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	gdrive "google.golang.org/api/drive/v3"
	ggmail "google.golang.org/api/gmail/v1"
)

var authFixFlag bool

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage Google OAuth authentication",
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the status of saved OAuth tokens",
	Long: `Check whether the saved Drive and Gmail OAuth tokens are valid and refreshable.

Use --fix to automatically re-authenticate any invalid or missing tokens.

Examples:
  nac-service-media auth status
  nac-service-media auth status --fix`,
	RunE: runAuthStatus,
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authStatusCmd)
	authStatusCmd.Flags().BoolVar(&authFixFlag, "fix", false, "Re-authenticate invalid or missing tokens")
}

// tokenStatus describes the state of a saved OAuth token
type tokenStatus int

const (
	tokenOK          tokenStatus = iota // Valid or successfully refreshed
	tokenMissing                        // Token file does not exist
	tokenCorrupt                        // Token file exists but can't be parsed
	tokenNoRefresh                      // Token has no refresh token
	tokenExpired                        // Refresh token is revoked or expired
	tokenCredsMissing                   // Credentials file not found
	tokenCredsInvalid                   // Credentials file can't be parsed
)

func (s tokenStatus) String() string {
	switch s {
	case tokenOK:
		return "valid"
	case tokenMissing:
		return "not found"
	case tokenCorrupt:
		return "corrupt"
	case tokenNoRefresh:
		return "no refresh token"
	case tokenExpired:
		return "expired (needs re-auth)"
	case tokenCredsMissing:
		return "credentials file missing"
	case tokenCredsInvalid:
		return "credentials file invalid"
	default:
		return "unknown"
	}
}

type tokenCheckResult struct {
	Status tokenStatus
	Expiry time.Time
	Error  error
}

func (r tokenCheckResult) ok() bool {
	return r.Status == tokenOK
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	cfg := GetConfig()
	if cfg == nil {
		return fmt.Errorf("configuration not loaded; ensure config/config.yaml exists")
	}

	ctx := cmd.Context()
	return RunAuthStatusWithDependencies(ctx, cfg.Google.CredentialsFile, cfg.Google.TokenFile, cfg.Google.GmailTokenFile, authFixFlag, os.Stdout)
}

// RunAuthStatusWithDependencies checks OAuth token status with injected dependencies
func RunAuthStatusWithDependencies(ctx context.Context, credentialsFile, driveTokenFile, gmailTokenFile string, fix bool, output io.Writer) error {
	fmt.Fprintln(output, "Checking OAuth token status...")
	fmt.Fprintln(output)

	// Check Drive token
	driveResult := checkToken(ctx, credentialsFile, driveTokenFile, gdrive.DriveScope)
	printTokenStatus(output, "Google Drive", driveResult)

	// Check Gmail token
	gmailResult := checkToken(ctx, credentialsFile, gmailTokenFile, ggmail.GmailSendScope)
	printTokenStatus(output, "Gmail", gmailResult)

	fmt.Fprintln(output)

	if driveResult.ok() && gmailResult.ok() {
		fmt.Fprintln(output, "All tokens are valid.")
		return nil
	}

	if !fix {
		fmt.Fprintln(output, "Run with --fix to re-authenticate invalid tokens.")
		return nil
	}

	// Fix invalid tokens
	fmt.Fprintln(output, "Attempting to fix invalid tokens...")
	fmt.Fprintln(output)

	if !driveResult.ok() {
		fmt.Fprintln(output, "Re-authenticating Google Drive...")
		_, err := drive.NewClientWithOAuth(ctx, credentialsFile, driveTokenFile)
		if err != nil {
			return fmt.Errorf("drive re-authentication failed: %w", err)
		}
		fmt.Fprintln(output, "Drive token saved.")
		fmt.Fprintln(output)
	}

	if !gmailResult.ok() {
		fmt.Fprintln(output, "Re-authenticating Gmail...")
		_, err := gmail.NewClientWithOAuth(ctx, gmail.OAuthConfig{
			CredentialsFile: credentialsFile,
			TokenFile:       gmailTokenFile,
		}, notification.Recipient{})
		if err != nil {
			return fmt.Errorf("gmail re-authentication failed: %w", err)
		}
		fmt.Fprintln(output, "Gmail token saved.")
		fmt.Fprintln(output)
	}

	fmt.Fprintln(output, "All tokens are now valid.")
	return nil
}

func printTokenStatus(output io.Writer, name string, result tokenCheckResult) {
	if result.ok() {
		if result.Expiry.IsZero() {
			fmt.Fprintf(output, "  %-14s %s\n", name+":", result.Status)
		} else {
			fmt.Fprintf(output, "  %-14s %s (expires %s)\n", name+":", result.Status, result.Expiry.Format(time.RFC3339))
		}
	} else {
		msg := result.Status.String()
		if result.Error != nil {
			msg += " - " + result.Error.Error()
		}
		fmt.Fprintf(output, "  %-14s %s\n", name+":", msg)
	}
}

// checkToken loads a saved token and verifies it can be refreshed
func checkToken(ctx context.Context, credentialsFile, tokenFile, scope string) tokenCheckResult {
	// Load credentials
	credBytes, err := os.ReadFile(credentialsFile)
	if err != nil {
		return tokenCheckResult{Status: tokenCredsMissing, Error: err}
	}

	oauthCfg, err := google.ConfigFromJSON(credBytes, scope)
	if err != nil {
		return tokenCheckResult{Status: tokenCredsInvalid, Error: err}
	}

	// Load token
	f, err := os.Open(tokenFile)
	if err != nil {
		return tokenCheckResult{Status: tokenMissing, Error: err}
	}
	defer f.Close()

	var token oauth2.Token
	if err := json.NewDecoder(f).Decode(&token); err != nil {
		return tokenCheckResult{Status: tokenCorrupt, Error: err}
	}

	if token.RefreshToken == "" {
		return tokenCheckResult{Status: tokenNoRefresh, Expiry: token.Expiry}
	}

	// Try to refresh
	tokenSource := oauthCfg.TokenSource(ctx, &token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return tokenCheckResult{Status: tokenExpired, Expiry: token.Expiry, Error: err}
	}

	// Save refreshed token if it changed
	if newToken.AccessToken != token.AccessToken {
		if sf, err := os.Create(tokenFile); err == nil {
			json.NewEncoder(sf).Encode(newToken)
			sf.Close()
		}
	}

	return tokenCheckResult{Status: tokenOK, Expiry: newToken.Expiry}
}

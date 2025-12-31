package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// OAuthConfig holds the configuration for OAuth 2.0 authentication
type OAuthConfig struct {
	CredentialsFile string // Path to OAuth client credentials JSON
	TokenFile       string // Path to store/load token
}

// newOAuthDriveService creates a Drive service using OAuth 2.0 user authentication
func newOAuthDriveService(ctx context.Context, cfg OAuthConfig) (*GoogleDriveService, error) {
	b, err := os.ReadFile(cfg.CredentialsFile)
	if err != nil {
		return nil, fmt.Errorf("unable to read OAuth credentials file: %w", err)
	}

	// Parse the OAuth client credentials
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse OAuth credentials: %w", err)
	}

	// Get or create token
	token, err := getToken(ctx, config, cfg.TokenFile)
	if err != nil {
		return nil, fmt.Errorf("unable to get OAuth token: %w", err)
	}

	// Create the Drive service
	client := config.Client(ctx, token)
	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create drive service: %w", err)
	}

	return &GoogleDriveService{service: srv}, nil
}

// getToken retrieves a token from file or initiates the OAuth flow
func getToken(ctx context.Context, config *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	// Try to load existing token
	token, err := loadToken(tokenFile)
	if err == nil {
		// Check if token is still valid or can be refreshed
		tokenSource := config.TokenSource(ctx, token)
		newToken, err := tokenSource.Token()
		if err == nil {
			// Save refreshed token if it changed
			if newToken.AccessToken != token.AccessToken {
				saveToken(tokenFile, newToken)
			}
			return newToken, nil
		}
		// Token refresh failed, need to re-authenticate
	}

	// No valid token, initiate OAuth flow
	return getTokenFromWeb(ctx, config, tokenFile)
}

// loadToken loads a token from a file
func loadToken(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	token := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(token)
	return token, err
}

// saveToken saves a token to a file
func saveToken(file string, token *oauth2.Token) error {
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(token)
}

// getTokenFromWeb initiates the OAuth flow via browser
func getTokenFromWeb(ctx context.Context, config *oauth2.Config, tokenFile string) (*oauth2.Token, error) {
	// Use localhost redirect for installed apps
	config.RedirectURL = "http://localhost:8085/callback"

	// Channel to receive the auth code
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Start local server to receive callback
	server := &http.Server{Addr: ":8085"}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errChan <- fmt.Errorf("no code in callback")
			fmt.Fprintf(w, "Error: No authorization code received")
			return
		}
		codeChan <- code
		fmt.Fprintf(w, "<html><body><h1>Authorization successful!</h1><p>You can close this window and return to the terminal.</p></body></html>")
	})

	// Start server in background
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Generate auth URL
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)

	fmt.Println()
	fmt.Println("Opening browser for Google authentication...")
	fmt.Println("If the browser doesn't open, please visit this URL:")
	fmt.Println()
	fmt.Println(authURL)
	fmt.Println()

	// Try to open browser
	openBrowser(authURL)

	// Wait for callback
	var authCode string
	select {
	case authCode = <-codeChan:
		// Got the code
	case err := <-errChan:
		server.Shutdown(ctx)
		return nil, err
	case <-ctx.Done():
		server.Shutdown(ctx)
		return nil, ctx.Err()
	}

	// Shutdown server
	server.Shutdown(ctx)

	// Exchange code for token
	token, err := config.Exchange(ctx, authCode)
	if err != nil {
		return nil, fmt.Errorf("unable to exchange auth code: %w", err)
	}

	// Save token for future use
	if err := saveToken(tokenFile, token); err != nil {
		fmt.Printf("Warning: couldn't save token: %v\n", err)
	}

	fmt.Println("Authentication successful!")
	return token, nil
}

// openBrowser opens a URL in the default browser
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		// Try various Linux browser openers
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", url)
		} else if _, err := exec.LookPath("wslview"); err == nil {
			// WSL
			cmd = exec.Command("wslview", url)
		} else {
			// Try Windows browser from WSL
			cmd = exec.Command("cmd.exe", "/c", "start", url)
		}
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	}

	if cmd != nil {
		cmd.Start()
	}
}

// NewClientWithOAuth creates a new Google Drive client using OAuth 2.0
func NewClientWithOAuth(ctx context.Context, credentialsPath, tokenPath string, opts ...ClientOption) (*Client, error) {
	c := &Client{}

	for _, opt := range opts {
		opt(c)
	}

	// If no custom drive service was provided, create one with OAuth
	if c.driveService == nil {
		svc, err := newOAuthDriveService(ctx, OAuthConfig{
			CredentialsFile: credentialsPath,
			TokenFile:       tokenPath,
		})
		if err != nil {
			return nil, err
		}
		c.driveService = svc
	}

	return c, nil
}

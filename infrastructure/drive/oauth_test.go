package drive

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// A hidden scheduled run has nobody at the browser: the sign-in fallback must
// give up after its bounded wait so the run can report the failure.
func TestGetTokenFromWeb_NonInteractiveGivesUpAfterBoundedWait(t *testing.T) {
	oldOpen, oldAddr := openBrowser, authListenAddr
	opened := false
	openBrowser = func(string) { opened = true }
	authListenAddr = "127.0.0.1:0"
	defer func() { openBrowser, authListenAddr = oldOpen, oldAddr }()

	cfg := &oauth2.Config{ClientID: "id", Endpoint: oauth2.Endpoint{AuthURL: "https://example.com/auth"}}
	tokenFile := filepath.Join(t.TempDir(), "token.json")

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		_, err := getTokenFromWeb(context.Background(), cfg, tokenFile, 100*time.Millisecond)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, ErrBrowserAuthTimeout) {
			t.Fatalf("expected ErrBrowserAuthTimeout, got %v", err)
		}
		if waited := time.Since(start); waited > 2*time.Second {
			t.Errorf("waited %s, want about 100ms", waited)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("browser sign-in still waiting after 3s; a non-interactive run would hang here forever")
	}
	if !opened {
		t.Error("the sign-in URL should still be offered to a browser")
	}
}

// The bounded wait applies only to the fallback a ClientOption asks for.
func TestWithBrowserAuthTimeout_SetsTheWait(t *testing.T) {
	c := &Client{}
	WithBrowserAuthTimeout(2 * time.Minute)(c)
	if c.authTimeout != 2*time.Minute {
		t.Errorf("authTimeout = %s, want 2m0s", c.authTimeout)
	}
}

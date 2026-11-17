// Package postern is the Sunday watch's read of Postern's alerts routes, as
// nac-service-media's own family: every request is signed with the family's
// key by a Postern2 proof (Postern's docs/api.md, Authentication).
package postern

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"

	domain "nac-service-media/domain/watch"
)

// maxReply is the most of a reply the client reads.
const maxReply = 8 << 20

// maxPages bounds the walk through the alerts list.
const maxPages = 20

// Key is the family's secp256k1 key. Printed, it shows no more than the start
// of its public half.
type Key struct{ priv *btcec.PrivateKey }

func (k Key) String() string {
	if k.priv == nil {
		return "postern key (none)"
	}
	return "postern key " + k.PublicHex()[:12] + "…"
}

// GoString is String, so %#v shows no more.
func (k Key) GoString() string { return k.String() }

// PublicHex is the compressed public key, hex.
func (k Key) PublicHex() string { return hex.EncodeToString(k.priv.PubKey().SerializeCompressed()) }

// sign is the DER ECDSA signature, hex, over sha256(message).
func (k Key) sign(message []byte) string {
	digest := sha256.Sum256(message)
	return hex.EncodeToString(ecdsa.Sign(k.priv, digest[:]).Serialize())
}

func (k Key) authorization(method, target string, body []byte, nonce string) string {
	sum := sha256.Sum256(body)
	msg := "postern-v2\n" + method + "\n" + target + "\n" + hex.EncodeToString(sum[:]) + "\n" + nonce
	return "Postern2 " + k.PublicHex() + ":" + nonce + ":" + k.sign([]byte(msg))
}

// Refused is Postern answering outside 2xx.
type Refused struct {
	Route  string
	Status int
	Reason string
	Said   string
}

func (e *Refused) Error() string {
	s := fmt.Sprintf("Postern refused %s %d", e.Route, e.Status)
	if e.Reason != "" {
		s += " " + e.Reason
	}
	if said := strings.Join(strings.Fields(e.Said), " "); said != "" {
		if len(said) > 200 {
			said = said[:200] + "…"
		}
		s += ": " + said
	}
	return s
}

// Client asks Postern's alerts routes. It is the watch's domain.Alerts.
type Client struct {
	Base string // Postern's backend, scheme and host
	Key  *Key
	HTTP *http.Client
}

var _ domain.Alerts = (*Client)(nil)

type wireAlert struct {
	ID   string `json:"id"`
	Seq  int    `json:"seq"`
	Kind string `json:"kind"`
	When string `json:"when"`
}

// List is every alert the family holds (Postern keeps 30 days), oldest first.
func (c *Client) List(ctx context.Context) ([]domain.Alert, error) {
	var all []domain.Alert
	seen := map[string]bool{}
	since := 0
	for page := 0; page < maxPages; page++ {
		b, err := c.signed(ctx, http.MethodGet, "/api/alerts?since="+strconv.Itoa(since))
		if err != nil {
			return nil, err
		}
		var reply struct {
			Alerts []wireAlert `json:"alerts"`
			Next   int         `json:"next"`
		}
		if err := json.Unmarshal(b, &reply); err != nil {
			return nil, errors.New("Postern's alerts are not JSON of the form {\"alerts\": [...]}")
		}
		top := since
		for _, w := range reply.Alerts {
			a := domain.Alert{ID: w.ID, Kind: w.Kind}
			if w.When != "" {
				if t, err := time.Parse(time.RFC3339, w.When); err == nil {
					a.When = t
				}
			}
			if !seen[a.ID] {
				seen[a.ID] = true
				all = append(all, a)
			}
			if w.Seq > top {
				top = w.Seq
			}
		}
		if reply.Next > top {
			top = reply.Next
		}
		if len(reply.Alerts) == 0 || top <= since {
			return all, nil
		}
		since = top
	}
	return all, nil
}

// Claim claims one alert, DELETE /api/alerts/{id}. One already gone is claimed.
func (c *Client) Claim(ctx context.Context, id string) error {
	_, err := c.signed(ctx, http.MethodDelete, "/api/alerts/"+url.PathEscape(id))
	var r *Refused
	if errors.As(err, &r) && r.Status == http.StatusNotFound {
		return nil
	}
	return err
}

// signed makes one signed request; a nonce refusal is tried once more from a
// fresh challenge, as Postern asks (nothing was acted on).
func (c *Client) signed(ctx context.Context, method, path string) ([]byte, error) {
	b, err := c.signedOnce(ctx, method, path)
	var r *Refused
	if errors.As(err, &r) && r.Status == http.StatusUnauthorized && r.Reason == "nonce" {
		b, err = c.signedOnce(ctx, method, path)
	}
	return b, err
}

func (c *Client) signedOnce(ctx context.Context, method, path string) ([]byte, error) {
	nonce, err := c.challenge(ctx)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimRight(c.Base, "/") + path)
	if err != nil {
		return nil, fmt.Errorf("Postern's address: %w", err)
	}
	return c.do(ctx, method, u, c.Key.authorization(method, u.RequestURI(), nil, nonce))
}

func (c *Client) challenge(ctx context.Context) (string, error) {
	u, err := url.Parse(strings.TrimRight(c.Base, "/") + "/api/challenge")
	if err != nil {
		return "", fmt.Errorf("Postern's address: %w", err)
	}
	b, err := c.do(ctx, http.MethodGet, u, "")
	if err != nil {
		return "", err
	}
	var reply struct {
		Nonce string `json:"nonce"`
	}
	if err := json.Unmarshal(b, &reply); err != nil || reply.Nonce == "" {
		return "", errors.New("Postern's challenge carried no nonce")
	}
	return reply.Nonce, nil
}

func (c *Client) do(ctx context.Context, method string, u *url.URL, auth string) ([]byte, error) {
	route := method + " " + u.Path
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", route, err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asking Postern %s: %w", route, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxReply))
	if err != nil {
		return nil, fmt.Errorf("reading Postern's reply to %s: %w", route, err)
	}
	if resp.StatusCode/100 != 2 {
		r := &Refused{Route: route, Status: resp.StatusCode}
		var said struct {
			Error  string `json:"error"`
			Reason string `json:"reason"`
		}
		if json.Unmarshal(bytes.TrimSpace(b), &said) == nil {
			r.Reason, r.Said = said.Reason, said.Error
		}
		return nil, r
	}
	return b, nil
}

// --- the settings file ---

// DefaultEnvPath is ~/.config/nac-service-media/postern.env.
func DefaultEnvPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "nac-service-media", "postern.env")
}

// Settings is what the watch reads from postern.env.
type Settings struct {
	URL string
	Key *Key
}

// LoadEnv reads KEY=VALUE lines: POSTERN_URL, and the family's key as either
// POSTERN_KEY (64 hex) or POSTERN_KEY_FILE (a file of 64 hex, relative to the
// env file). Other keys (the address's token, say) are ignored. A file that
// others may read is refused, and a bad key is refused without a word of it.
func LoadEnv(path string) (*Settings, error) {
	if path == "" {
		return nil, errors.New("no postern.env named and no home directory to find it in")
	}
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("no Postern settings at %s: the watch needs ~/.config/nac-service-media/postern.env (POSTERN_URL and POSTERN_KEY, mode 0600)", path)
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is mode %04o: others may read it; make it 0600 (chmod 600)", path, info.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	vals := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		vals[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	s := &Settings{URL: vals["POSTERN_URL"]}
	if s.URL == "" {
		return nil, fmt.Errorf("%s has no POSTERN_URL", path)
	}
	keyHex := vals["POSTERN_KEY"]
	if keyHex == "" && vals["POSTERN_KEY_FILE"] != "" {
		kp := vals["POSTERN_KEY_FILE"]
		if !filepath.IsAbs(kp) {
			kp = filepath.Join(filepath.Dir(path), kp)
		}
		ki, err := os.Stat(kp)
		if err != nil {
			return nil, fmt.Errorf("reading the key file named in %s: %w", path, err)
		}
		if ki.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("the key file %s is mode %04o: make it 0600", kp, ki.Mode().Perm())
		}
		kb, err := os.ReadFile(kp)
		if err != nil {
			return nil, fmt.Errorf("reading the key file named in %s: %w", path, err)
		}
		keyHex = strings.TrimSpace(string(kb))
	}
	if keyHex == "" {
		return nil, fmt.Errorf("%s has no POSTERN_KEY (or POSTERN_KEY_FILE)", path)
	}
	raw, err := hex.DecodeString(keyHex)
	if err != nil || len(raw) != 32 {
		return nil, fmt.Errorf("the Postern key in %s is not 64 hex characters", path)
	}
	priv, _ := btcec.PrivKeyFromBytes(raw)
	if priv.Key.IsZero() {
		return nil, fmt.Errorf("the Postern key in %s is not 64 hex characters", path)
	}
	s.Key = &Key{priv: priv}
	return s, nil
}

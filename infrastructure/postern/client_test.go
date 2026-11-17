package postern

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

const testKeyHex = "0101010101010101010101010101010101010101010101010101010101010101"

func testKey(t *testing.T) *Key {
	t.Helper()
	raw, _ := hex.DecodeString(testKeyHex)
	priv, _ := btcec.PrivKeyFromBytes(raw)
	return &Key{priv: priv}
}

// fakePostern checks every signed request the way Postern does (docs/api.md,
// Authentication) and answers the alerts routes.
func fakePostern(t *testing.T, alerts string, claimed *[]string) *httptest.Server {
	t.Helper()
	var nonces atomic.Int32
	used := map[string]bool{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/challenge" {
			n := nonces.Add(1)
			w.Write([]byte(`{"nonce":"` + hex.EncodeToString([]byte{byte(n), 0xaa}) + `"}`))
			return
		}
		h := strings.TrimPrefix(r.Header.Get("Authorization"), "Postern2 ")
		parts := strings.Split(h, ":")
		if len(parts) != 3 {
			http.Error(w, `{"reason":"malformed_authorization"}`, 401)
			return
		}
		pub, nonce, sig := parts[0], parts[1], parts[2]
		if used[nonce] {
			http.Error(w, `{"reason":"nonce"}`, 401)
			return
		}
		used[nonce] = true
		sum := sha256.Sum256(nil)
		msg := "postern-v2\n" + r.Method + "\n" + r.RequestURI + "\n" + hex.EncodeToString(sum[:]) + "\n" + nonce
		digest := sha256.Sum256([]byte(msg))
		pubRaw, _ := hex.DecodeString(pub)
		pk, err := btcec.ParsePubKey(pubRaw)
		sigRaw, _ := hex.DecodeString(sig)
		parsed, serr := ecdsa.ParseDERSignature(sigRaw)
		if err != nil || serr != nil || !parsed.Verify(digest[:], pk) {
			http.Error(w, `{"error":"bad signature","reason":"signature"}`, 401)
			return
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/alerts":
			if r.URL.Query().Get("since") != "0" { // Postern lists only what is above since
				w.Write([]byte(`{"alerts":[],"next":5}`))
				return
			}
			w.Write([]byte(alerts))
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/api/alerts/"):
			*claimed = append(*claimed, strings.TrimPrefix(r.URL.Path, "/api/alerts/"))
			w.WriteHeader(204)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestClient_ListsAndClaimsWithAValidProof(t *testing.T) {
	var claimed []string
	srv := fakePostern(t, `{"alerts":[
		{"id":"x1","seq":4,"kind":"service-recording","when":"2026-11-15T00:00:00-05:00"},
		{"id":"x2","seq":5,"kind":"unread"}],"next":5}`, &claimed)
	defer srv.Close()
	c := &Client{Base: srv.URL, Key: testKey(t), HTTP: srv.Client()}

	alerts, err := c.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 2 || alerts[0].ID != "x1" || alerts[0].Kind != "service-recording" ||
		alerts[0].When.Format("2006-01-02") != "2026-11-15" {
		t.Fatalf("alerts = %+v", alerts)
	}
	if err := c.Claim(context.Background(), "x1"); err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0] != "x1" {
		t.Errorf("claimed %v", claimed)
	}
}

func TestClient_RefusalNamesTheReasonNotTheKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/challenge" {
			w.Write([]byte(`{"nonce":"aa"}`))
			return
		}
		http.Error(w, `{"error":"no licence","reason":"no_licence"}`, 401)
	}))
	defer srv.Close()
	_, err := (&Client{Base: srv.URL, Key: testKey(t), HTTP: srv.Client()}).List(context.Background())
	if err == nil || !strings.Contains(err.Error(), "no_licence") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), testKeyHex) {
		t.Fatal("the private key must never be in an error")
	}
}

func TestClient_ClaimOfAGoneAlertIsClaimed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/challenge" {
			w.Write([]byte(`{"nonce":"aa"}`))
			return
		}
		http.Error(w, `{"error":"gone"}`, 404)
	}))
	defer srv.Close()
	if err := (&Client{Base: srv.URL, Key: testKey(t), HTTP: srv.Client()}).Claim(context.Background(), "x1"); err != nil {
		t.Fatal(err)
	}
}

func writeEnv(t *testing.T, mode os.FileMode, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "postern.env")
	if err := os.WriteFile(p, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadEnv_ReadsURLAndKey(t *testing.T) {
	p := writeEnv(t, 0o600, "# the watch's\nexport POSTERN_URL=\"https://postern.example.org/\"\nPOSTERN_KEY="+testKeyHex+"\nPOSTERN_ALERTS_TOKEN=abc\n")
	s, err := LoadEnv(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.URL != "https://postern.example.org/" || s.Key.PublicHex() != testKey(t).PublicHex() {
		t.Fatalf("%+v", s)
	}
}

func TestLoadEnv_KeyFileNextToTheEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "k"), []byte(testKeyHex+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "postern.env")
	if err := os.WriteFile(p, []byte("POSTERN_URL=http://x\nPOSTERN_KEY_FILE=k\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadEnv(p)
	if err != nil || s.Key.PublicHex() != testKey(t).PublicHex() {
		t.Fatalf("%v %v", s, err)
	}
}

func TestLoadEnv_Refusals(t *testing.T) {
	tests := []struct {
		name, body, want string
		mode             os.FileMode
	}{
		{"others may read", "POSTERN_URL=http://x\nPOSTERN_KEY=" + testKeyHex + "\n", "0600", 0o644},
		{"no url", "POSTERN_KEY=" + testKeyHex + "\n", "POSTERN_URL", 0o600},
		{"no key", "POSTERN_URL=http://x\n", "POSTERN_KEY", 0o600},
		{"bad key never echoed", "POSTERN_URL=http://x\nPOSTERN_KEY=zz-secret\n", "64 hex", 0o600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := LoadEnv(writeEnv(t, tt.mode, tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "zz-secret") {
				t.Fatal("a bad key was echoed")
			}
		})
	}
	if _, err := LoadEnv(filepath.Join(t.TempDir(), "nope.env")); err == nil || !strings.Contains(err.Error(), "postern.env") {
		t.Fatalf("missing file: %v", err)
	}
}

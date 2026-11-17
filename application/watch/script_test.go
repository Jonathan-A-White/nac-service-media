package watch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The script runs under a real sh against a fixture of the PC's folders, and
// its output is read back by the service: the format the PC prints and the
// format the service parses are one thing.
func TestRemoteScript_RunsUnderShAndIsReadBack(t *testing.T) {
	root := t.TempDir()
	videos := filepath.Join(root, "Videos")
	logs := filepath.Join(root, "repo", "logs")
	for _, d := range []string{videos, logs} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, body string) {
		t.Helper()
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(videos, "2026-11-15 10-06-16.mp4"), "12345")
	write(filepath.Join(videos, "2026-11-08 10-06-16.mp4"), "other day")
	write(filepath.Join(logs, "scheduled-run-2026-11-15_12-30-02.log"),
		"x\n      Sent to: A <a@example.org>\n[t] Exit code: 0\n[t] SUCCESS: Processing completed successfully\n")
	write(filepath.Join(logs, "scheduled-run-2026-11-08_12-30-02.log"), "[t] Exit code: 1\n")

	run := runnerFunc(func(ctx context.Context, script string) (string, error) {
		out, err := exec.CommandContext(ctx, "sh", "-c", script).Output()
		return string(out), err
	})
	a := &fakeAlerts{}
	m := &fakeMail{}
	s := &Service{Alerts: a, PC: run, Mailer: m, Out: &strings.Builder{}, VideosDir: videos, RepoDir: filepath.Join(root, "repo")}
	res, err := s.Run(context.Background(), "2026-11-15")
	if err != nil {
		t.Fatal(err)
	}
	// a clean run that sent its mail, and Postern holds nothing: other
	if res.Class != Other {
		t.Fatalf("class %q\n%s", res.Class, res.Body)
	}
	for _, want := range []string{"2026-11-15 10-06-16.mp4 (5 bytes)", "scheduled-run-2026-11-15_12-30-02.log", "Sent to:"} {
		if !strings.Contains(res.Body, want) {
			t.Errorf("body lacks %q:\n%s", want, res.Body)
		}
	}
	if strings.Contains(res.Body, "2026-11-08") {
		t.Errorf("another day's files leaked into the evidence:\n%s", res.Body)
	}

	// a day with no files at all: quiet
	res, err = s.Run(context.Background(), "2026-11-22")
	if err != nil || res.Class != NoRecording {
		t.Fatalf("empty day: %v %v", res.Class, err)
	}
}

type runnerFunc func(ctx context.Context, script string) (string, error)

func (f runnerFunc) Run(ctx context.Context, script string) (string, error) { return f(ctx, script) }

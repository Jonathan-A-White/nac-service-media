package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	appprocess "nac-service-media/application/process"
	"nac-service-media/domain/distribution"
	"nac-service-media/domain/notification"
	"nac-service-media/infrastructure/config"
	infradetection "nac-service-media/infrastructure/detection"
	"nac-service-media/infrastructure/lock"
)

// sentMail is one plain email the fake mailer was asked to send.
type sentMail struct {
	to      []notification.Recipient
	subject string
	body    string
}

// fakeMailer stands in for the Gmail client: it records the links email and
// every plain (failure) email instead of sending them.
type fakeMailer struct {
	links   []*notification.EmailRequest
	plain   []sentMail
	sendErr error
}

func (m *fakeMailer) Send(req *notification.EmailRequest) error {
	m.links = append(m.links, req)
	return nil
}

func (m *fakeMailer) SendPlain(to []notification.Recipient, subject, body string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.plain = append(m.plain, sentMail{to: to, subject: subject, body: body})
	return nil
}

// fakeDrive answers only the "already processed" lookups; other calls panic.
type fakeDrive struct {
	distribution.DriveClient
	existing map[string]bool
	findErr  error
}

func (d *fakeDrive) FindFileByName(ctx context.Context, folderID, fileName string) (*distribution.FileInfo, error) {
	if d.findErr != nil {
		return nil, d.findErr
	}
	if d.existing[fileName] {
		return &distribution.FileInfo{Name: fileName}, nil
	}
	return nil, nil
}

const sundayVideo = "/test/source/2026-10-11 10-17-09.mp4"

func processTestConfig() *config.Config {
	return &config.Config{
		Paths: config.PathsConfig{SourceDirectory: "/test/source"},
		Email: config.EmailConfig{
			FromName:    "Test Church",
			FromAddress: "church@example.com",
			Recipients: map[string]config.RecipientConfig{
				"jane": {Name: "Jane Doe", Address: "jane@example.com"},
			},
		},
		Detection: config.DetectionConfig{Enabled: true},
	}
}

// newTestRun builds a run whose every seam succeeds: the newest video is the
// 2026-10-11 Sunday, nothing is on Drive yet, the cross is found at 00:14:07
// and the amen at 01:05:00, and the workflow succeeds. Tests break one seam.
func newTestRun(out *bytes.Buffer, mailer *fakeMailer) *processRun {
	tail := appprocess.NewLogTail(30)
	w := io.MultiWriter(out, tail)
	return &processRun{
		cfg:      processTestConfig(),
		input:    ProcessInput{RecipientKeys: []string{"jane"}},
		out:      w,
		tail:     tail,
		lockPath: "/tmp/test-process.lock",
		workDir:  "/home/church/nac-service-media",
		acquireLock: func() (func(), error) {
			return func() {}, nil
		},
		findNewest: func(dir string) (string, error) {
			return sundayVideo, nil
		},
		newDrive: func(ctx context.Context) (distribution.DriveClient, error) {
			return &fakeDrive{}, nil
		},
		newMailer: func(ctx context.Context) (processMailer, error) {
			return mailer, nil
		},
		detectStart: func(ctx context.Context, videoPath string) (string, error) {
			fmt.Fprintln(w, "Searching for cross lighting...")
			return "00:14:07", nil
		},
		detectEnd: func(ctx context.Context, videoPath string, startSeconds int) (string, error) {
			return "01:05:00", nil
		},
		workflow: func(ctx context.Context, driveClient distribution.DriveClient, m notification.EmailSender, input ProcessInput) error {
			return nil
		},
	}
}

func TestProcess_CrossNotFound_SendsOneFailureEmail(t *testing.T) {
	var out bytes.Buffer
	mailer := &fakeMailer{}
	run := newTestRun(&out, mailer)
	run.detectStart = func(ctx context.Context, videoPath string) (string, error) {
		fmt.Fprintln(run.out, "Searching for cross lighting...")
		return "", errors.New("auto-detection failed: could not detect cross lighting up in search range\nUse --start to specify manually")
	}

	err := run.run(context.Background())

	if err == nil {
		t.Fatal("expected the run to fail (exit non-zero)")
	}
	if len(mailer.plain) != 1 {
		t.Fatalf("expected exactly one failure email, got %d", len(mailer.plain))
	}
	mail := mailer.plain[0]
	if len(mail.to) != 1 || mail.to[0].Address != "jane@example.com" {
		t.Errorf("failure email should go to the run's --recipient, got %+v", mail.to)
	}
	if want := "Service recording NOT processed: 2026-10-11: cross not found"; mail.subject != want {
		t.Errorf("subject = %q, want %q", mail.subject, want)
	}
	for _, want := range []string{
		"could not detect cross lighting up in search range",
		sundayVideo,
		`nac-service-media process --input "` + sundayVideo + `" --start HH:MM:SS --end HH:MM:SS --recipient jane`,
		"cd \"/home/church/nac-service-media\"",
		"Searching for cross lighting...",
	} {
		if !strings.Contains(mail.body, want) {
			t.Errorf("failure email body missing %q\nbody:\n%s", want, mail.body)
		}
	}
}

func TestProcess_AmenNotFoundAfterCross_HandCommandCarriesStart(t *testing.T) {
	var out bytes.Buffer
	mailer := &fakeMailer{}
	run := newTestRun(&out, mailer)
	run.detectEnd = func(ctx context.Context, videoPath string, startSeconds int) (string, error) {
		return "", errors.New("end detection failed: could not detect amen: Video too short: 3884s, need at least 4320s\nUse --end to specify manually")
	}

	err := run.run(context.Background())

	if err == nil {
		t.Fatal("expected the run to fail (exit non-zero)")
	}
	if len(mailer.plain) != 1 {
		t.Fatalf("expected exactly one failure email, got %d", len(mailer.plain))
	}
	mail := mailer.plain[0]
	if want := "Service recording NOT processed: 2026-10-11: amen not found"; mail.subject != want {
		t.Errorf("subject = %q, want %q", mail.subject, want)
	}
	want := `nac-service-media process --input "` + sundayVideo + `" --start 00:14:07 --end HH:MM:SS --recipient jane`
	if !strings.Contains(mail.body, want) {
		t.Errorf("hand command should carry the found --start; want %q in\n%s", want, mail.body)
	}
}

func TestProcess_AlreadyProcessed_ExitsZeroAndSendsNothing(t *testing.T) {
	var out bytes.Buffer
	mailer := &fakeMailer{}
	run := newTestRun(&out, mailer)
	run.newDrive = func(ctx context.Context) (distribution.DriveClient, error) {
		return &fakeDrive{existing: map[string]bool{"2026-10-11.mp4": true, "2026-10-11.mp3": true}}, nil
	}

	err := run.run(context.Background())

	if err != nil {
		t.Fatalf("an already processed video is not a failure; got error %v", err)
	}
	if len(mailer.plain) != 0 || len(mailer.links) != 0 {
		t.Errorf("expected no email, got %d failure and %d links emails", len(mailer.plain), len(mailer.links))
	}
	if got := strings.Count(out.String(), "\n"); got != 1 {
		t.Errorf("expected one log line, got %d:\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), "already been processed") {
		t.Errorf("log line should say it was already processed, got %q", out.String())
	}
}

func TestProcess_LockHeld_ExitsNonZeroAndSaysSo(t *testing.T) {
	var out bytes.Buffer
	mailer := &fakeMailer{}
	run := newTestRun(&out, mailer)
	run.acquireLock = func() (func(), error) {
		return nil, lock.ErrAlreadyHeld
	}

	err := run.run(context.Background())

	if err == nil {
		t.Fatal("a run skipped because another holds the lock must exit non-zero")
	}
	for _, want := range []string{"another nac-service-media process run", "/tmp/test-process.lock", "did nothing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should contain %q", err.Error(), want)
		}
	}
	if len(mailer.plain) != 1 {
		t.Fatalf("expected one failure email for the skipped run, got %d", len(mailer.plain))
	}
}

func TestProcess_FailureEmailCannotBeSent_LogsLoudlyAndExitsNonZero(t *testing.T) {
	var out bytes.Buffer
	mailer := &fakeMailer{sendErr: errors.New("oauth2: token expired and refresh token is not set")}
	run := newTestRun(&out, mailer)
	run.detectStart = func(ctx context.Context, videoPath string) (string, error) {
		return "", errors.New("auto-detection failed: could not detect cross lighting up in search range")
	}

	err := run.run(context.Background())

	if err == nil {
		t.Fatal("expected the run to fail (exit non-zero)")
	}
	if !strings.Contains(err.Error(), "could not detect cross") {
		t.Errorf("the run should still return its own failure, got %v", err)
	}
	for _, want := range []string{"FAILURE EMAIL NOT SENT", "token expired"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("log should say loudly %q\nlog:\n%s", want, out.String())
		}
	}
}

func TestProcess_GmailSignInFails_LogsLoudlyAndExitsNonZero(t *testing.T) {
	var out bytes.Buffer
	run := newTestRun(&out, &fakeMailer{})
	run.input.StartTime = "00:14:07"
	run.input.EndTime = "01:05:00"
	run.newMailer = func(ctx context.Context) (processMailer, error) {
		return nil, errors.New("unable to get OAuth token: browser sign-in timed out")
	}
	calls := 0
	inner := run.newMailer
	run.newMailer = func(ctx context.Context) (processMailer, error) {
		calls++
		return inner(ctx)
	}

	err := run.run(context.Background())

	if err == nil {
		t.Fatal("expected the run to fail (exit non-zero)")
	}
	if !strings.Contains(out.String(), "FAILURE EMAIL NOT SENT") {
		t.Errorf("log should say loudly that no email went out\nlog:\n%s", out.String())
	}
	if calls != 1 {
		t.Errorf("Gmail sign-in should be tried once per run (a second try waits again), got %d", calls)
	}
}

func TestProcess_Success_SendsNoFailureEmail(t *testing.T) {
	var out bytes.Buffer
	mailer := &fakeMailer{}
	run := newTestRun(&out, mailer)

	if err := run.run(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mailer.plain) != 0 {
		t.Errorf("a successful run sends no failure email, got %d", len(mailer.plain))
	}
}

// TestProcess_EveryErrorPathSendsOneFailureEmail is the sweep: each way the
// process command can return an error goes through the one failure notifier.
func TestProcess_EveryErrorPathSendsOneFailureEmail(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name   string
		break_ func(r *processRun)
		reason string
	}{
		{"lock cannot be taken", func(r *processRun) {
			r.acquireLock = func() (func(), error) { return nil, boom }
		}, "could not take the run lock"},
		{"lock held by another run", func(r *processRun) {
			r.acquireLock = func() (func(), error) { return nil, lock.ErrAlreadyHeld }
		}, "another run is still going"},
		{"no video in the source folder", func(r *processRun) {
			r.findNewest = func(string) (string, error) { return "", errors.New("no video files found in /test/source") }
		}, "no video file found"},
		{"Drive sign-in fails at the already-processed check", func(r *processRun) {
			r.newDrive = func(context.Context) (distribution.DriveClient, error) { return nil, boom }
		}, "Google Drive sign-in failed"},
		{"Drive lookup fails at the already-processed check", func(r *processRun) {
			r.newDrive = func(context.Context) (distribution.DriveClient, error) { return &fakeDrive{findErr: boom}, nil }
		}, "Google Drive check failed"},
		{"detection off and no --start", func(r *processRun) {
			r.cfg.Detection.Enabled = false
		}, "no start time"},
		{"cross not found", func(r *processRun) {
			r.detectStart = func(context.Context, string) (string, error) { return "", boom }
		}, "cross not found"},
		{"cross search read no frame", func(r *processRun) {
			r.detectStart = func(context.Context, string) (string, error) {
				return "", fmt.Errorf("auto-detection failed: %w", fmt.Errorf("cross search could not run: 22 of 22 frames could not be read: %w", infradetection.ErrNothingAnalysed))
			}
		}, "cross search could not run"},
		{"detection off and no --end", func(r *processRun) {
			r.input.StartTime = "00:14:07"
			r.cfg.Detection.Enabled = false
		}, "no end time"},
		{"bad --start", func(r *processRun) {
			r.input.StartTime = "fourteen minutes"
		}, "bad start time"},
		{"amen not found", func(r *processRun) {
			r.detectEnd = func(context.Context, string, int) (string, error) { return "", boom }
		}, "amen not found"},
		{"amen search could not run", func(r *processRun) {
			r.detectEnd = func(context.Context, string, int) (string, error) {
				return "", fmt.Errorf("end detection failed: %w", fmt.Errorf("amen search could not run: Video too short (%w)", infradetection.ErrNothingAnalysed))
			}
		}, "amen search could not run"},
		{"Drive sign-in fails with --input given", func(r *processRun) {
			r.input.InputPath = sundayVideo
			r.newDrive = func(context.Context) (distribution.DriveClient, error) { return nil, boom }
		}, "Google Drive sign-in failed"},
		{"workflow fails (trim, extract, upload, links email, ...)", func(r *processRun) {
			r.workflow = func(context.Context, distribution.DriveClient, notification.EmailSender, ProcessInput) error {
				return errors.New("video upload failed: googleapi: Error 403: quota")
			}
		}, "video upload failed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			mailer := &fakeMailer{}
			run := newTestRun(&out, mailer)
			tc.break_(run)

			err := run.run(context.Background())

			if err == nil {
				t.Fatal("expected the run to fail (exit non-zero)")
			}
			if len(mailer.plain) != 1 {
				t.Fatalf("expected exactly one failure email, got %d", len(mailer.plain))
			}
			if !strings.HasSuffix(mailer.plain[0].subject, ": "+tc.reason) {
				t.Errorf("subject %q should end with reason %q", mailer.plain[0].subject, tc.reason)
			}
			if !strings.Contains(mailer.plain[0].body, "nac-service-media process --input ") {
				t.Errorf("body should carry a hand command\n%s", mailer.plain[0].body)
			}
		})
	}
}

func TestBrowserAuthTimeout_BoundedWhenNotATerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "log")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if got := browserAuthTimeout(f, f); got != browserAuthWait {
		t.Errorf("a run writing to a file (the scheduled task's log) should bound the sign-in wait; got %s", got)
	}
}

func TestProcess_CrossMiss_EmailNamesTheSampledFramesAndTemplatesAdd(t *testing.T) {
	var out bytes.Buffer
	mailer := &fakeMailer{}
	run := newTestRun(&out, mailer)
	framesDir := "/test/source/2026-10-11 10-17-09 cross frames"
	run.detectStart = func(ctx context.Context, videoPath string) (string, error) {
		miss := &infradetection.CrossNotFoundError{Analysed: 62, FramesDir: framesDir, FramesKept: 62}
		return "", fmt.Errorf("auto-detection failed: %w\nUse --start to specify manually", miss)
	}

	err := run.run(context.Background())

	if err == nil || !strings.Contains(err.Error(), framesDir) {
		t.Fatalf("the run's error should name the frames folder, got %v", err)
	}
	if len(mailer.plain) != 1 {
		t.Fatalf("expected exactly one failure email, got %d", len(mailer.plain))
	}
	mail := mailer.plain[0]
	if !strings.HasSuffix(mail.subject, ": cross not found") {
		t.Errorf("subject = %q, want it to end with 'cross not found'", mail.subject)
	}
	for _, want := range []string{
		framesDir,
		`nac-service-media templates add --input "` + sundayVideo + `" --lit-at HH:MM:SS --unlit-at HH:MM:SS --name`,
	} {
		if !strings.Contains(mail.body, want) {
			t.Errorf("failure email body missing %q\nbody:\n%s", want, mail.body)
		}
	}
}

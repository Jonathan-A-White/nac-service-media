package watch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// --- fakes ---

type fakeAlerts struct {
	alerts  []Alert
	err     error
	claimed []string
	claimEr error
}

func (f *fakeAlerts) List(ctx context.Context) ([]Alert, error) { return f.alerts, f.err }
func (f *fakeAlerts) Claim(ctx context.Context, id string) error {
	f.claimed = append(f.claimed, id)
	return f.claimEr
}

type fakePC struct {
	out    string
	err    error
	called []string
}

func (f *fakePC) Run(ctx context.Context, script string) (string, error) {
	f.called = append(f.called, script)
	return f.out, f.err
}

type fakeMail struct {
	subjects []string
	bodies   []string
	err      error
}

func (f *fakeMail) SendMayor(ctx context.Context, subject, body string) error {
	f.subjects = append(f.subjects, subject)
	f.bodies = append(f.bodies, body)
	return f.err
}

const sunday = "2026-11-15"

// report builds what the remote script prints.
func report(recordings, logs, markers []string, tail string) string {
	var b strings.Builder
	b.WriteString("@@recordings\n")
	for _, r := range recordings {
		b.WriteString(r + "\n")
	}
	b.WriteString("@@logs\n")
	for _, l := range logs {
		b.WriteString(l + "\n")
	}
	b.WriteString("@@markers\n")
	for _, m := range markers {
		b.WriteString(m + "\n")
	}
	b.WriteString("@@tail\n" + tail + "\n@@end\n")
	return b.String()
}

const (
	rec = "1503210496 /mnt/d/Videos/2026-11-15 10-06-16.mp4"
	lg  = "/home/x/src/nac-service-media/logs/scheduled-run-2026-11-15_12-30-02.log"
)

func newService(a *fakeAlerts, pc *fakePC, m *fakeMail, out *bytes.Buffer) *Service {
	return &Service{
		Alerts: a, PC: pc, Mailer: m, Out: out,
		VideosDir: "/mnt/d/Videos", RepoDir: "~/src/nac-service-media",
	}
}

func ny(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestRun_AlertFoundClaimsLogsOneLineAndNothingElse(t *testing.T) {
	loc := ny(t)
	a := &fakeAlerts{alerts: []Alert{
		{ID: "a0", Kind: "charge", When: time.Date(2026, 11, 15, 0, 0, 0, 0, loc)},
		{ID: "a1", Kind: "service-recording", When: time.Date(2026, 11, 8, 0, 0, 0, 0, loc)},
		{ID: "a2", Kind: "service-recording", When: time.Date(2026, 11, 15, 0, 0, 0, 0, loc)},
	}}
	pc, m := &fakePC{}, &fakeMail{}
	var out bytes.Buffer
	res, err := newService(a, pc, m, &out).Run(context.Background(), sunday)
	if err != nil {
		t.Fatal(err)
	}
	if res.Class != Found {
		t.Fatalf("class = %q, want found", res.Class)
	}
	if len(a.claimed) != 1 || a.claimed[0] != "a2" {
		t.Errorf("claimed %v, want [a2]", a.claimed)
	}
	if len(pc.called) != 0 || len(m.subjects) != 0 {
		t.Errorf("found must not look at the PC or mail anyone: pc=%d mail=%d", len(pc.called), len(m.subjects))
	}
	if n := strings.Count(strings.TrimRight(out.String(), "\n"), "\n") + 1; n != 1 {
		t.Errorf("want one log line, got %d: %q", n, out.String())
	}
}

func TestRun_AlertWhenInEasternNotUTC(t *testing.T) {
	// midnight Eastern is 05:00Z; a UTC reading of the date would still be the 15th,
	// but 23:00 Eastern on the 14th is the 15th in UTC and must not match.
	loc := ny(t)
	a := &fakeAlerts{alerts: []Alert{{ID: "late", Kind: "service-recording", When: time.Date(2026, 11, 14, 23, 0, 0, 0, loc)}}}
	pc := &fakePC{out: report(nil, nil, nil, "")}
	var out bytes.Buffer
	res, err := newService(a, pc, &fakeMail{}, &out).Run(context.Background(), sunday)
	if err != nil {
		t.Fatal(err)
	}
	if res.Class == Found {
		t.Fatal("an alert for the 14th Eastern must not count for the 15th")
	}
}

func TestRun_ClassesFromThePCsReport(t *testing.T) {
	tests := []struct {
		name     string
		out      string
		err      error
		want     Class
		mailed   bool
		contains []string
	}{
		{
			name: "no recording and no run log: the PC answers, quiet",
			out:  report(nil, nil, nil, ""),
			want: NoRecording,
		},
		{
			name: "no recording and the run said no new recording: quiet",
			out: report(nil, []string{lg},
				[]string{lg + ":12:No new service recording since 2026-11-08", lg + ":20:[2026-11-15 12:31:00] Exit code: 1", lg + ":21:[2026-11-15 12:31:00] FAILURE: Processing failed with exit code 1"},
				"No new service recording since 2026-11-08"),
			want: NoRecording,
		},
		{
			name: "no recording and the run said already processed, nothing to do: quiet",
			out: report(nil, []string{lg},
				[]string{lg + ":9:Most recent file (2026-11-08) has already been processed; nothing to do.", lg + ":10:[2026-11-15 12:31:00] Exit code: 0", lg + ":11:[2026-11-15 12:31:00] SUCCESS: Processing completed successfully"},
				"nothing to do"),
			want: NoRecording,
		},
		{
			name: "no recording but the run failed some other way: other",
			out: report(nil, []string{lg},
				[]string{lg + ":10:[2026-11-15 12:31:00] Exit code: 1", lg + ":11:[2026-11-15 12:31:00] FAILURE: Processing failed with exit code 1"},
				"google: token expired"),
			want:     Other,
			mailed:   true,
			contains: []string{"google: token expired"},
		},
		{
			name: "no recording, the stale run's no-new-recording mail did not go out: other, not quiet",
			out: report(nil, []string{lg},
				[]string{lg + ":3:!!! NO-NEW-RECORDING EMAIL NOT SENT: Gmail sign-in failed: token expired", lg + ":9:No new service recording since 2026-11-08", lg + ":10:[2026-11-15 12:31:00] Exit code: 1"},
				"!!! NO-NEW-RECORDING EMAIL NOT SENT: Gmail sign-in failed: token expired"),
			want:     Other,
			mailed:   true,
			contains: []string{"NO-NEW-RECORDING EMAIL NOT SENT", "nobody has been told"},
		},
		{
			name: "a recording, the run failed and its failure email did not go out: run-failed names it",
			out: report([]string{rec}, []string{lg},
				[]string{lg + ":3:!!! FAILURE EMAIL NOT SENT: Gmail sign-in failed: token expired", lg + ":30:[2026-11-15 12:50:00] Exit code: 1"},
				""),
			want:     RunFailed,
			mailed:   true,
			contains: []string{"Class: run-failed: a recording exists and the run failed (exit code 1), and the run's log says \"FAILURE EMAIL NOT SENT: Gmail sign-in failed: token expired\""},
		},
		{
			name:     "ssh cannot reach the PC",
			err:      &Unreachable{Detail: "ssh: connect to host 10.88.0.9 port 22: Connection timed out"},
			want:     PCUnreachable,
			mailed:   true,
			contains: []string{"Connection timed out"},
		},
		{
			name:     "a recording and no run log: run-failed",
			out:      report([]string{rec}, nil, nil, ""),
			want:     RunFailed,
			mailed:   true,
			contains: []string{"/mnt/d/Videos/2026-11-15 10-06-16.mp4", "1503210496", "no run log"},
		},
		{
			name: "a recording and the run errored: run-failed with the log tail",
			out: report([]string{rec}, []string{lg},
				[]string{lg + ":30:[2026-11-15 12:50:00] Exit code: 1", lg + ":31:[2026-11-15 12:50:00] FAILURE: Processing failed with exit code 1"},
				"Error: cross not found in 3 pictures"),
			want:     RunFailed,
			mailed:   true,
			contains: []string{"cross not found", lg, "1503210496"},
		},
		{
			name: "a recording, the run succeeded, no mail sent: mail-not-sent",
			out: report([]string{rec}, []string{lg},
				[]string{lg + ":30:[2026-11-15 12:50:00] Exit code: 0", lg + ":31:[2026-11-15 12:50:00] SUCCESS: Processing completed successfully"},
				"Done! Completed in 12m"),
			want:     MailNotSent,
			mailed:   true,
			contains: []string{"Done! Completed"},
		},
		{
			name: "a recording, the run succeeded and sent mail, Postern has none: other",
			out: report([]string{rec}, []string{lg},
				[]string{lg + ":28:      Sent to: Jonathan <j@example.org>", lg + ":30:[2026-11-15 12:50:00] Exit code: 0", lg + ":31:[2026-11-15 12:50:00] SUCCESS: Processing completed successfully"},
				"Done!"),
			want:     Other,
			mailed:   true,
			contains: []string{"Sent to"},
		},
		{
			name: "a recording and a run that never wrote its exit code: other",
			out: report([]string{rec}, []string{lg},
				[]string{}, "[7/7] Sending email..."),
			want:     Other,
			mailed:   true,
			contains: []string{"did not finish"},
		},
		{
			name:     "a report cut short: other",
			out:      "@@recordings\n",
			want:     Other,
			mailed:   true,
			contains: []string{"@@recordings"},
		},
		{
			name:     "the ssh runner failed some other way: other",
			err:      errors.New("exit status 127: sh: stat: not found"),
			want:     Other,
			mailed:   true,
			contains: []string{"exit status 127"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, pc, m := &fakeAlerts{}, &fakePC{out: tt.out, err: tt.err}, &fakeMail{}
			var out bytes.Buffer
			res, err := newService(a, pc, m, &out).Run(context.Background(), sunday)
			if err != nil {
				t.Fatal(err)
			}
			if res.Class != tt.want {
				t.Fatalf("class = %q, want %q\n%s", res.Class, tt.want, out.String())
			}
			if tt.mailed {
				if len(m.subjects) != 1 {
					t.Fatalf("want ONE mail, got %d", len(m.subjects))
				}
				wantSubj := fmt.Sprintf("Church Sunday watch %s: %s", sunday, tt.want)
				if m.subjects[0] != wantSubj {
					t.Errorf("subject = %q, want %q", m.subjects[0], wantSubj)
				}
				for _, c := range tt.contains {
					if !strings.Contains(m.bodies[0], c) {
						t.Errorf("mail body lacks %q:\n%s", c, m.bodies[0])
					}
				}
			} else {
				if len(m.subjects) != 0 {
					t.Errorf("a quiet class sent mail: %v", m.subjects)
				}
				if n := strings.Count(strings.TrimRight(out.String(), "\n"), "\n") + 1; n != 1 {
					t.Errorf("a quiet class logs one line, got %d: %q", n, out.String())
				}
			}
			if len(pc.called) != 1 {
				t.Errorf("want exactly one ssh call, got %d", len(pc.called))
			}
		})
	}
}

func TestRun_DryRunPrintsTheMailAndSendsNone(t *testing.T) {
	a, pc, m := &fakeAlerts{}, &fakePC{err: &Unreachable{Detail: "timed out"}}, &fakeMail{}
	var out bytes.Buffer
	s := newService(a, pc, m, &out)
	s.DryRun = true
	res, err := s.Run(context.Background(), sunday)
	if err != nil {
		t.Fatal(err)
	}
	if res.Class != PCUnreachable || len(m.subjects) != 0 {
		t.Fatalf("class %q, mails %d", res.Class, len(m.subjects))
	}
	if !strings.Contains(out.String(), "Church Sunday watch 2026-11-15: pc-unreachable") || !strings.Contains(out.String(), "timed out") {
		t.Errorf("dry run did not print the mail:\n%s", out.String())
	}
}

func TestRun_DryRunDoesNotClaim(t *testing.T) {
	loc := ny(t)
	a := &fakeAlerts{alerts: []Alert{{ID: "a2", Kind: "service-recording", When: time.Date(2026, 11, 15, 0, 0, 0, 0, loc)}}}
	s := newService(a, &fakePC{}, &fakeMail{}, &bytes.Buffer{})
	s.DryRun = true
	res, err := s.Run(context.Background(), sunday)
	if err != nil || res.Class != Found {
		t.Fatalf("%v %v", res, err)
	}
	if len(a.claimed) != 0 {
		t.Error("a dry run must not claim")
	}
}

func TestRun_PosternDownStillLooksAtThePC(t *testing.T) {
	// Postern failing says nothing about the church: look anyway. With no
	// recording that is quiet; with a clean run that sent its mail it is not,
	// because the watch could not confirm the alert.
	a := &fakeAlerts{err: errors.New("asking Postern: connection refused")}

	pc, m := &fakePC{out: report(nil, nil, nil, "")}, &fakeMail{}
	var out bytes.Buffer
	res, err := newService(a, pc, m, &out).Run(context.Background(), sunday)
	if err != nil || res.Class != NoRecording || len(m.subjects) != 0 {
		t.Fatalf("no recording: %v %v mails=%d", res.Class, err, len(m.subjects))
	}
	if !strings.Contains(out.String(), "connection refused") {
		t.Errorf("the quiet line should still mention Postern's error: %q", out.String())
	}

	pc, m = &fakePC{out: report([]string{rec}, []string{lg},
		[]string{lg + ":28:      Sent to: J <j@example.org>", lg + ":30:Exit code: 0"}, "Done!")}, &fakeMail{}
	res, err = newService(a, pc, m, &bytes.Buffer{}).Run(context.Background(), sunday)
	if err != nil || res.Class != Other || len(m.subjects) != 1 {
		t.Fatalf("clean run, Postern down: %v %v mails=%d", res.Class, err, len(m.subjects))
	}
	if !strings.Contains(m.bodies[0], "connection refused") {
		t.Errorf("mail must carry Postern's error:\n%s", m.bodies[0])
	}
}

func TestRun_MailFailureIsAnError(t *testing.T) {
	m := &fakeMail{err: errors.New("mw: MW_SEAT unset")}
	_, err := newService(&fakeAlerts{}, &fakePC{err: &Unreachable{Detail: "x"}}, m, &bytes.Buffer{}).Run(context.Background(), sunday)
	if err == nil || !strings.Contains(err.Error(), "MW_SEAT") {
		t.Fatalf("err = %v", err)
	}
}

func TestRun_BadDate(t *testing.T) {
	_, err := newService(&fakeAlerts{}, &fakePC{}, &fakeMail{}, &bytes.Buffer{}).Run(context.Background(), "11/15/2026")
	if err == nil {
		t.Fatal("want an error for a date not YYYY-MM-DD")
	}
}

func TestRemoteScript_IsReadOnlyAndNamesTheDay(t *testing.T) {
	s := RemoteScript(sunday, "/mnt/d/Videos", "~/src/nac-service-media")
	for _, want := range []string{"2026-11-15", "/mnt/d/Videos", "$HOME/src/nac-service-media", "@@end"} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %q:\n%s", want, s)
		}
	}
	bare := strings.ReplaceAll(s, "2>/dev/null", "")
	for _, bad := range []string{"rm ", "mv ", "cp ", "tee ", "chmod", "kill", "sudo", ">"} {
		if strings.Contains(bare, bad) {
			t.Errorf("script is not read-only: has %q", bad)
		}
	}
}

func TestRemoteScript_QuotesAnOddPath(t *testing.T) {
	s := RemoteScript(sunday, "/mnt/d/My Videos'; rm -rf /", "~/src/x")
	if !strings.Contains(s, `My Videos'\''; rm -rf /'`) {
		t.Errorf("a quote in a path must be escaped:\n%s", s)
	}
}

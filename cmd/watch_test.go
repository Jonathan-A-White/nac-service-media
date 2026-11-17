package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appwatch "nac-service-media/application/watch"
	domain "nac-service-media/domain/watch"
)

func TestServiceDateDefaultsToTodayInNewYork(t *testing.T) {
	// 02:00Z on Monday the 16th is still Sunday evening the 15th in New York.
	now := time.Date(2026, 11, 16, 2, 0, 0, 0, time.UTC)
	got, err := serviceDate("", false, false, now)
	if err != nil || got != "2026-11-15" {
		t.Fatalf("%q %v", got, err)
	}
	got, err = serviceDate("2026-11-08", false, false, now)
	if err != nil || got != "2026-11-08" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := serviceDate("next sunday", false, false, now); err == nil {
		t.Fatal("want an error for a bad --date")
	}
}

func TestServiceDateSundayIsTheLatestSundayInNewYork(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	for _, c := range []struct {
		now  time.Time
		want string
	}{
		{time.Date(2026, 11, 15, 13, 0, 0, 0, loc), "2026-11-15"}, // Sunday itself
		{time.Date(2026, 11, 16, 9, 30, 0, 0, loc), "2026-11-15"}, // Monday catch-up
		{time.Date(2026, 11, 21, 23, 0, 0, 0, loc), "2026-11-15"}, // Saturday night
		{time.Date(2026, 11, 16, 2, 0, 0, 0, time.UTC), "2026-11-15"},
	} {
		got, err := serviceDate("", true, false, c.now)
		if err != nil || got != c.want {
			t.Errorf("%v: %q %v, want %q", c.now, got, err, c.want)
		}
	}
	if _, err := serviceDate("2026-11-15", true, false, time.Now()); err == nil {
		t.Error("--date with --sunday must be refused")
	}
}

func TestServiceDateWednesdayIsTheLatestWednesdayInNewYork(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	for _, c := range []struct {
		now  time.Time
		want string
	}{
		{time.Date(2026, 11, 18, 22, 30, 0, 0, loc), "2026-11-18"},    // Wednesday itself
		{time.Date(2026, 11, 19, 9, 30, 0, 0, loc), "2026-11-18"},     // Thursday catch-up
		{time.Date(2026, 11, 17, 12, 0, 0, 0, loc), "2026-11-11"},     // Tuesday: last week's
		{time.Date(2026, 11, 19, 3, 0, 0, 0, time.UTC), "2026-11-18"}, // Wednesday evening in New York
		{time.Date(2026, 11, 15, 13, 0, 0, 0, loc), "2026-11-11"},     // Sunday
	} {
		got, err := serviceDate("", false, true, c.now)
		if err != nil || got != c.want {
			t.Errorf("%v: %q %v, want %q", c.now, got, err, c.want)
		}
	}
}

func TestServiceDateRefusesTwoOfDateSundayWednesday(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		date         string
		sunday, wedn bool
	}{
		{"2026-11-18", false, true},
		{"2026-11-15", true, false},
		{"", true, true},
		{"2026-11-18", true, true},
	} {
		if got, err := serviceDate(c.date, c.sunday, c.wedn, now); err == nil {
			t.Errorf("date=%q sunday=%v wednesday=%v: want a refusal, got %q", c.date, c.sunday, c.wedn, got)
		}
	}
}

// aliveAlerts holds one service-recording alert for one date.
type aliveAlerts struct{ day time.Time }

func (a aliveAlerts) List(context.Context) ([]domain.Alert, error) {
	return []domain.Alert{{ID: "a1", Kind: appwatch.KindServiceRecording, When: a.day}}, nil
}
func (aliveAlerts) Claim(context.Context, string) error { return nil }

type fixedPC struct{ out string }

func (f fixedPC) Run(context.Context, string) (string, error) { return f.out, nil }

// A Wednesday date goes through each class, as a Sunday does.
func TestRunWatchWithDependencies_WednesdayDateThroughEachClass(t *testing.T) {
	const wed = "2026-11-18"
	loc, _ := time.LoadLocation("America/New_York")
	noRec := "@@recordings\n@@logs\n@@markers\n@@tail\n@@end\n"
	badRun := "@@recordings\n1000 /mnt/d/Videos/2026-11-18 20-00-00.mkv\n@@logs\n/r/logs/scheduled-run-2026-11-18_21-30-00.log\n" +
		"@@markers\n/r/logs/scheduled-run-2026-11-18_21-30-00.log:9:Exit code: 1\n@@tail\n@@end\n"
	for _, c := range []struct {
		name   string
		alerts domain.Alerts
		pc     domain.PC
		want   string
	}{
		{"alert found", aliveAlerts{time.Date(2026, 11, 18, 22, 0, 0, 0, loc)}, downPC{}, "found service-recording alert"},
		{"no recording made", noAlerts{}, fixedPC{noRec}, "no-recording"},
		{"something wrong", noAlerts{}, fixedPC{badRun}, "run-failed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var out bytes.Buffer
			m := &countMail{}
			err := RunWatchWithDependencies(context.Background(), WatchOptions{Date: wed, DryRun: true}, c.alerts, c.pc, m, &out)
			if err != nil {
				t.Fatal(err)
			}
			if m.n != 0 || !strings.Contains(out.String(), c.want) || !strings.Contains(out.String(), wed) {
				t.Errorf("mails=%d out=%q", m.n, out.String())
			}
			if strings.Contains(out.String(), "Sunday") {
				t.Errorf("a Wednesday watch says Sunday: %q", out.String())
			}
		})
	}
}

type noAlerts struct{}

func (noAlerts) List(context.Context) ([]domain.Alert, error) { return nil, nil }
func (noAlerts) Claim(context.Context, string) error          { return nil }

type downPC struct{}

func (downPC) Run(context.Context, string) (string, error) {
	return "", &domain.Unreachable{Detail: "Connection timed out"}
}

type countMail struct{ n int }

func (c *countMail) SendMayor(context.Context, string, string) error { c.n++; return nil }

func TestRunWatchWithDependencies_DryRunSendsNoMail(t *testing.T) {
	var out bytes.Buffer
	m := &countMail{}
	err := RunWatchWithDependencies(context.Background(), WatchOptions{Date: "2026-11-15", DryRun: true}, noAlerts{}, downPC{}, m, &out)
	if err != nil {
		t.Fatal(err)
	}
	if m.n != 0 || !strings.Contains(out.String(), "pc-unreachable") {
		t.Errorf("mails=%d out=%q", m.n, out.String())
	}
}

func TestRunWatchWithDependencies_SendsOneMail(t *testing.T) {
	var out bytes.Buffer
	m := &countMail{}
	if err := RunWatchWithDependencies(context.Background(), WatchOptions{Date: "2026-11-15"}, noAlerts{}, downPC{}, m, &out); err != nil {
		t.Fatal(err)
	}
	if m.n != 1 {
		t.Errorf("mails = %d", m.n)
	}
}

func TestRunWatchWithDependencies_BadDateIsAnError(t *testing.T) {
	err := RunWatchWithDependencies(context.Background(), WatchOptions{Date: "x"}, noAlerts{}, downPC{}, &countMail{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("want error")
	}
}

type failingAlerts struct{}

func (failingAlerts) List(context.Context) ([]domain.Alert, error) {
	return nil, errors.New("no Postern settings")
}
func (failingAlerts) Claim(context.Context, string) error { return nil }

func TestUnavailableAlertsStillLetsThePCBeChecked(t *testing.T) {
	var out bytes.Buffer
	m := &countMail{}
	if err := RunWatchWithDependencies(context.Background(), WatchOptions{Date: "2026-11-15"}, failingAlerts{}, downPC{}, m, &out); err != nil {
		t.Fatal(err)
	}
	if m.n != 1 {
		t.Errorf("mails = %d", m.n)
	}
	_ = appwatch.Found
}

// Each timer's OnCalendar parses in systemd-analyze, and each service runs the
// watch for its own day.
func TestWatchTimersParseInSystemdAnalyze(t *testing.T) {
	analyze, lookErr := exec.LookPath("systemd-analyze")
	for _, c := range []struct{ unit, calendar, flag string }{
		{"nac-service-media-watch", "Sun *-*-* 13:00:00 America/New_York", "watch --sunday"},
		{"nac-service-media-watch-wednesday", "Wed *-*-* 22:30:00 America/New_York", "watch --wednesday"},
	} {
		timer, err := os.ReadFile(filepath.Join("..", "scripts", "systemd", c.unit+".timer"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(timer), "OnCalendar="+c.calendar+"\n") || !strings.Contains(string(timer), "Persistent=true") {
			t.Errorf("%s.timer: want OnCalendar=%s and Persistent=true:\n%s", c.unit, c.calendar, timer)
		}
		service, err := os.ReadFile(filepath.Join("..", "scripts", "systemd", c.unit+".service"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(service), "nac-service-media "+c.flag+"\n") {
			t.Errorf("%s.service does not run '%s':\n%s", c.unit, c.flag, service)
		}
		if lookErr != nil {
			continue
		}
		if out, err := exec.Command(analyze, "calendar", c.calendar).CombinedOutput(); err != nil {
			t.Errorf("systemd-analyze calendar %q: %v\n%s", c.calendar, err, out)
		}
	}
	if lookErr != nil {
		t.Skip("systemd-analyze is not on PATH: the OnCalendar lines were compared, not parsed")
	}
}

package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	appwatch "nac-service-media/application/watch"
	domain "nac-service-media/domain/watch"
)

func TestServiceDateDefaultsToTodayInNewYork(t *testing.T) {
	// 02:00Z on Monday the 16th is still Sunday evening the 15th in New York.
	now := time.Date(2026, 11, 16, 2, 0, 0, 0, time.UTC)
	got, err := serviceDate("", false, now)
	if err != nil || got != "2026-11-15" {
		t.Fatalf("%q %v", got, err)
	}
	got, err = serviceDate("2026-11-08", false, now)
	if err != nil || got != "2026-11-08" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := serviceDate("next sunday", false, now); err == nil {
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
		got, err := serviceDate("", true, c.now)
		if err != nil || got != c.want {
			t.Errorf("%v: %q %v, want %q", c.now, got, err, c.want)
		}
	}
	if _, err := serviceDate("2026-11-15", true, time.Now()); err == nil {
		t.Error("--date with --sunday must be refused")
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

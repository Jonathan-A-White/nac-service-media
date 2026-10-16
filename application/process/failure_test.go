package process

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestLogTail_KeepsLastLines(t *testing.T) {
	tail := NewLogTail(3)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(tail, "line %d\n", i)
	}
	fmt.Fprint(tail, "unfinished")

	got := strings.Join(tail.Lines(), "|")
	if want := "line 4|line 5|unfinished"; got != want {
		t.Errorf("Lines() = %q, want %q", got, want)
	}
}

func TestLogTail_JoinsLinesSplitAcrossWrites(t *testing.T) {
	tail := NewLogTail(30)
	fmt.Fprint(tail, "Searching for ")
	fmt.Fprint(tail, "cross\r\nnext\n")

	got := strings.Join(tail.Lines(), "|")
	if want := "Searching for cross|next"; got != want {
		t.Errorf("Lines() = %q, want %q", got, want)
	}
}

func TestFailure_HandCommandCarriesTheRunsFlags(t *testing.T) {
	f := Failure{
		VideoPath: "/src/2026-10-11 10-17-09.mp4",
		StartTime: "00:14:07",
		EndTime:   "01:05:00",
		Input: Input{
			RecipientKeys: []string{"jane", "john"},
			MinisterKey:   "smith",
			CCKeys:        []string{"pastor"},
			SenderKey:     "avteam",
			DateOverride:  "2026-10-11",
			SkipVideo:     true,
		},
	}
	want := `nac-service-media process --input "/src/2026-10-11 10-17-09.mp4" --start 00:14:07 --end 01:05:00 --recipient jane --recipient john --minister smith --cc pastor --sender avteam --date 2026-10-11 --skip-video`
	if got := f.HandCommand(); got != want {
		t.Errorf("HandCommand() =\n%s\nwant\n%s", got, want)
	}
}

func TestFailure_HandCommandWithoutVideoUsesPlaceholder(t *testing.T) {
	f := Failure{Input: Input{RecipientKeys: []string{"jane"}}}
	want := `nac-service-media process --input "<video file>" --start HH:MM:SS --end HH:MM:SS --recipient jane`
	if got := f.HandCommand(); got != want {
		t.Errorf("HandCommand() = %s, want %s", got, want)
	}
}

func TestFailure_SubjectFallsBackToRunDate(t *testing.T) {
	when := time.Date(2026, 10, 18, 12, 31, 0, 0, time.UTC)
	f := Failure{Reason: "no video file found", When: when}
	if got, want := f.Subject(), "Service recording NOT processed: 2026-10-18: no video file found"; got != want {
		t.Errorf("Subject() = %q, want %q", got, want)
	}
	f.ServiceDate = time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC)
	if got, want := f.Subject(), "Service recording NOT processed: 2026-10-11: no video file found"; got != want {
		t.Errorf("Subject() = %q, want %q", got, want)
	}
}

func TestFailure_BodySaysWhatFailedAndCarriesTheLog(t *testing.T) {
	f := Failure{
		Reason:    "Google Drive sign-in failed",
		Hint:      "sign in to Google again",
		Err:       errors.New("failed to create Google Drive client: browser sign-in timed out"),
		When:      time.Date(2026, 10, 18, 12, 31, 0, 0, time.UTC),
		VideoPath: "/src/2026-10-18 10-02-00.mp4",
		Input:     Input{RecipientKeys: []string{"jane"}},
		WorkDir:   "/home/church/nac-service-media",
		LogTail:   []string{"Opening browser for Google authentication..."},
	}
	body := f.Body()
	for _, want := range []string{
		"NOT processed",
		"What failed: Google Drive sign-in failed",
		"browser sign-in timed out",
		"Video file: /src/2026-10-18 10-02-00.mp4",
		"First: sign in to Google again",
		`cd "/home/church/nac-service-media"`,
		`nac-service-media process --input "/src/2026-10-18 10-02-00.mp4"`,
		"--start is when the cross lights up",
		"  Opening browser for Google authentication...",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q\n%s", want, body)
		}
	}
}

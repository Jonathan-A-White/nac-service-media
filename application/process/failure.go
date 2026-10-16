package process

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"nac-service-media/domain/notification"
)

// LogTail keeps the last lines written to it, so a failure email can show
// what the run printed just before it stopped.
type LogTail struct {
	mu      sync.Mutex
	max     int
	lines   []string
	partial string
}

// NewLogTail returns a LogTail that keeps the last max lines.
func NewLogTail(max int) *LogTail {
	return &LogTail{max: max}
}

// Write implements io.Writer.
func (t *LogTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	parts := strings.Split(t.partial+string(p), "\n")
	t.partial = parts[len(parts)-1]
	for _, line := range parts[:len(parts)-1] {
		t.lines = append(t.lines, strings.TrimRight(line, "\r"))
	}
	if len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
	return len(p), nil
}

// Lines returns the kept lines, oldest first, with any unfinished last line.
func (t *LogTail) Lines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := append([]string(nil), t.lines...)
	if t.partial != "" {
		lines = append(lines, t.partial)
	}
	if len(lines) > t.max {
		lines = lines[len(lines)-t.max:]
	}
	return lines
}

// FailureMailer sends the plain email that tells the recipient a run failed.
type FailureMailer interface {
	SendPlain(to []notification.Recipient, subject, body string) error
}

// Failure is what a process run knew when it stopped, for the failure email.
type Failure struct {
	Reason      string    // short, for the subject, e.g. "cross not found"
	Hint        string    // optional plain advice to do first
	Err         error     // the error the run stopped with
	When        time.Time // when the run stopped
	ServiceDate time.Time // zero when not known
	VideoPath   string    // empty when no video was found
	StartTime   string    // found or given; empty when not known
	EndTime     string    // found or given; empty when not known
	Input       Input     // the run's own flags
	WorkDir     string    // where the hand command must run (config/ is relative)
	LogTail     []string  // the last lines the run printed
}

// placeholderTime stands for a time the run did not find.
const placeholderTime = "HH:MM:SS"

// SendFailureEmail sends one email saying the run failed and how to finish it.
func SendFailureEmail(mailer FailureMailer, to []notification.Recipient, f Failure) error {
	return mailer.SendPlain(to, f.Subject(), f.Body())
}

// Subject is "Service recording NOT processed: <date>: <short reason>".
func (f Failure) Subject() string {
	date := f.ServiceDate
	if date.IsZero() {
		date = f.When
	}
	return fmt.Sprintf("Service recording NOT processed: %s: %s", date.Format("2006-01-02"), f.Reason)
}

// HandCommand is the process command that finishes the run by hand, with every
// value the run already knew filled in.
func (f Failure) HandCommand() string {
	videoPath := f.VideoPath
	if videoPath == "" {
		videoPath = "<video file>"
	}
	start := f.StartTime
	if start == "" {
		start = placeholderTime
	}
	end := f.EndTime
	if end == "" {
		end = placeholderTime
	}

	parts := []string{"nac-service-media process", fmt.Sprintf("--input %q", videoPath), "--start " + start, "--end " + end}
	for _, key := range f.Input.RecipientKeys {
		parts = append(parts, "--recipient "+key)
	}
	if f.Input.MinisterKey != "" {
		parts = append(parts, "--minister "+f.Input.MinisterKey)
	}
	for _, key := range f.Input.CCKeys {
		parts = append(parts, "--cc "+key)
	}
	if f.Input.SenderKey != "" {
		parts = append(parts, "--sender "+f.Input.SenderKey)
	}
	if f.Input.DateOverride != "" {
		parts = append(parts, "--date "+f.Input.DateOverride)
	}
	if f.Input.SkipVideo {
		parts = append(parts, "--skip-video")
	}
	return strings.Join(parts, " ")
}

// Body says in plain words what failed, the video it used, how to finish it
// by hand, and the last lines of the log.
func (f Failure) Body() string {
	var b strings.Builder
	b.WriteString("The service recording was NOT processed. This run sent no links email.\n\n")
	fmt.Fprintf(&b, "What failed: %s\n", f.Reason)
	if f.Err != nil {
		b.WriteString("Details:\n")
		for _, line := range strings.Split(strings.TrimSpace(f.Err.Error()), "\n") {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	if f.VideoPath != "" {
		fmt.Fprintf(&b, "Video file: %s\n", f.VideoPath)
	} else {
		b.WriteString("Video file: none found\n")
	}
	fmt.Fprintf(&b, "When: %s\n\n", f.When.Format("2006-01-02 15:04 MST"))

	if f.Hint != "" {
		fmt.Fprintf(&b, "First: %s\n\n", f.Hint)
	}

	b.WriteString("To finish it by hand, open WSL on the church PC and run:\n\n")
	if f.WorkDir != "" {
		fmt.Fprintf(&b, "  cd %q\n", f.WorkDir)
	}
	fmt.Fprintf(&b, "  %s\n\n", f.HandCommand())

	switch {
	case f.StartTime == "" && f.EndTime == "":
		b.WriteString("Replace each HH:MM:SS with a time from the video: --start is when the cross lights up, --end is the end of the three-fold amen.\n\n")
	case f.EndTime == "":
		b.WriteString("The start (the cross lighting up) was found and is filled in. Replace HH:MM:SS with the time the three-fold amen ends.\n\n")
	case f.StartTime == "":
		b.WriteString("Replace HH:MM:SS with the time the cross lights up.\n\n")
	}

	if len(f.LogTail) > 0 {
		b.WriteString("The last lines of the log:\n\n")
		for _, line := range f.LogTail {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	return b.String()
}

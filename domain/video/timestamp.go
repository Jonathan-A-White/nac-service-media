package video

import (
	"fmt"
	"regexp"
	"strconv"
)

// Timestamp represents a video timestamp in HH:MM:SS format
type Timestamp struct {
	Hours   int
	Minutes int
	Seconds int
}

// timestampRegex matches HH:MM:SS format
var timestampRegex = regexp.MustCompile(`^(\d{2}):(\d{2}):(\d{2})$`)

// ParseTimestamp parses a timestamp string in HH:MM:SS format
func ParseTimestamp(s string) (Timestamp, error) {
	matches := timestampRegex.FindStringSubmatch(s)
	if matches == nil {
		return Timestamp{}, fmt.Errorf("invalid timestamp format %q: expected HH:MM:SS", s)
	}

	hours, _ := strconv.Atoi(matches[1])
	minutes, _ := strconv.Atoi(matches[2])
	seconds, _ := strconv.Atoi(matches[3])

	if minutes > 59 {
		return Timestamp{}, fmt.Errorf("invalid timestamp %q: minutes must be 0-59", s)
	}
	if seconds > 59 {
		return Timestamp{}, fmt.Errorf("invalid timestamp %q: seconds must be 0-59", s)
	}

	return Timestamp{
		Hours:   hours,
		Minutes: minutes,
		Seconds: seconds,
	}, nil
}

// String returns the timestamp in HH:MM:SS format
func (t Timestamp) String() string {
	return fmt.Sprintf("%02d:%02d:%02d", t.Hours, t.Minutes, t.Seconds)
}

// TotalSeconds returns the timestamp as total seconds
func (t Timestamp) TotalSeconds() int {
	return t.Hours*3600 + t.Minutes*60 + t.Seconds
}

// IsZero returns true if the timestamp is 00:00:00
func (t Timestamp) IsZero() bool {
	return t.Hours == 0 && t.Minutes == 0 && t.Seconds == 0
}

// Before returns true if t is before other
func (t Timestamp) Before(other Timestamp) bool {
	return t.TotalSeconds() < other.TotalSeconds()
}

// After returns true if t is after other
func (t Timestamp) After(other Timestamp) bool {
	return t.TotalSeconds() > other.TotalSeconds()
}

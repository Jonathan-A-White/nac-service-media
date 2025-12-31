package video

import (
	"testing"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Timestamp
		wantErr bool
		errMsg  string
	}{
		{
			name:  "valid timestamp",
			input: "01:30:45",
			want:  Timestamp{Hours: 1, Minutes: 30, Seconds: 45},
		},
		{
			name:  "all zeros",
			input: "00:00:00",
			want:  Timestamp{Hours: 0, Minutes: 0, Seconds: 0},
		},
		{
			name:  "max valid minutes/seconds",
			input: "23:59:59",
			want:  Timestamp{Hours: 23, Minutes: 59, Seconds: 59},
		},
		{
			name:  "large hours value",
			input: "99:00:00",
			want:  Timestamp{Hours: 99, Minutes: 0, Seconds: 0},
		},
		{
			name:    "missing leading zero in hours",
			input:   "1:30:45",
			wantErr: true,
			errMsg:  "invalid timestamp format",
		},
		{
			name:    "missing leading zero in minutes",
			input:   "01:3:45",
			wantErr: true,
			errMsg:  "invalid timestamp format",
		},
		{
			name:    "missing leading zero in seconds",
			input:   "01:30:5",
			wantErr: true,
			errMsg:  "invalid timestamp format",
		},
		{
			name:    "wrong separator - dash",
			input:   "01-30-45",
			wantErr: true,
			errMsg:  "invalid timestamp format",
		},
		{
			name:    "too few parts",
			input:   "01:30",
			wantErr: true,
			errMsg:  "invalid timestamp format",
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
			errMsg:  "invalid timestamp format",
		},
		{
			name:    "minutes too high",
			input:   "01:60:00",
			wantErr: true,
			errMsg:  "minutes must be 0-59",
		},
		{
			name:    "seconds too high",
			input:   "01:30:60",
			wantErr: true,
			errMsg:  "seconds must be 0-59",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimestamp(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTimestamp(%q) expected error, got nil", tt.input)
					return
				}
				if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("ParseTimestamp(%q) error = %v, want error containing %q", tt.input, err, tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseTimestamp(%q) unexpected error: %v", tt.input, err)
				return
			}

			if got != tt.want {
				t.Errorf("ParseTimestamp(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTimestamp_String(t *testing.T) {
	tests := []struct {
		timestamp Timestamp
		want      string
	}{
		{Timestamp{0, 0, 0}, "00:00:00"},
		{Timestamp{1, 2, 3}, "01:02:03"},
		{Timestamp{12, 34, 56}, "12:34:56"},
		{Timestamp{99, 59, 59}, "99:59:59"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.timestamp.String(); got != tt.want {
				t.Errorf("Timestamp.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTimestamp_TotalSeconds(t *testing.T) {
	tests := []struct {
		timestamp Timestamp
		want      int
	}{
		{Timestamp{0, 0, 0}, 0},
		{Timestamp{0, 0, 1}, 1},
		{Timestamp{0, 1, 0}, 60},
		{Timestamp{1, 0, 0}, 3600},
		{Timestamp{1, 30, 45}, 5445},
	}

	for _, tt := range tests {
		t.Run(tt.timestamp.String(), func(t *testing.T) {
			if got := tt.timestamp.TotalSeconds(); got != tt.want {
				t.Errorf("Timestamp.TotalSeconds() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTimestamp_IsZero(t *testing.T) {
	zero := Timestamp{Hours: 0, Minutes: 0, Seconds: 0}
	if !zero.IsZero() {
		t.Error("expected zero timestamp to be zero")
	}
	nonzero := Timestamp{Hours: 0, Minutes: 0, Seconds: 1}
	if nonzero.IsZero() {
		t.Error("expected non-zero timestamp to not be zero")
	}
}

func TestTimestamp_Before(t *testing.T) {
	earlier := Timestamp{Hours: 0, Minutes: 30, Seconds: 0}
	later := Timestamp{Hours: 1, Minutes: 0, Seconds: 0}

	if !earlier.Before(later) {
		t.Error("expected earlier to be before later")
	}
	if later.Before(earlier) {
		t.Error("expected later to not be before earlier")
	}
	if earlier.Before(earlier) {
		t.Error("expected timestamp to not be before itself")
	}
}

func TestTimestamp_After(t *testing.T) {
	earlier := Timestamp{Hours: 0, Minutes: 30, Seconds: 0}
	later := Timestamp{Hours: 1, Minutes: 0, Seconds: 0}

	if !later.After(earlier) {
		t.Error("expected later to be after earlier")
	}
	if earlier.After(later) {
		t.Error("expected earlier to not be after later")
	}
	if later.After(later) {
		t.Error("expected timestamp to not be after itself")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

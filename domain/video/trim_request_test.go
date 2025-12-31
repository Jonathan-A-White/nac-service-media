package video

import (
	"testing"
	"time"
)

func TestNewTrimRequest(t *testing.T) {
	tests := []struct {
		name        string
		sourcePath  string
		start       Timestamp
		end         Timestamp
		wantDate    string
		wantErr     bool
		errContains string
	}{
		{
			name:       "valid request",
			sourcePath: "/path/to/2025-12-28 10-06-16.mp4",
			start:      Timestamp{0, 5, 30},
			end:        Timestamp{1, 45, 0},
			wantDate:   "2025-12-28",
		},
		{
			name:       "valid request with different date",
			sourcePath: "/videos/2024-01-15 09-00-00.mp4",
			start:      Timestamp{0, 0, 0},
			end:        Timestamp{2, 0, 0},
			wantDate:   "2024-01-15",
		},
		{
			name:        "filename without date",
			sourcePath:  "/path/to/recording.mp4",
			start:       Timestamp{0, 5, 30},
			end:         Timestamp{1, 45, 0},
			wantErr:     true,
			errContains: "does not match expected format",
		},
		{
			name:        "filename with wrong format",
			sourcePath:  "/path/to/12-28-2025 10-06-16.mp4",
			start:       Timestamp{0, 5, 30},
			end:         Timestamp{1, 45, 0},
			wantErr:     true,
			errContains: "does not match expected format",
		},
		{
			name:        "end before start",
			sourcePath:  "/path/to/2025-12-28 10-06-16.mp4",
			start:       Timestamp{1, 0, 0},
			end:         Timestamp{0, 30, 0},
			wantErr:     true,
			errContains: "must be after start time",
		},
		{
			name:        "end equals start",
			sourcePath:  "/path/to/2025-12-28 10-06-16.mp4",
			start:       Timestamp{1, 0, 0},
			end:         Timestamp{1, 0, 0},
			wantErr:     true,
			errContains: "must be after start time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewTrimRequest(tt.sourcePath, tt.start, tt.end)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewTrimRequest() expected error, got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("NewTrimRequest() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("NewTrimRequest() unexpected error: %v", err)
				return
			}

			if got.ServiceDate.Format("2006-01-02") != tt.wantDate {
				t.Errorf("NewTrimRequest() ServiceDate = %v, want %v", got.ServiceDate.Format("2006-01-02"), tt.wantDate)
			}
		})
	}
}

func TestTrimRequest_OutputFilename(t *testing.T) {
	req := &TrimRequest{
		ServiceDate: time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
	}

	want := "2025-12-28.mp4"
	if got := req.OutputFilename(); got != want {
		t.Errorf("TrimRequest.OutputFilename() = %q, want %q", got, want)
	}
}

func TestTrimRequest_OutputPath(t *testing.T) {
	req := &TrimRequest{
		ServiceDate: time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		outputDir string
		want      string
	}{
		{"/home/user/trimmed", "/home/user/trimmed/2025-12-28.mp4"},
		{"/tmp", "/tmp/2025-12-28.mp4"},
	}

	for _, tt := range tests {
		t.Run(tt.outputDir, func(t *testing.T) {
			if got := req.OutputPath(tt.outputDir); got != tt.want {
				t.Errorf("TrimRequest.OutputPath(%q) = %q, want %q", tt.outputDir, got, tt.want)
			}
		})
	}
}

func TestTrimRequest_Validate(t *testing.T) {
	tests := []struct {
		name        string
		req         TrimRequest
		wantErr     bool
		errContains string
	}{
		{
			name: "valid request",
			req: TrimRequest{
				SourcePath: "/path/to/video.mp4",
				Start:      Timestamp{0, 5, 0},
				End:        Timestamp{1, 0, 0},
			},
			wantErr: false,
		},
		{
			name: "empty source path",
			req: TrimRequest{
				SourcePath: "",
				Start:      Timestamp{0, 5, 0},
				End:        Timestamp{1, 0, 0},
			},
			wantErr:     true,
			errContains: "source path is required",
		},
		{
			name: "end before start",
			req: TrimRequest{
				SourcePath: "/path/to/video.mp4",
				Start:      Timestamp{1, 0, 0},
				End:        Timestamp{0, 30, 0},
			},
			wantErr:     true,
			errContains: "must be after start time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()

			if tt.wantErr {
				if err == nil {
					t.Errorf("TrimRequest.Validate() expected error, got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("TrimRequest.Validate() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("TrimRequest.Validate() unexpected error: %v", err)
			}
		})
	}
}

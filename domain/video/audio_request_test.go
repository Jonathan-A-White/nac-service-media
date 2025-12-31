package video

import (
	"testing"
	"time"
)

func TestNewAudioExtractionRequest(t *testing.T) {
	tests := []struct {
		name        string
		sourcePath  string
		serviceDate time.Time
		bitrate     string
		wantBitrate string
		wantErr     bool
		errContains string
	}{
		{
			name:        "valid request with explicit bitrate",
			sourcePath:  "/path/to/2025-12-28.mp4",
			serviceDate: time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
			bitrate:     "192k",
			wantBitrate: "192k",
		},
		{
			name:        "valid request with default bitrate",
			sourcePath:  "/path/to/2025-12-28.mp4",
			serviceDate: time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
			bitrate:     "",
			wantBitrate: DefaultAudioBitrate,
		},
		{
			name:        "valid request with custom bitrate",
			sourcePath:  "/path/to/2025-12-28.mp4",
			serviceDate: time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
			bitrate:     "320k",
			wantBitrate: "320k",
		},
		{
			name:        "empty source path",
			sourcePath:  "",
			serviceDate: time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
			bitrate:     "192k",
			wantErr:     true,
			errContains: "source video path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewAudioExtractionRequest(tt.sourcePath, tt.serviceDate, tt.bitrate)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewAudioExtractionRequest() expected error, got nil")
					return
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("NewAudioExtractionRequest() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("NewAudioExtractionRequest() unexpected error: %v", err)
				return
			}

			if got.Bitrate != tt.wantBitrate {
				t.Errorf("NewAudioExtractionRequest() Bitrate = %q, want %q", got.Bitrate, tt.wantBitrate)
			}
		})
	}
}

func TestAudioExtractionRequest_OutputFilename(t *testing.T) {
	tests := []struct {
		name        string
		serviceDate time.Time
		want        string
	}{
		{
			name:        "standard date",
			serviceDate: time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
			want:        "2025-12-28.mp3",
		},
		{
			name:        "january date",
			serviceDate: time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC),
			want:        "2025-01-15.mp3",
		},
		{
			name:        "single digit day",
			serviceDate: time.Date(2025, 3, 5, 0, 0, 0, 0, time.UTC),
			want:        "2025-03-05.mp3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &AudioExtractionRequest{
				ServiceDate: tt.serviceDate,
			}

			if got := req.OutputFilename(); got != tt.want {
				t.Errorf("AudioExtractionRequest.OutputFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAudioExtractionRequest_OutputPath(t *testing.T) {
	req := &AudioExtractionRequest{
		ServiceDate: time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
	}

	tests := []struct {
		outputDir string
		want      string
	}{
		{"/home/user/audio", "/home/user/audio/2025-12-28.mp3"},
		{"/tmp", "/tmp/2025-12-28.mp3"},
	}

	for _, tt := range tests {
		t.Run(tt.outputDir, func(t *testing.T) {
			if got := req.OutputPath(tt.outputDir); got != tt.want {
				t.Errorf("AudioExtractionRequest.OutputPath(%q) = %q, want %q", tt.outputDir, got, tt.want)
			}
		})
	}
}

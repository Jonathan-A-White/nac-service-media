package drive

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/api/drive/v3"
)

// mockDriveService is a mock implementation for testing
type mockDriveService struct {
	files      []*drive.File
	shouldFail bool
	failError  error
}

func (m *mockDriveService) ListFiles(ctx context.Context, query string, fields string, orderBy string) ([]*drive.File, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	return m.files, nil
}

func TestClient_ListFiles(t *testing.T) {
	testTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		mock      *mockDriveService
		folderID  string
		wantCount int
		wantErr   bool
		errMsg    string
	}{
		{
			name: "lists files successfully",
			mock: &mockDriveService{
				files: []*drive.File{
					{
						Id:          "file-1",
						Name:        "2025-12-28.mp4",
						MimeType:    "video/mp4",
						Size:        1000000,
						CreatedTime: testTime.Format(time.RFC3339),
					},
					{
						Id:          "file-2",
						Name:        "2025-12-21.mp4",
						MimeType:    "video/mp4",
						Size:        900000,
						CreatedTime: testTime.Add(-7 * 24 * time.Hour).Format(time.RFC3339),
					},
				},
			},
			folderID:  "test-folder-id",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "returns empty list for empty folder",
			mock: &mockDriveService{
				files: []*drive.File{},
			},
			folderID:  "empty-folder-id",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "handles API error",
			mock: &mockDriveService{
				shouldFail: true,
				failError:  fmt.Errorf("googleapi: Error 403: permission denied"),
			},
			folderID: "test-folder-id",
			wantErr:  true,
			errMsg:   "failed to list files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(
				context.Background(),
				"",
				WithDriveService(tt.mock),
			)
			if err != nil {
				t.Fatalf("failed to create client: %v", err)
			}

			files, err := client.ListFiles(context.Background(), tt.folderID)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				} else if tt.errMsg != "" && !containsString(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(files) != tt.wantCount {
				t.Errorf("expected %d files, got %d", tt.wantCount, len(files))
			}
		})
	}
}

func TestClient_ListFiles_FileInfo(t *testing.T) {
	testTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)

	mock := &mockDriveService{
		files: []*drive.File{
			{
				Id:          "file-123",
				Name:        "2025-12-28.mp4",
				MimeType:    "video/mp4",
				Size:        1234567,
				CreatedTime: testTime.Format(time.RFC3339),
			},
		},
	}

	client, err := NewClient(
		context.Background(),
		"",
		WithDriveService(mock),
	)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	files, err := client.ListFiles(context.Background(), "test-folder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	file := files[0]
	if file.ID != "file-123" {
		t.Errorf("expected ID 'file-123', got %q", file.ID)
	}
	if file.Name != "2025-12-28.mp4" {
		t.Errorf("expected Name '2025-12-28.mp4', got %q", file.Name)
	}
	if file.MimeType != "video/mp4" {
		t.Errorf("expected MimeType 'video/mp4', got %q", file.MimeType)
	}
	if file.Size != 1234567 {
		t.Errorf("expected Size 1234567, got %d", file.Size)
	}
	if !file.CreatedTime.Equal(testTime) {
		t.Errorf("expected CreatedTime %v, got %v", testTime, file.CreatedTime)
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantZero bool
	}{
		{
			name:     "valid RFC3339 time",
			input:    "2025-12-28T10:00:00Z",
			wantZero: false,
		},
		{
			name:     "invalid time format",
			input:    "invalid",
			wantZero: true,
		},
		{
			name:     "empty string",
			input:    "",
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTime(tt.input)
			if tt.wantZero && !result.IsZero() {
				t.Error("expected zero time, got non-zero")
			}
			if !tt.wantZero && result.IsZero() {
				t.Error("expected non-zero time, got zero")
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

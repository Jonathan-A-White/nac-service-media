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
	files          []*drive.File
	shouldFail     bool
	failError      error
	storageLimit   int64
	storageUsage   int64
	deletedFileIDs []string
	trashEmptied   bool
}

func (m *mockDriveService) ListFiles(ctx context.Context, query string, fields string, orderBy string) ([]*drive.File, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	return m.files, nil
}

func (m *mockDriveService) GetAbout(ctx context.Context, fields string) (*drive.About, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	return &drive.About{
		StorageQuota: &drive.AboutStorageQuota{
			Limit: m.storageLimit,
			Usage: m.storageUsage,
		},
	}, nil
}

func (m *mockDriveService) DeleteFile(ctx context.Context, fileID string) error {
	if m.shouldFail {
		return m.failError
	}
	m.deletedFileIDs = append(m.deletedFileIDs, fileID)
	return nil
}

func (m *mockDriveService) EmptyTrash(ctx context.Context) error {
	if m.shouldFail {
		return m.failError
	}
	m.trashEmptied = true
	return nil
}

func (m *mockDriveService) UploadFile(ctx context.Context, fileName, mimeType, folderID, localPath string) (*drive.File, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	return &drive.File{
		Id:          "uploaded-file-id",
		Name:        fileName,
		MimeType:    mimeType,
		Size:        1024,
		WebViewLink: "https://drive.google.com/file/d/uploaded-file-id/view",
	}, nil
}

func (m *mockDriveService) CreatePermission(ctx context.Context, fileID string, permission *drive.Permission) error {
	if m.shouldFail {
		return m.failError
	}
	return nil
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

func TestClient_GetStorageQuota(t *testing.T) {
	tests := []struct {
		name          string
		mock          *mockDriveService
		wantTotal     int64
		wantUsed      int64
		wantAvailable int64
		wantErr       bool
	}{
		{
			name: "returns storage quota successfully",
			mock: &mockDriveService{
				storageLimit: 15000000000, // 15 GB
				storageUsage: 5000000000,  // 5 GB
			},
			wantTotal:     15000000000,
			wantUsed:      5000000000,
			wantAvailable: 10000000000,
			wantErr:       false,
		},
		{
			name: "handles API error",
			mock: &mockDriveService{
				shouldFail: true,
				failError:  fmt.Errorf("API error"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := NewClient(context.Background(), "", WithDriveService(tt.mock))
			storage, err := client.GetStorageQuota(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if storage.TotalBytes != tt.wantTotal {
				t.Errorf("expected TotalBytes %d, got %d", tt.wantTotal, storage.TotalBytes)
			}
			if storage.UsedBytes != tt.wantUsed {
				t.Errorf("expected UsedBytes %d, got %d", tt.wantUsed, storage.UsedBytes)
			}
			if storage.AvailableBytes != tt.wantAvailable {
				t.Errorf("expected AvailableBytes %d, got %d", tt.wantAvailable, storage.AvailableBytes)
			}
		})
	}
}

func TestClient_ListMP4Files(t *testing.T) {
	testTime := time.Date(2025, 12, 28, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		mock      *mockDriveService
		wantCount int
		wantFirst string
		wantErr   bool
	}{
		{
			name: "lists mp4 files sorted by name",
			mock: &mockDriveService{
				files: []*drive.File{
					{Id: "file-1", Name: "2025-11-03.mp4", MimeType: "video/mp4", Size: 1000000, CreatedTime: testTime.Format(time.RFC3339)},
					{Id: "file-2", Name: "2025-11-10 08-00-00.mp4", MimeType: "video/mp4", Size: 2000000, CreatedTime: testTime.Format(time.RFC3339)},
					{Id: "file-3", Name: "2025-11-17.mp4", MimeType: "video/mp4", Size: 3000000, CreatedTime: testTime.Format(time.RFC3339)},
				},
			},
			wantCount: 3,
			wantFirst: "2025-11-03.mp4",
			wantErr:   false,
		},
		{
			name: "returns empty list for folder with no mp4s",
			mock: &mockDriveService{
				files: []*drive.File{},
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name: "handles API error",
			mock: &mockDriveService{
				shouldFail: true,
				failError:  fmt.Errorf("API error"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := NewClient(context.Background(), "", WithDriveService(tt.mock))
			files, err := client.ListMP4Files(context.Background(), "test-folder")

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
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

			if tt.wantFirst != "" && len(files) > 0 && files[0].Name != tt.wantFirst {
				t.Errorf("expected first file %q, got %q", tt.wantFirst, files[0].Name)
			}
		})
	}
}

func TestClient_DeletePermanently(t *testing.T) {
	tests := []struct {
		name    string
		mock    *mockDriveService
		fileID  string
		wantErr bool
	}{
		{
			name:    "deletes file successfully",
			mock:    &mockDriveService{},
			fileID:  "file-123",
			wantErr: false,
		},
		{
			name: "handles API error",
			mock: &mockDriveService{
				shouldFail: true,
				failError:  fmt.Errorf("API error"),
			},
			fileID:  "file-123",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := NewClient(context.Background(), "", WithDriveService(tt.mock))
			err := client.DeletePermanently(context.Background(), tt.fileID)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify the file was marked as deleted in the mock
			found := false
			for _, id := range tt.mock.deletedFileIDs {
				if id == tt.fileID {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected file %q to be deleted", tt.fileID)
			}
		})
	}
}

func TestClient_EmptyTrash(t *testing.T) {
	tests := []struct {
		name    string
		mock    *mockDriveService
		wantErr bool
	}{
		{
			name:    "empties trash successfully",
			mock:    &mockDriveService{},
			wantErr: false,
		},
		{
			name: "handles API error",
			mock: &mockDriveService{
				shouldFail: true,
				failError:  fmt.Errorf("API error"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := NewClient(context.Background(), "", WithDriveService(tt.mock))
			err := client.EmptyTrash(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if !tt.mock.trashEmptied {
				t.Error("expected trash to be emptied")
			}
		})
	}
}

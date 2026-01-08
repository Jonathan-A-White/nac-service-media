package process

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"nac-service-media/domain/distribution"
	"nac-service-media/domain/notification"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
)

// --- Mock implementations for testing ---

// mockTrimmer implements video.Trimmer for testing
type mockTrimmer struct {
	shouldFail bool
	failError  error
}

func (m *mockTrimmer) Trim(ctx context.Context, req *video.TrimRequest, outputPath string) error {
	if m.shouldFail {
		return m.failError
	}
	return nil
}

// mockExtractor implements video.AudioExtractor for testing
type mockExtractor struct {
	shouldFail bool
	failError  error
}

func (m *mockExtractor) Extract(ctx context.Context, req *video.AudioExtractionRequest, outputPath string) error {
	if m.shouldFail {
		return m.failError
	}
	return nil
}

// mockFileChecker implements video.FileChecker for testing
type mockFileChecker struct {
	existingFiles map[string]bool
}

func (m *mockFileChecker) Exists(path string) bool {
	return m.existingFiles[path]
}

// mockFileSizer implements FileSizer for testing
type mockFileSizer struct {
	sizes map[string]int64
}

func (m *mockFileSizer) Size(path string) int64 {
	if size, ok := m.sizes[path]; ok {
		return size
	}
	return 0
}

// mockDriveClient implements distribution.DriveClient for testing
type mockDriveClient struct {
	files              map[string]*distribution.FileInfo // keyed by fileName
	findFileByNameErr  error                             // error to return from FindFileByName
	findFileByNameErrs map[string]error                  // per-file errors for FindFileByName
	uploadErr          error
	storageInfo        *distribution.StorageInfo
}

func newMockDriveClient() *mockDriveClient {
	return &mockDriveClient{
		files:              make(map[string]*distribution.FileInfo),
		findFileByNameErrs: make(map[string]error),
		storageInfo: &distribution.StorageInfo{
			TotalBytes:     15 * 1024 * 1024 * 1024,
			UsedBytes:      0,
			AvailableBytes: 15 * 1024 * 1024 * 1024,
		},
	}
}

func (m *mockDriveClient) ListFiles(ctx context.Context, folderID string) ([]distribution.FileInfo, error) {
	var result []distribution.FileInfo
	for _, f := range m.files {
		result = append(result, *f)
	}
	return result, nil
}

func (m *mockDriveClient) FindFileByName(ctx context.Context, folderID, fileName string) (*distribution.FileInfo, error) {
	// Check for per-file error first
	if err, ok := m.findFileByNameErrs[fileName]; ok && err != nil {
		return nil, err
	}
	// Then check global error
	if m.findFileByNameErr != nil {
		return nil, m.findFileByNameErr
	}
	// Return file if exists
	if file, ok := m.files[fileName]; ok {
		return file, nil
	}
	return nil, nil // Not found is not an error
}

func (m *mockDriveClient) GetStorageQuota(ctx context.Context) (*distribution.StorageInfo, error) {
	return m.storageInfo, nil
}

func (m *mockDriveClient) ListMP4Files(ctx context.Context, folderID string) ([]distribution.FileInfo, error) {
	var result []distribution.FileInfo
	for _, f := range m.files {
		if f.MimeType == "video/mp4" {
			result = append(result, *f)
		}
	}
	return result, nil
}

func (m *mockDriveClient) DeletePermanently(ctx context.Context, fileID string) error {
	return nil
}

func (m *mockDriveClient) EmptyTrash(ctx context.Context) error {
	return nil
}

func (m *mockDriveClient) Upload(ctx context.Context, req distribution.UploadRequest) (*distribution.UploadResult, error) {
	if m.uploadErr != nil {
		return nil, m.uploadErr
	}
	return &distribution.UploadResult{
		FileID:       "test-file-id",
		FileName:     req.FileName,
		ShareableURL: "https://drive.google.com/file/d/test-file-id/view",
		Size:         1024,
	}, nil
}

func (m *mockDriveClient) SetPublicSharing(ctx context.Context, fileID string) error {
	return nil
}

func (m *mockDriveClient) UploadAndShare(ctx context.Context, req distribution.UploadRequest) (*distribution.UploadResult, error) {
	if m.uploadErr != nil {
		return nil, m.uploadErr
	}
	return &distribution.UploadResult{
		FileID:       "test-file-id",
		FileName:     req.FileName,
		ShareableURL: "https://drive.google.com/file/d/test-file-id/view?usp=sharing",
		Size:         1024,
	}, nil
}

// mockEmailSender implements notification.EmailSender for testing
type mockEmailSender struct {
	sentEmails []notification.Email
	shouldFail bool
	failError  error
}

func (m *mockEmailSender) Send(email notification.Email) error {
	if m.shouldFail {
		return m.failError
	}
	m.sentEmails = append(m.sentEmails, email)
	return nil
}

// mockFileFinder implements FileFinder for testing
type mockFileFinder struct {
	files     []string
	sourceDir string
	findErr   error
}

func (m *mockFileFinder) FindNewestFile(dir, ext string) (string, error) {
	if m.findErr != nil {
		return "", m.findErr
	}
	if len(m.files) == 0 {
		return "", errors.New("no video files found")
	}
	return m.files[0], nil
}

func (m *mockFileFinder) ListFiles(dir, ext string) ([]string, error) {
	return m.files, nil
}

// --- Helper functions ---

func createTestConfig() *config.Config {
	return &config.Config{
		Paths: config.PathsConfig{
			SourceDirectory:  "/test/source",
			TrimmedDirectory: "/test/trimmed",
			AudioDirectory:   "/test/audio",
		},
		Audio: config.AudioConfig{
			Bitrate: "192k",
		},
		Google: config.GoogleConfig{
			ServicesFolderID: "folder123",
		},
		Email: config.EmailConfig{
			FromName:    "Test Church",
			FromAddress: "church@example.com",
			Recipients: map[string]config.RecipientConfig{
				"jane": {Name: "Jane Doe", Address: "jane@example.com"},
				"john": {Name: "John Doe", Address: "john@example.com"},
			},
		},
		Ministers: map[string]config.MinisterConfig{
			"smith": {Name: "Pr. John Smith"},
		},
		Senders: config.SendersConfig{
			Senders: map[string]config.SenderConfig{
				"avteam": {Name: "A/V Team"},
			},
			DefaultSender: "avteam",
		},
	}
}

func createTestService(
	driveClient distribution.DriveClient,
	fileChecker *mockFileChecker,
	fileFinder *mockFileFinder,
	cfg *config.Config,
) *Service {
	return NewService(
		&mockTrimmer{},
		&mockExtractor{},
		fileChecker,
		&mockFileSizer{sizes: make(map[string]int64)},
		driveClient,
		&mockEmailSender{},
		fileFinder,
		cfg,
		&bytes.Buffer{},
	)
}

// Note: Already-processed check tests have been removed because the check
// is now performed earlier in cmd/process.go before auto-detection runs.
// This functionality is tested via integration tests in features/process.feature.

// --- Additional Edge Case Tests ---

func TestValidateInputs_ReturnsCorrectServiceDate(t *testing.T) {
	ctx := context.Background()
	cfg := createTestConfig()

	driveClient := newMockDriveClient()

	fileChecker := &mockFileChecker{
		existingFiles: map[string]bool{
			"/test/source/2025-12-28 10-06-16.mp4": true,
		},
	}

	fileFinder := &mockFileFinder{
		files: []string{"/test/source/2025-12-28 10-06-16.mp4"},
	}

	service := createTestService(driveClient, fileChecker, fileFinder, cfg)

	input := Input{
		InputPath:     "", // auto-detect mode
		StartTime:     "00:05:30",
		EndTime:       "01:45:00",
		MinisterKey:   "smith",
		RecipientKeys: []string{"jane"},
	}

	_, serviceDate, _, _, _, _, err := service.validateInputs(ctx, input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedDate := time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC)
	if !serviceDate.Equal(expectedDate) {
		t.Errorf("expected service date %v, got %v", expectedDate, serviceDate)
	}
}

func TestValidateInputs_ReturnsCorrectSourcePath(t *testing.T) {
	ctx := context.Background()
	cfg := createTestConfig()

	driveClient := newMockDriveClient()

	expectedPath := "/test/source/2025-12-28 10-06-16.mp4"
	fileChecker := &mockFileChecker{
		existingFiles: map[string]bool{
			expectedPath: true,
		},
	}

	fileFinder := &mockFileFinder{
		files: []string{expectedPath},
	}

	service := createTestService(driveClient, fileChecker, fileFinder, cfg)

	input := Input{
		InputPath:     "", // auto-detect mode
		StartTime:     "00:05:30",
		EndTime:       "01:45:00",
		MinisterKey:   "smith",
		RecipientKeys: []string{"jane"},
	}

	sourcePath, _, _, _, _, _, err := service.validateInputs(ctx, input)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sourcePath != expectedPath {
		t.Errorf("expected source path %q, got %q", expectedPath, sourcePath)
	}
}

// --- Helper ---

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

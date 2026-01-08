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

// --- Unit Tests for validateInputs: Already-Processed Check ---

func TestValidateInputs_AlreadyProcessedCheck(t *testing.T) {
	tests := []struct {
		name           string
		inputPath      string // empty = auto-detect mode
		mp4Exists      bool
		mp3Exists      bool
		mp4CheckErr    error
		mp3CheckErr    error
		wantErr        bool
		wantErrContain string
	}{
		{
			name:           "auto-detect mode - both files exist - should return error",
			inputPath:      "",
			mp4Exists:      true,
			mp3Exists:      true,
			wantErr:        true,
			wantErrContain: "has already been processed",
		},
		{
			name:      "auto-detect mode - only mp4 exists - should succeed",
			inputPath: "",
			mp4Exists: true,
			mp3Exists: false,
			wantErr:   false,
		},
		{
			name:      "auto-detect mode - only mp3 exists - should succeed",
			inputPath: "",
			mp4Exists: false,
			mp3Exists: true,
			wantErr:   false,
		},
		{
			name:      "auto-detect mode - neither file exists - should succeed",
			inputPath: "",
			mp4Exists: false,
			mp3Exists: false,
			wantErr:   false,
		},
		{
			name:      "explicit input mode - both files exist - should succeed (skip check)",
			inputPath: "/test/source/2025-12-28 10-06-16.mp4",
			mp4Exists: true,
			mp3Exists: true,
			wantErr:   false,
		},
		{
			name:           "auto-detect mode - Drive API error during mp4 check",
			inputPath:      "",
			mp4CheckErr:    errors.New("network timeout"),
			wantErr:        true,
			wantErrContain: "failed to check Drive",
		},
		{
			name:           "auto-detect mode - Drive API error during mp3 check",
			inputPath:      "",
			mp4Exists:      false, // mp4 check passes
			mp3CheckErr:    errors.New("authentication expired"),
			wantErr:        true,
			wantErrContain: "failed to check Drive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := createTestConfig()

			// Setup mock drive client
			driveClient := newMockDriveClient()

			// Setup files in drive based on test case
			if tt.mp4Exists {
				driveClient.files["2025-12-28.mp4"] = &distribution.FileInfo{
					ID:       "mp4-file-id",
					Name:     "2025-12-28.mp4",
					MimeType: "video/mp4",
					Size:     1200000000,
				}
			}
			if tt.mp3Exists {
				driveClient.files["2025-12-28.mp3"] = &distribution.FileInfo{
					ID:       "mp3-file-id",
					Name:     "2025-12-28.mp3",
					MimeType: "audio/mpeg",
					Size:     85000000,
				}
			}

			// Setup per-file errors for Drive API
			if tt.mp4CheckErr != nil {
				driveClient.findFileByNameErrs["2025-12-28.mp4"] = tt.mp4CheckErr
			}
			if tt.mp3CheckErr != nil {
				driveClient.findFileByNameErrs["2025-12-28.mp3"] = tt.mp3CheckErr
			}

			// Setup file checker
			fileChecker := &mockFileChecker{
				existingFiles: map[string]bool{
					"/test/source/2025-12-28 10-06-16.mp4": true,
				},
			}

			// Setup file finder
			fileFinder := &mockFileFinder{
				files: []string{"/test/source/2025-12-28 10-06-16.mp4"},
			}

			// Create service
			service := createTestService(driveClient, fileChecker, fileFinder, cfg)

			// Create input
			input := Input{
				InputPath:     tt.inputPath,
				StartTime:     "00:05:30",
				EndTime:       "01:45:00",
				MinisterKey:   "smith",
				RecipientKeys: []string{"jane"},
			}

			// Call validateInputs
			_, _, _, _, _, _, err := service.validateInputs(ctx, input)

			// Check results
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErrContain)
					return
				}
				if !containsString(err.Error(), tt.wantErrContain) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErrContain, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestValidateInputs_AlreadyProcessedCheck_DateFromFilename tests that the correct date
// is used when checking for already-processed files
func TestValidateInputs_AlreadyProcessedCheck_DateFromFilename(t *testing.T) {
	tests := []struct {
		name           string
		sourceFile     string
		expectedDate   string
		mp4Exists      bool
		mp3Exists      bool
		wantErr        bool
		wantErrContain string
	}{
		{
			name:           "OBS format filename - 2025-12-28 - both exist",
			sourceFile:     "/test/source/2025-12-28 10-06-16.mp4",
			expectedDate:   "2025-12-28",
			mp4Exists:      true,
			mp3Exists:      true,
			wantErr:        true,
			wantErrContain: "2025-12-28",
		},
		{
			name:           "OBS format filename - 2025-01-05 - both exist",
			sourceFile:     "/test/source/2025-01-05 09-30-00.mp4",
			expectedDate:   "2025-01-05",
			mp4Exists:      true,
			mp3Exists:      true,
			wantErr:        true,
			wantErrContain: "2025-01-05",
		},
		{
			name:         "OBS format filename - neither exist",
			sourceFile:   "/test/source/2025-12-28 10-06-16.mp4",
			expectedDate: "2025-12-28",
			mp4Exists:    false,
			mp3Exists:    false,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := createTestConfig()

			driveClient := newMockDriveClient()

			// Setup files in drive based on expected date
			if tt.mp4Exists {
				driveClient.files[tt.expectedDate+".mp4"] = &distribution.FileInfo{
					ID:       "mp4-file-id",
					Name:     tt.expectedDate + ".mp4",
					MimeType: "video/mp4",
				}
			}
			if tt.mp3Exists {
				driveClient.files[tt.expectedDate+".mp3"] = &distribution.FileInfo{
					ID:       "mp3-file-id",
					Name:     tt.expectedDate + ".mp3",
					MimeType: "audio/mpeg",
				}
			}

			fileChecker := &mockFileChecker{
				existingFiles: map[string]bool{
					tt.sourceFile: true,
				},
			}

			fileFinder := &mockFileFinder{
				files: []string{tt.sourceFile},
			}

			service := createTestService(driveClient, fileChecker, fileFinder, cfg)

			input := Input{
				InputPath:     "", // auto-detect mode
				StartTime:     "00:05:30",
				EndTime:       "01:45:00",
				MinisterKey:   "smith",
				RecipientKeys: []string{"jane"},
			}

			_, _, _, _, _, _, err := service.validateInputs(ctx, input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErrContain)
					return
				}
				if !containsString(err.Error(), tt.wantErrContain) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErrContain, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestValidateInputs_ExplicitInputBypassesCheck verifies that when --input is provided,
// the already-processed check is completely bypassed regardless of Drive state
func TestValidateInputs_ExplicitInputBypassesCheck(t *testing.T) {
	ctx := context.Background()
	cfg := createTestConfig()

	// Create a drive client where both files exist
	driveClient := newMockDriveClient()
	driveClient.files["2025-12-28.mp4"] = &distribution.FileInfo{
		ID:       "mp4-file-id",
		Name:     "2025-12-28.mp4",
		MimeType: "video/mp4",
	}
	driveClient.files["2025-12-28.mp3"] = &distribution.FileInfo{
		ID:       "mp3-file-id",
		Name:     "2025-12-28.mp3",
		MimeType: "audio/mpeg",
	}

	fileChecker := &mockFileChecker{
		existingFiles: map[string]bool{
			"/test/source/2025-12-28 10-06-16.mp4": true,
		},
	}

	fileFinder := &mockFileFinder{
		files: []string{"/test/source/2025-12-28 10-06-16.mp4"},
	}

	service := createTestService(driveClient, fileChecker, fileFinder, cfg)

	// With explicit input path, should NOT check Drive
	input := Input{
		InputPath:     "/test/source/2025-12-28 10-06-16.mp4",
		StartTime:     "00:05:30",
		EndTime:       "01:45:00",
		MinisterKey:   "smith",
		RecipientKeys: []string{"jane"},
	}

	_, _, _, _, _, _, err := service.validateInputs(ctx, input)

	if err != nil {
		t.Errorf("expected no error with explicit input (should bypass check), got: %v", err)
	}
}

// TestValidateInputs_DriveAPIErrorDetails tests that Drive API errors are properly wrapped
func TestValidateInputs_DriveAPIErrorDetails(t *testing.T) {
	ctx := context.Background()
	cfg := createTestConfig()

	driveClient := newMockDriveClient()
	driveClient.findFileByNameErrs["2025-12-28.mp4"] = errors.New("oauth2: token expired and refresh token is not set")

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

	_, _, _, _, _, _, err := service.validateInputs(ctx, input)

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should contain both the wrapper message and the original error
	if !containsString(err.Error(), "failed to check Drive") {
		t.Errorf("expected error to contain 'failed to check Drive', got: %v", err)
	}
	if !containsString(err.Error(), "token expired") {
		t.Errorf("expected error to contain original error message, got: %v", err)
	}
}

// TestValidateInputs_DateOverrideWithAlreadyProcessed tests that date override
// uses the overridden date when checking for already-processed files
func TestValidateInputs_DateOverrideWithAlreadyProcessed(t *testing.T) {
	ctx := context.Background()
	cfg := createTestConfig()

	driveClient := newMockDriveClient()
	// File exists with the overridden date
	driveClient.files["2025-12-31.mp4"] = &distribution.FileInfo{
		ID:       "mp4-file-id",
		Name:     "2025-12-31.mp4",
		MimeType: "video/mp4",
	}
	driveClient.files["2025-12-31.mp3"] = &distribution.FileInfo{
		ID:       "mp3-file-id",
		Name:     "2025-12-31.mp3",
		MimeType: "audio/mpeg",
	}
	// File does NOT exist with the source file's date
	// (2025-12-28 from the source filename)

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
		DateOverride:  "2025-12-31", // Override to a date with existing files
	}

	_, _, _, _, _, _, err := service.validateInputs(ctx, input)

	if err == nil {
		t.Fatal("expected error because files exist for overridden date")
	}

	if !containsString(err.Error(), "2025-12-31") {
		t.Errorf("expected error to mention overridden date 2025-12-31, got: %v", err)
	}
}

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

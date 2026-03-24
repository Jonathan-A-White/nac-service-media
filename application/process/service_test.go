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
	sentEmails []*notification.EmailRequest
	shouldFail bool
	failError  error
}

func (m *mockEmailSender) Send(req *notification.EmailRequest) error {
	if m.shouldFail {
		return m.failError
	}
	m.sentEmails = append(m.sentEmails, req)
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

// mockDiskChecker implements filesystem.DiskChecker for testing
type mockDiskChecker struct {
	usage float64
	err   error
}

func (m *mockDiskChecker) UsagePercent(path string) (float64, error) {
	return m.usage, m.err
}

// mockFileRemover implements filesystem.FileRemover for testing
type mockFileRemover struct {
	removedFiles []string
	err          error
}

func (m *mockFileRemover) Remove(path string) error {
	if m.err != nil {
		return m.err
	}
	m.removedFiles = append(m.removedFiles, path)
	return nil
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
		&mockDiskChecker{usage: 50.0},
		&mockFileRemover{},
	)
}

func createTestServiceWithCleanup(
	driveClient distribution.DriveClient,
	fileChecker *mockFileChecker,
	fileFinder *mockFileFinder,
	cfg *config.Config,
	diskChecker *mockDiskChecker,
	fileRemover *mockFileRemover,
	output *bytes.Buffer,
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
		output,
		diskChecker,
		fileRemover,
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

// --- Local Cleanup Tests ---

func TestCleanupLocalFiles_SkippedWhenNotNewlyProcessed(t *testing.T) {
	cfg := createTestConfig()
	diskChecker := &mockDiskChecker{usage: 95.0}
	fileRemover := &mockFileRemover{}
	output := &bytes.Buffer{}

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{}},
		&mockFileFinder{files: []string{"/test/source/2025-01-01 10-00-00.mp4"}},
		cfg, diskChecker, fileRemover, output,
	)

	input := CleanupInput{
		IsNewlyProcessed: false,
		SourcePath:       "/test/source/2025-12-28 10-06-16.mp4",
		ServiceDate:      time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
	}

	service.cleanupLocalFiles(input, 70.0, "Post-processing")

	if len(fileRemover.removedFiles) != 0 {
		t.Errorf("expected no files removed, got %v", fileRemover.removedFiles)
	}
}

func TestCleanupLocalFiles_SkippedWhenDiskBelowThreshold(t *testing.T) {
	cfg := createTestConfig()
	diskChecker := &mockDiskChecker{usage: 50.0}
	fileRemover := &mockFileRemover{}
	output := &bytes.Buffer{}

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{}},
		&mockFileFinder{files: []string{"/test/source/2025-01-01 10-00-00.mp4"}},
		cfg, diskChecker, fileRemover, output,
	)

	input := CleanupInput{
		IsNewlyProcessed: true,
		SourcePath:       "/test/source/2025-12-28 10-06-16.mp4",
		ServiceDate:      time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
	}

	service.cleanupLocalFiles(input, 70.0, "Post-processing")

	if len(fileRemover.removedFiles) != 0 {
		t.Errorf("expected no files removed, got %v", fileRemover.removedFiles)
	}
}

func TestCleanupLocalFiles_DeletesOldestFromAllBuckets(t *testing.T) {
	cfg := createTestConfig()
	diskChecker := &mockDiskChecker{usage: 80.0}
	fileRemover := &mockFileRemover{}
	output := &bytes.Buffer{}

	fileFinder := &mockFileFinder{}

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{}},
		fileFinder, cfg, diskChecker, fileRemover, output,
	)

	// Set up files that ListFiles will return for each directory
	// mockFileFinder returns the same files for all dirs, so we need to
	// test deleteOldestFile directly for precise control
	input := CleanupInput{
		IsNewlyProcessed: true,
		SourcePath:       "/test/source/2025-12-28 10-06-16.mp4",
		ServiceDate:      time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
		SkipVideo:        false,
	}

	// Test deleteOldestFile directly
	fileFinder.files = []string{
		"/test/source/2025-01-01 10-00-00.mp4",
		"/test/source/2025-06-15 10-00-00.mp4",
		"/test/source/2025-12-28 10-06-16.mp4",
	}
	err := service.deleteOldestFile("/test/source", ".mp4", input.SourcePath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fileRemover.removedFiles) != 1 || fileRemover.removedFiles[0] != "/test/source/2025-01-01 10-00-00.mp4" {
		t.Errorf("expected oldest source deleted, got %v", fileRemover.removedFiles)
	}
}

func TestCleanupLocalFiles_AudioOnlySkipsTrimmed(t *testing.T) {
	cfg := createTestConfig()
	diskChecker := &mockDiskChecker{usage: 80.0}
	fileRemover := &mockFileRemover{}
	output := &bytes.Buffer{}

	fileFinder := &mockFileFinder{
		files: []string{
			"/test/source/2025-01-01 10-00-00.mp4",
			"/test/source/2025-12-28 10-06-16.mp4",
		},
	}

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{}},
		fileFinder, cfg, diskChecker, fileRemover, output,
	)

	input := CleanupInput{
		IsNewlyProcessed: true,
		SourcePath:       "/test/source/2025-12-28 10-06-16.mp4",
		ServiceDate:      time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
		SkipVideo:        true,
	}

	service.cleanupLocalFiles(input, 70.0, "Post-processing")

	// Should delete from source and audio, NOT trimmed
	// With mockFileFinder returning same files for all dirs, we expect 2 deletions
	if len(fileRemover.removedFiles) != 2 {
		t.Errorf("expected 2 files removed (source + audio), got %d: %v", len(fileRemover.removedFiles), fileRemover.removedFiles)
	}
	outStr := output.String()
	if containsSubstring(outStr, "trimmed cleanup") {
		t.Error("trimmed cleanup should not run in audio-only mode")
	}
}

func TestCleanupLocalFiles_NeverDeletesJustProcessedFile(t *testing.T) {
	cfg := createTestConfig()
	diskChecker := &mockDiskChecker{usage: 80.0}
	fileRemover := &mockFileRemover{}
	output := &bytes.Buffer{}

	// Only file in source is the one just processed
	fileFinder := &mockFileFinder{
		files: []string{"/test/source/2025-12-28 10-06-16.mp4"},
	}

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{}},
		fileFinder, cfg, diskChecker, fileRemover, output,
	)

	err := service.deleteOldestFile("/test/source", ".mp4", "/test/source/2025-12-28 10-06-16.mp4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fileRemover.removedFiles) != 0 {
		t.Errorf("expected no files removed (only file is excluded), got %v", fileRemover.removedFiles)
	}
}

func TestCleanupLocalFiles_ErrorIsNonFatal(t *testing.T) {
	cfg := createTestConfig()
	diskChecker := &mockDiskChecker{usage: 80.0}
	fileRemover := &mockFileRemover{err: errors.New("permission denied")}
	output := &bytes.Buffer{}

	fileFinder := &mockFileFinder{
		files: []string{
			"/test/source/2025-01-01 10-00-00.mp4",
			"/test/source/2025-12-28 10-06-16.mp4",
		},
	}

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{}},
		fileFinder, cfg, diskChecker, fileRemover, output,
	)

	input := CleanupInput{
		IsNewlyProcessed: true,
		SourcePath:       "/test/source/2025-12-28 10-06-16.mp4",
		ServiceDate:      time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
		SkipVideo:        true,
	}

	// Should not panic; errors logged as warnings
	service.cleanupLocalFiles(input, 70.0, "Post-processing")

	outStr := output.String()
	if !containsSubstring(outStr, "Warning") {
		t.Error("expected warning output for cleanup errors")
	}
}

func TestCleanupLocalFiles_PreCleanupTriggersAt90(t *testing.T) {
	cfg := createTestConfig()
	diskChecker := &mockDiskChecker{usage: 91.0}
	fileRemover := &mockFileRemover{}
	output := &bytes.Buffer{}

	fileFinder := &mockFileFinder{
		files: []string{
			"/test/source/2025-01-01 10-00-00.mp4",
			"/test/source/2025-12-28 10-06-16.mp4",
		},
	}

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{}},
		fileFinder, cfg, diskChecker, fileRemover, output,
	)

	input := CleanupInput{
		IsNewlyProcessed: true,
		SourcePath:       "/test/source/2025-12-28 10-06-16.mp4",
		ServiceDate:      time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
		SkipVideo:        false,
	}

	service.cleanupLocalFiles(input, 90.0, "Pre-processing")

	outStr := output.String()
	if !containsSubstring(outStr, "Pre-processing disk cleanup") {
		t.Errorf("expected pre-processing cleanup message, got: %s", outStr)
	}
	if len(fileRemover.removedFiles) == 0 {
		t.Error("expected files to be removed during pre-processing cleanup")
	}
}

func TestCleanupLocalFiles_SkippedAtExactThreshold(t *testing.T) {
	cfg := createTestConfig()
	diskChecker := &mockDiskChecker{usage: 70.0} // exactly at threshold
	fileRemover := &mockFileRemover{}
	output := &bytes.Buffer{}

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{}},
		&mockFileFinder{files: []string{"/test/source/2025-01-01 10-00-00.mp4"}},
		cfg, diskChecker, fileRemover, output,
	)

	input := CleanupInput{
		IsNewlyProcessed: true,
		SourcePath:       "/test/source/2025-12-28 10-06-16.mp4",
		ServiceDate:      time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
	}

	service.cleanupLocalFiles(input, 70.0, "Post-processing")

	if len(fileRemover.removedFiles) != 0 {
		t.Errorf("expected no files removed at exact threshold, got %v", fileRemover.removedFiles)
	}
}

func TestComputeCleanupInput_NewlyProcessed(t *testing.T) {
	cfg := createTestConfig()

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{
			"/test/source/2025-12-28 10-06-16.mp4": true,
			// No trimmed or audio files exist
		}},
		&mockFileFinder{},
		cfg,
		&mockDiskChecker{usage: 50.0},
		&mockFileRemover{},
		&bytes.Buffer{},
	)

	result := service.computeCleanupInput(false, "/test/source/2025-12-28 10-06-16.mp4", time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	if !result.IsNewlyProcessed {
		t.Error("expected IsNewlyProcessed=true when no trimmed/audio files exist")
	}
}

func TestComputeCleanupInput_AlreadyProcessed(t *testing.T) {
	cfg := createTestConfig()

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{
			"/test/source/2025-12-28 10-06-16.mp4": true,
			"/test/trimmed/2025-12-28.mp4":         true,
			"/test/audio/2025-12-28.mp3":           true,
		}},
		&mockFileFinder{},
		cfg,
		&mockDiskChecker{usage: 50.0},
		&mockFileRemover{},
		&bytes.Buffer{},
	)

	result := service.computeCleanupInput(false, "/test/source/2025-12-28 10-06-16.mp4", time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	if result.IsNewlyProcessed {
		t.Error("expected IsNewlyProcessed=false when both trimmed and audio files exist")
	}
}

func TestComputeCleanupInput_AudioOnlyMode(t *testing.T) {
	cfg := createTestConfig()

	service := createTestServiceWithCleanup(
		newMockDriveClient(),
		&mockFileChecker{existingFiles: map[string]bool{
			"/test/source/2025-12-28 10-06-16.mp4": true,
			// Audio doesn't exist, trimmed does (irrelevant in audio-only mode)
			"/test/trimmed/2025-12-28.mp4": true,
		}},
		&mockFileFinder{},
		cfg,
		&mockDiskChecker{usage: 50.0},
		&mockFileRemover{},
		&bytes.Buffer{},
	)

	result := service.computeCleanupInput(true, "/test/source/2025-12-28 10-06-16.mp4", time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC))

	if !result.IsNewlyProcessed {
		t.Error("expected IsNewlyProcessed=true in audio-only mode when audio file doesn't exist")
	}
	if !result.SkipVideo {
		t.Error("expected SkipVideo=true")
	}
}

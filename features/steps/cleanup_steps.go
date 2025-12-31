//go:build integration

package steps

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	appdist "nac-service-media/application/distribution"
	"nac-service-media/domain/distribution"
	"nac-service-media/infrastructure/drive"

	googledrive "google.golang.org/api/drive/v3"

	"github.com/cucumber/godog"
)

// cleanupMockDriveService is a mock implementation for cleanup testing
// It implements the drive.DriveService interface and simulates storage behavior
type cleanupMockDriveService struct {
	files           []*googledrive.File
	storageLimit    int64
	storageUsage    int64
	deletedFileIDs  []string
	trashEmptied    bool
	shouldFail      bool
	failError       error
	permissionError bool
}

func (m *cleanupMockDriveService) ListFiles(ctx context.Context, query string, fields string, orderBy string) ([]*googledrive.File, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	if m.permissionError {
		return nil, fmt.Errorf("googleapi: Error 403: The user does not have permission")
	}

	// Filter out deleted files
	var result []*googledrive.File
	for _, f := range m.files {
		deleted := false
		for _, id := range m.deletedFileIDs {
			if f.Id == id {
				deleted = true
				break
			}
		}
		if !deleted {
			result = append(result, f)
		}
	}

	// Sort by name (oldest first by filename)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (m *cleanupMockDriveService) GetAbout(ctx context.Context, fields string) (*googledrive.About, error) {
	if m.shouldFail {
		return nil, m.failError
	}

	// Calculate current usage by subtracting deleted file sizes
	currentUsage := m.storageUsage
	for _, f := range m.files {
		for _, id := range m.deletedFileIDs {
			if f.Id == id {
				currentUsage -= f.Size
				break
			}
		}
	}

	return &googledrive.About{
		StorageQuota: &googledrive.AboutStorageQuota{
			Limit: m.storageLimit,
			Usage: currentUsage,
		},
	}, nil
}

func (m *cleanupMockDriveService) DeleteFile(ctx context.Context, fileID string) error {
	if m.shouldFail {
		return m.failError
	}
	m.deletedFileIDs = append(m.deletedFileIDs, fileID)
	return nil
}

func (m *cleanupMockDriveService) EmptyTrash(ctx context.Context) error {
	if m.shouldFail {
		return m.failError
	}
	m.trashEmptied = true
	return nil
}

func (m *cleanupMockDriveService) UploadFile(ctx context.Context, fileName, mimeType, folderID, localPath string) (*googledrive.File, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	return &googledrive.File{
		Id:          "uploaded-file-id",
		Name:        fileName,
		MimeType:    mimeType,
		Size:        1024,
		WebViewLink: "https://drive.google.com/file/d/uploaded-file-id/view",
	}, nil
}

func (m *cleanupMockDriveService) CreatePermission(ctx context.Context, fileID string, permission *googledrive.Permission) error {
	if m.shouldFail {
		return m.failError
	}
	return nil
}

// cleanupContext holds test state for cleanup scenarios
type cleanupContext struct {
	folderID      string
	client        *drive.Client
	mockService   *cleanupMockDriveService
	cleanupResult *distribution.CleanupResult
	files         []distribution.FileInfo
	err           error
	service       *appdist.CleanupService
}

// SharedCleanupContext is reset before each scenario via Before hook
var SharedCleanupContext *cleanupContext

func getCleanupContext() *cleanupContext {
	return SharedCleanupContext
}

func InitializeCleanupScenario(ctx *godog.ScenarioContext) {
	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		SharedCleanupContext = &cleanupContext{
			mockService: &cleanupMockDriveService{
				storageLimit: 15 * 1024 * 1024 * 1024, // 15 GB default
				storageUsage: 0,
			},
		}
		return c, nil
	})

	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		SharedCleanupContext = nil
		return c, nil
	})

	// Reuse steps from drive_steps.go that set folderID and credentials
	ctx.Step(`^there is (\d+) (GB|MB) of available storage$`, thereIsAvailableStorage)
	ctx.Step(`^the Services folder contains mp4 files:$`, theServicesFolderContainsMP4Files)
	ctx.Step(`^I ensure (\d+) (GB|MB) of space is available$`, iEnsureSpaceIsAvailable)
	ctx.Step(`^no files should be deleted$`, noFilesShouldBeDeleted)
	ctx.Step(`^the cleanup result should show (\d+) bytes freed$`, theCleanupResultShouldShowBytesFreed)
	ctx.Step(`^"([^"]*)" should be deleted$`, fileShouldBeDeleted)
	ctx.Step(`^the cleanup result should show (\d+) files? deleted$`, theCleanupResultShouldShowFilesDeleted)
	ctx.Step(`^I should receive an error about insufficient storage$`, iShouldReceiveAnErrorAboutInsufficientStorage)
	ctx.Step(`^I list mp4 files sorted by date$`, iListMP4FilesSortedByDate)
	ctx.Step(`^the files should be in order:$`, theFilesShouldBeInOrder)
}

func thereIsAvailableStorage(amount int, unit string) error {
	c := getCleanupContext()

	var bytes int64
	switch unit {
	case "GB":
		bytes = int64(amount) * 1024 * 1024 * 1024
	case "MB":
		bytes = int64(amount) * 1024 * 1024
	}

	// Set storage usage so that available = limit - usage = bytes
	c.mockService.storageUsage = c.mockService.storageLimit - bytes

	return nil
}

func theServicesFolderContainsMP4Files(table *godog.Table) error {
	c := getCleanupContext()
	c.mockService.files = []*googledrive.File{}

	for i, row := range table.Rows {
		if i == 0 {
			continue // Skip header row
		}
		name := row.Cells[0].Value
		var size int64
		fmt.Sscanf(row.Cells[1].Value, "%d", &size)

		c.mockService.files = append(c.mockService.files, &googledrive.File{
			Id:          fmt.Sprintf("file-%d", i),
			Name:        name,
			MimeType:    "video/mp4",
			Size:        size,
			CreatedTime: time.Now().Format(time.RFC3339),
		})
	}
	return nil
}

func iEnsureSpaceIsAvailable(amount int, unit string) error {
	c := getCleanupContext()

	var neededBytes int64
	switch unit {
	case "GB":
		neededBytes = int64(amount) * 1024 * 1024 * 1024
	case "MB":
		neededBytes = int64(amount) * 1024 * 1024
	}

	// Create client with mock service
	client, err := drive.NewClient(
		context.Background(),
		"",
		drive.WithDriveService(c.mockService),
	)
	if err != nil {
		return fmt.Errorf("failed to create client: %v", err)
	}
	c.client = client

	// Create cleanup service
	c.service = appdist.NewCleanupService(client, c.folderID)

	// Run the cleanup
	result, err := c.service.EnsureSpaceAvailable(context.Background(), neededBytes)
	c.cleanupResult = result
	c.err = err

	return nil
}

func noFilesShouldBeDeleted() error {
	c := getCleanupContext()
	if c.cleanupResult == nil {
		return fmt.Errorf("cleanup result is nil")
	}
	if len(c.cleanupResult.DeletedFiles) != 0 {
		return fmt.Errorf("expected no files deleted, but %d were deleted", len(c.cleanupResult.DeletedFiles))
	}
	return nil
}

func theCleanupResultShouldShowBytesFreed(bytes int64) error {
	c := getCleanupContext()
	if c.cleanupResult == nil {
		return fmt.Errorf("cleanup result is nil")
	}
	if c.cleanupResult.FreedBytes != bytes {
		return fmt.Errorf("expected %d bytes freed, got %d", bytes, c.cleanupResult.FreedBytes)
	}
	return nil
}

func fileShouldBeDeleted(filename string) error {
	c := getCleanupContext()
	if c.cleanupResult == nil {
		return fmt.Errorf("cleanup result is nil")
	}

	for _, df := range c.cleanupResult.DeletedFiles {
		if df.Name == filename {
			return nil
		}
	}

	deletedNames := make([]string, len(c.cleanupResult.DeletedFiles))
	for i, df := range c.cleanupResult.DeletedFiles {
		deletedNames[i] = df.Name
	}
	return fmt.Errorf("expected %q to be deleted, but only deleted: %v", filename, deletedNames)
}

func theCleanupResultShouldShowFilesDeleted(count int) error {
	c := getCleanupContext()
	if c.cleanupResult == nil {
		return fmt.Errorf("cleanup result is nil")
	}
	if len(c.cleanupResult.DeletedFiles) != count {
		return fmt.Errorf("expected %d files deleted, got %d", count, len(c.cleanupResult.DeletedFiles))
	}
	return nil
}

func iShouldReceiveAnErrorAboutInsufficientStorage() error {
	c := getCleanupContext()
	if c.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(c.err.Error(), "no mp4 files to delete") {
		return fmt.Errorf("expected error about insufficient storage, got: %v", c.err)
	}
	return nil
}

func iListMP4FilesSortedByDate() error {
	c := getCleanupContext()

	// Create client with mock service
	client, err := drive.NewClient(
		context.Background(),
		"",
		drive.WithDriveService(c.mockService),
	)
	if err != nil {
		return fmt.Errorf("failed to create client: %v", err)
	}
	c.client = client

	// Create cleanup service
	c.service = appdist.NewCleanupService(client, c.folderID)

	// List files
	files, err := c.service.ListMP4FilesSorted(context.Background())
	if err != nil {
		c.err = err
		return fmt.Errorf("failed to list files: %v", err)
	}
	c.files = files
	return nil
}

func theFilesShouldBeInOrder(table *godog.Table) error {
	c := getCleanupContext()

	var expectedOrder []string
	for i, row := range table.Rows {
		if i == 0 {
			continue // Skip header row
		}
		expectedOrder = append(expectedOrder, row.Cells[0].Value)
	}

	if len(c.files) != len(expectedOrder) {
		return fmt.Errorf("expected %d files, got %d", len(expectedOrder), len(c.files))
	}

	for i, expected := range expectedOrder {
		if c.files[i].Name != expected {
			actualOrder := make([]string, len(c.files))
			for j, f := range c.files {
				actualOrder[j] = f.Name
			}
			return fmt.Errorf("expected file at position %d to be %q, got %q. Full order: %v", i, expected, c.files[i].Name, actualOrder)
		}
	}

	return nil
}

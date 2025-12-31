//go:build integration

package steps

import (
	"context"
	"fmt"
	"os"
	"strings"

	appdist "nac-service-media/application/distribution"
	"nac-service-media/domain/distribution"
	"nac-service-media/infrastructure/drive"

	googledrive "google.golang.org/api/drive/v3"

	"github.com/cucumber/godog"
)

// uploadMockDriveService is a mock implementation for upload testing
type uploadMockDriveService struct {
	files            []*googledrive.File
	uploadedFiles    []*googledrive.File
	permissions      map[string]*googledrive.Permission
	shouldFail       bool
	failError        error
	permissionFail   bool
	storageLimit     int64
	storageUsage     int64
	deletedFileIDs   []string
	trashEmptied     bool
	permissionError  bool
	nextFileID       int
}

func newUploadMockDriveService() *uploadMockDriveService {
	return &uploadMockDriveService{
		permissions: make(map[string]*googledrive.Permission),
		nextFileID:  1,
	}
}

func (m *uploadMockDriveService) ListFiles(ctx context.Context, query string, fields string, orderBy string) ([]*googledrive.File, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	if m.permissionError {
		return nil, fmt.Errorf("googleapi: Error 403: The user does not have permission")
	}
	return m.files, nil
}

func (m *uploadMockDriveService) GetAbout(ctx context.Context, fields string) (*googledrive.About, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	return &googledrive.About{
		StorageQuota: &googledrive.AboutStorageQuota{
			Limit: m.storageLimit,
			Usage: m.storageUsage,
		},
	}, nil
}

func (m *uploadMockDriveService) DeleteFile(ctx context.Context, fileID string) error {
	if m.shouldFail {
		return m.failError
	}
	m.deletedFileIDs = append(m.deletedFileIDs, fileID)
	return nil
}

func (m *uploadMockDriveService) EmptyTrash(ctx context.Context) error {
	if m.shouldFail {
		return m.failError
	}
	m.trashEmptied = true
	return nil
}

func (m *uploadMockDriveService) UploadFile(ctx context.Context, fileName, mimeType, folderID, localPath string) (*googledrive.File, error) {
	if m.shouldFail {
		return nil, m.failError
	}

	// Check if file exists (for realistic testing)
	info, err := os.Stat(localPath)
	if err != nil {
		return nil, fmt.Errorf("unable to open file: %w", err)
	}

	fileID := fmt.Sprintf("uploaded-file-%d", m.nextFileID)
	m.nextFileID++

	file := &googledrive.File{
		Id:          fileID,
		Name:        fileName,
		MimeType:    mimeType,
		Size:        info.Size(),
		WebViewLink: fmt.Sprintf("https://drive.google.com/file/d/%s/view", fileID),
	}

	m.uploadedFiles = append(m.uploadedFiles, file)
	return file, nil
}

func (m *uploadMockDriveService) CreatePermission(ctx context.Context, fileID string, permission *googledrive.Permission) error {
	if m.permissionFail {
		return fmt.Errorf("permission API error: unable to set sharing permission")
	}
	if m.shouldFail {
		return m.failError
	}
	m.permissions[fileID] = permission
	return nil
}

// uploadContext holds test state for upload scenarios
type uploadContext struct {
	folderID           string
	client             *drive.Client
	mockService        *uploadMockDriveService
	uploadResult       *distribution.UploadResult
	distributionResult *appdist.DistributionResult
	err                error
	videoPath          string
	audioPath          string
	uploadedFileID     string
	service            *appdist.UploadService
}

// SharedUploadContext is reset before each scenario via Before hook
var SharedUploadContext *uploadContext

func getUploadContext() *uploadContext {
	return SharedUploadContext
}

func InitializeUploadScenario(ctx *godog.ScenarioContext) {
	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		SharedUploadContext = &uploadContext{
			mockService: newUploadMockDriveService(),
		}
		return c, nil
	})

	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		// Clean up test files if created
		if SharedUploadContext != nil {
			if SharedUploadContext.videoPath != "" {
				os.Remove(SharedUploadContext.videoPath)
			}
			if SharedUploadContext.audioPath != "" {
				os.Remove(SharedUploadContext.audioPath)
			}
		}
		SharedUploadContext = nil
		return c, nil
	})

	// Reuse folder ID step from drive_steps
	ctx.Step(`^the Services folder ID is "([^"]*)"$`, uploadTheServicesFolderIDIs)
	ctx.Step(`^valid Google Drive upload credentials$`, validGoogleDriveUploadCredentials)
	ctx.Step(`^I have a video file at "([^"]*)"$`, iHaveAVideoFileAt)
	ctx.Step(`^I have an audio file at "([^"]*)"$`, iHaveAnAudioFileAt)
	ctx.Step(`^I upload the video to the Services folder$`, iUploadTheVideoToTheServicesFolder)
	ctx.Step(`^I upload the audio to the Services folder$`, iUploadTheAudioToTheServicesFolder)
	ctx.Step(`^the upload should succeed$`, theUploadShouldSucceed)
	ctx.Step(`^I should receive a file ID$`, iShouldReceiveAFileID)
	ctx.Step(`^I have uploaded a file with ID "([^"]*)"$`, iHaveUploadedAFileWithID)
	ctx.Step(`^I set public sharing permission$`, iSetPublicSharingPermission)
	ctx.Step(`^the permission should be set successfully$`, thePermissionShouldBeSetSuccessfully)
	ctx.Step(`^I upload and share the video$`, iUploadAndShareTheVideo)
	ctx.Step(`^I should receive a shareable URL in the format "([^"]*)"$`, iShouldReceiveAShareableURLInTheFormat)
	ctx.Step(`^I distribute both files$`, iDistributeBothFiles)
	ctx.Step(`^both uploads should succeed$`, bothUploadsShouldSucceed)
	ctx.Step(`^I should receive shareable URLs for both files$`, iShouldReceiveShareableURLsForBothFiles)
	ctx.Step(`^I attempt to upload the video$`, iAttemptToUploadTheVideo)
	ctx.Step(`^I should receive an error about missing file$`, iShouldReceiveAnErrorAboutMissingFile)
	ctx.Step(`^the permission API will fail$`, thePermissionAPIWillFail)
	ctx.Step(`^I attempt to set public sharing permission$`, iAttemptToSetPublicSharingPermission)
	ctx.Step(`^I should receive an error about permission failure$`, iShouldReceiveAnErrorAboutPermissionFailure)
}

func uploadTheServicesFolderIDIs(folderID string) error {
	u := getUploadContext()
	u.folderID = folderID
	return nil
}

func validGoogleDriveUploadCredentials() error {
	u := getUploadContext()

	// Initialize client with mock service
	client, err := drive.NewClient(
		context.Background(),
		"",
		drive.WithDriveService(u.mockService),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize client: %v", err)
	}
	u.client = client
	u.service = appdist.NewUploadService(client, u.folderID)
	return nil
}

func iHaveAVideoFileAt(path string) error {
	u := getUploadContext()
	// Create a test file for the mock to find
	if !strings.Contains(path, "nonexistent") {
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("failed to create test file: %v", err)
		}
		// Write some content to make it non-empty
		f.WriteString("test video content")
		f.Close()
	}
	u.videoPath = path
	return nil
}

func iHaveAnAudioFileAt(path string) error {
	u := getUploadContext()
	// Create a test file for the mock to find
	if !strings.Contains(path, "nonexistent") {
		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("failed to create test file: %v", err)
		}
		// Write some content to make it non-empty
		f.WriteString("test audio content")
		f.Close()
	}
	u.audioPath = path
	return nil
}

func iUploadTheVideoToTheServicesFolder() error {
	u := getUploadContext()

	result, err := u.service.UploadVideo(context.Background(), u.videoPath)
	u.uploadResult = result
	u.err = err
	return nil
}

func iUploadTheAudioToTheServicesFolder() error {
	u := getUploadContext()

	result, err := u.service.UploadAudio(context.Background(), u.audioPath)
	u.uploadResult = result
	u.err = err
	return nil
}

func theUploadShouldSucceed() error {
	u := getUploadContext()
	if u.err != nil {
		return fmt.Errorf("expected upload to succeed, but got error: %v", u.err)
	}
	return nil
}

func iShouldReceiveAFileID() error {
	u := getUploadContext()
	if u.uploadResult == nil {
		return fmt.Errorf("no upload result")
	}
	if u.uploadResult.FileID == "" {
		return fmt.Errorf("expected file ID, got empty string")
	}
	return nil
}

func iHaveUploadedAFileWithID(fileID string) error {
	u := getUploadContext()
	u.uploadedFileID = fileID
	return nil
}

func iSetPublicSharingPermission() error {
	u := getUploadContext()
	u.err = u.client.SetPublicSharing(context.Background(), u.uploadedFileID)
	return nil
}

func thePermissionShouldBeSetSuccessfully() error {
	u := getUploadContext()
	if u.err != nil {
		return fmt.Errorf("expected permission to be set, but got error: %v", u.err)
	}
	// Check that permission was recorded in mock
	perm, ok := u.mockService.permissions[u.uploadedFileID]
	if !ok {
		return fmt.Errorf("permission not found for file %s", u.uploadedFileID)
	}
	if perm.Type != "anyone" || perm.Role != "reader" {
		return fmt.Errorf("expected anyone/reader permission, got %s/%s", perm.Type, perm.Role)
	}
	return nil
}

func iUploadAndShareTheVideo() error {
	u := getUploadContext()

	req := distribution.UploadRequest{
		LocalPath: u.videoPath,
		FileName:  "test-video.mp4",
		FolderID:  u.folderID,
		MimeType:  distribution.MimeTypeMP4,
	}

	result, err := u.client.UploadAndShare(context.Background(), req)
	u.uploadResult = result
	u.err = err
	return nil
}

func iShouldReceiveAShareableURLInTheFormat(format string) error {
	u := getUploadContext()
	if u.uploadResult == nil {
		return fmt.Errorf("no upload result")
	}
	if u.uploadResult.ShareableURL == "" {
		return fmt.Errorf("expected shareable URL, got empty string")
	}
	// Check that URL matches expected format (contains drive.google.com and view?usp=sharing)
	if !strings.Contains(u.uploadResult.ShareableURL, "drive.google.com/file/d/") {
		return fmt.Errorf("URL doesn't match expected format: %s", u.uploadResult.ShareableURL)
	}
	if !strings.Contains(u.uploadResult.ShareableURL, "/view?usp=sharing") {
		return fmt.Errorf("URL doesn't contain sharing params: %s", u.uploadResult.ShareableURL)
	}
	return nil
}

func iDistributeBothFiles() error {
	u := getUploadContext()

	result, err := u.service.Distribute(context.Background(), u.videoPath, u.audioPath)
	u.distributionResult = result
	u.err = err
	return nil
}

func bothUploadsShouldSucceed() error {
	u := getUploadContext()
	if u.err != nil {
		return fmt.Errorf("expected distribution to succeed, but got error: %v", u.err)
	}
	if u.distributionResult == nil {
		return fmt.Errorf("no distribution result")
	}
	return nil
}

func iShouldReceiveShareableURLsForBothFiles() error {
	u := getUploadContext()
	if u.distributionResult == nil {
		return fmt.Errorf("no distribution result")
	}
	if u.distributionResult.VideoURL == "" {
		return fmt.Errorf("expected video URL, got empty string")
	}
	if u.distributionResult.AudioURL == "" {
		return fmt.Errorf("expected audio URL, got empty string")
	}
	return nil
}

func iAttemptToUploadTheVideo() error {
	u := getUploadContext()

	result, err := u.service.UploadVideo(context.Background(), u.videoPath)
	u.uploadResult = result
	u.err = err
	return nil
}

func iShouldReceiveAnErrorAboutMissingFile() error {
	u := getUploadContext()
	if u.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(u.err.Error(), "does not exist") && !strings.Contains(u.err.Error(), "no such file") {
		return fmt.Errorf("expected error about missing file, got: %v", u.err)
	}
	return nil
}

func thePermissionAPIWillFail() error {
	u := getUploadContext()
	u.mockService.permissionFail = true
	return nil
}

func iAttemptToSetPublicSharingPermission() error {
	u := getUploadContext()
	u.err = u.client.SetPublicSharing(context.Background(), u.uploadedFileID)
	return nil
}

func iShouldReceiveAnErrorAboutPermissionFailure() error {
	u := getUploadContext()
	if u.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(u.err.Error(), "permission") && !strings.Contains(u.err.Error(), "sharing") {
		return fmt.Errorf("expected error about permission, got: %v", u.err)
	}
	return nil
}

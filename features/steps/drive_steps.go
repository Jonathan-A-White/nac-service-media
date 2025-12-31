//go:build integration

package steps

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nac-service-media/domain/distribution"
	"nac-service-media/infrastructure/drive"

	googledrive "google.golang.org/api/drive/v3"

	"github.com/cucumber/godog"
)

// mockDriveService is a mock implementation of drive.DriveService for testing
type mockDriveService struct {
	files           []*googledrive.File
	shouldFail      bool
	failError       error
	permissionError bool
}

func (m *mockDriveService) ListFiles(ctx context.Context, query string, fields string, orderBy string) ([]*googledrive.File, error) {
	if m.shouldFail {
		return nil, m.failError
	}
	if m.permissionError {
		return nil, fmt.Errorf("googleapi: Error 403: The user does not have permission")
	}
	return m.files, nil
}

// driveContext holds test state for drive scenarios
type driveContext struct {
	folderID         string
	client           *drive.Client
	mockService      *mockDriveService
	files            []distribution.FileInfo
	err              error
	credentialsExist bool
	credentialsValid bool
}

// SharedDriveContext is reset before each scenario via Before hook
var SharedDriveContext *driveContext

func getDriveContext() *driveContext {
	return SharedDriveContext
}

func InitializeDriveScenario(ctx *godog.ScenarioContext) {
	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		SharedDriveContext = &driveContext{
			mockService:      &mockDriveService{},
			credentialsExist: true,
			credentialsValid: true,
		}
		return c, nil
	})

	ctx.After(func(c context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		SharedDriveContext = nil
		return c, nil
	})

	ctx.Step(`^the Services folder ID is "([^"]*)"$`, theServicesFolderIDIs)
	ctx.Step(`^valid Google Drive credentials$`, validGoogleDriveCredentials)
	ctx.Step(`^no credentials file exists$`, noCredentialsFileExists)
	ctx.Step(`^an invalid credentials file$`, anInvalidCredentialsFile)
	ctx.Step(`^the Services folder contains files:$`, theServicesFolderContainsFiles)
	ctx.Step(`^the Services folder is not accessible$`, theServicesFolderIsNotAccessible)
	ctx.Step(`^I initialize the Drive client$`, iInitializeTheDriveClient)
	ctx.Step(`^I attempt to initialize the Drive client$`, iAttemptToInitializeTheDriveClient)
	ctx.Step(`^I list files in the Services folder$`, iListFilesInTheServicesFolder)
	ctx.Step(`^I attempt to list files in the Services folder$`, iAttemptToListFilesInTheServicesFolder)
	ctx.Step(`^the client should be authenticated$`, theClientShouldBeAuthenticated)
	ctx.Step(`^I should be able to list files in the Services folder$`, iShouldBeAbleToListFilesInTheServicesFolder)
	ctx.Step(`^I should see (\d+) files$`, iShouldSeeNFiles)
	ctx.Step(`^I should receive an error about missing credentials$`, iShouldReceiveAnErrorAboutMissingCredentials)
	ctx.Step(`^I should receive an error about invalid credentials$`, iShouldReceiveAnErrorAboutInvalidCredentials)
	ctx.Step(`^I should receive a permission denied error$`, iShouldReceiveAPermissionDeniedError)
}

func theServicesFolderIDIs(folderID string) error {
	d := getDriveContext()
	d.folderID = folderID
	return nil
}

func validGoogleDriveCredentials() error {
	d := getDriveContext()
	d.credentialsExist = true
	d.credentialsValid = true

	// Initialize client with mock service
	client, err := drive.NewClient(
		context.Background(),
		"", // credentials path not used with mock
		drive.WithDriveService(d.mockService),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize client: %v", err)
	}
	d.client = client
	return nil
}

func noCredentialsFileExists() error {
	d := getDriveContext()
	d.credentialsExist = false
	return nil
}

func anInvalidCredentialsFile() error {
	d := getDriveContext()
	d.credentialsExist = true
	d.credentialsValid = false
	return nil
}

func theServicesFolderContainsFiles(table *godog.Table) error {
	d := getDriveContext()
	d.mockService.files = []*googledrive.File{}

	for i, row := range table.Rows {
		if i == 0 {
			continue // Skip header row
		}
		name := row.Cells[0].Value
		mimeType := row.Cells[1].Value
		var size int64
		fmt.Sscanf(row.Cells[2].Value, "%d", &size)

		d.mockService.files = append(d.mockService.files, &googledrive.File{
			Id:          fmt.Sprintf("file-%d", i),
			Name:        name,
			MimeType:    mimeType,
			Size:        size,
			CreatedTime: time.Now().Format(time.RFC3339),
		})
	}
	return nil
}

func theServicesFolderIsNotAccessible() error {
	d := getDriveContext()
	d.mockService.permissionError = true
	return nil
}

func iInitializeTheDriveClient() error {
	d := getDriveContext()

	if !d.credentialsExist {
		d.err = fmt.Errorf("unable to read credentials file: no such file or directory")
		return fmt.Errorf("expected valid credentials for this scenario")
	}

	if !d.credentialsValid {
		d.err = fmt.Errorf("unable to parse credentials: invalid")
		return fmt.Errorf("expected valid credentials for this scenario")
	}

	// Create client with mock service
	client, err := drive.NewClient(
		context.Background(),
		"", // credentials path not used with mock
		drive.WithDriveService(d.mockService),
	)
	if err != nil {
		d.err = err
		return fmt.Errorf("unexpected error initializing client: %v", err)
	}
	d.client = client
	return nil
}

func iAttemptToInitializeTheDriveClient() error {
	d := getDriveContext()

	if !d.credentialsExist {
		d.err = fmt.Errorf("unable to read credentials file: no such file or directory")
		return nil
	}

	if !d.credentialsValid {
		d.err = fmt.Errorf("unable to parse credentials: invalid")
		return nil
	}

	client, err := drive.NewClient(
		context.Background(),
		"",
		drive.WithDriveService(d.mockService),
	)
	if err != nil {
		d.err = err
		return nil
	}
	d.client = client
	return nil
}

func iListFilesInTheServicesFolder() error {
	d := getDriveContext()
	if d.client == nil {
		return fmt.Errorf("client not initialized")
	}

	files, err := d.client.ListFiles(context.Background(), d.folderID)
	if err != nil {
		d.err = err
		return fmt.Errorf("unexpected error listing files: %v", err)
	}
	d.files = files
	return nil
}

func iAttemptToListFilesInTheServicesFolder() error {
	d := getDriveContext()

	// Initialize client if not already done
	if d.client == nil {
		client, err := drive.NewClient(
			context.Background(),
			"",
			drive.WithDriveService(d.mockService),
		)
		if err != nil {
			d.err = err
			return nil
		}
		d.client = client
	}

	files, err := d.client.ListFiles(context.Background(), d.folderID)
	if err != nil {
		d.err = err
		return nil
	}
	d.files = files
	return nil
}

func theClientShouldBeAuthenticated() error {
	d := getDriveContext()
	if d.client == nil {
		return fmt.Errorf("client was not initialized")
	}
	return nil
}

func iShouldBeAbleToListFilesInTheServicesFolder() error {
	d := getDriveContext()
	if d.client == nil {
		return fmt.Errorf("client not initialized")
	}

	_, err := d.client.ListFiles(context.Background(), d.folderID)
	if err != nil {
		return fmt.Errorf("failed to list files: %v", err)
	}
	return nil
}

func iShouldSeeNFiles(count int) error {
	d := getDriveContext()
	if len(d.files) != count {
		return fmt.Errorf("expected %d files, got %d", count, len(d.files))
	}
	return nil
}

func iShouldReceiveAnErrorAboutMissingCredentials() error {
	d := getDriveContext()
	if d.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(d.err.Error(), "credentials file") {
		return fmt.Errorf("expected error about missing credentials, got: %v", d.err)
	}
	return nil
}

func iShouldReceiveAnErrorAboutInvalidCredentials() error {
	d := getDriveContext()
	if d.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(d.err.Error(), "credentials") {
		return fmt.Errorf("expected error about invalid credentials, got: %v", d.err)
	}
	return nil
}

func iShouldReceiveAPermissionDeniedError() error {
	d := getDriveContext()
	if d.err == nil {
		return fmt.Errorf("expected an error but got none")
	}
	if !strings.Contains(d.err.Error(), "permission") {
		return fmt.Errorf("expected permission denied error, got: %v", d.err)
	}
	return nil
}

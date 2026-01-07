package drive

import (
	"context"
	"fmt"
	"os"
	"time"

	"nac-service-media/domain/distribution"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// DriveService defines the interface for Google Drive API operations
// This allows mocking the Google Drive API in tests
type DriveService interface {
	ListFiles(ctx context.Context, query string, fields string, orderBy string) ([]*drive.File, error)
	GetAbout(ctx context.Context, fields string) (*drive.About, error)
	DeleteFile(ctx context.Context, fileID string) error
	EmptyTrash(ctx context.Context) error
	UploadFile(ctx context.Context, fileName, mimeType, folderID, localPath string) (*drive.File, error)
	CreatePermission(ctx context.Context, fileID string, permission *drive.Permission) error
}

// GoogleDriveService is the production implementation using the Google Drive API
type GoogleDriveService struct {
	service *drive.Service
}

// ListFiles lists files matching the query
func (s *GoogleDriveService) ListFiles(ctx context.Context, query string, fields string, orderBy string) ([]*drive.File, error) {
	r, err := s.service.Files.List().
		Q(query).
		Fields(googleapi.Field("files(" + fields + ")")).
		OrderBy(orderBy).
		Context(ctx).
		Do()
	if err != nil {
		return nil, err
	}
	return r.Files, nil
}

// GetAbout gets information about the user's Drive
func (s *GoogleDriveService) GetAbout(ctx context.Context, fields string) (*drive.About, error) {
	return s.service.About.Get().Fields(googleapi.Field(fields)).Context(ctx).Do()
}

// DeleteFile deletes a file permanently (bypasses trash)
func (s *GoogleDriveService) DeleteFile(ctx context.Context, fileID string) error {
	return s.service.Files.Delete(fileID).Context(ctx).Do()
}

// EmptyTrash empties the trash
func (s *GoogleDriveService) EmptyTrash(ctx context.Context) error {
	return s.service.Files.EmptyTrash().Context(ctx).Do()
}

// UploadFile uploads a file to Google Drive
func (s *GoogleDriveService) UploadFile(ctx context.Context, fileName, mimeType, folderID, localPath string) (*drive.File, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return nil, fmt.Errorf("unable to open file: %w", err)
	}
	defer f.Close()

	fileMetadata := &drive.File{
		Name:     fileName,
		Parents:  []string{folderID},
		MimeType: mimeType,
	}

	file, err := s.service.Files.Create(fileMetadata).
		Media(f).
		Fields("id, name, size, webViewLink").
		Context(ctx).
		Do()
	if err != nil {
		return nil, fmt.Errorf("unable to upload file: %w", err)
	}

	return file, nil
}

// CreatePermission creates a permission on a file
func (s *GoogleDriveService) CreatePermission(ctx context.Context, fileID string, permission *drive.Permission) error {
	_, err := s.service.Permissions.Create(fileID, permission).Context(ctx).Do()
	return err
}

// Client implements distribution.DriveClient using Google Drive API
type Client struct {
	driveService DriveService
}

// ClientOption is a functional option for configuring Client
type ClientOption func(*Client)

// WithDriveService sets a custom drive service (for testing)
func WithDriveService(svc DriveService) ClientOption {
	return func(c *Client) {
		c.driveService = svc
	}
}

// NewClient creates a new Google Drive client
// If no options are provided, it initializes a real Google Drive service
func NewClient(ctx context.Context, credentialsPath string, opts ...ClientOption) (*Client, error) {
	c := &Client{}

	for _, opt := range opts {
		opt(c)
	}

	// If no custom drive service was provided, create a real one
	if c.driveService == nil {
		svc, err := newGoogleDriveService(ctx, credentialsPath)
		if err != nil {
			return nil, err
		}
		c.driveService = svc
	}

	return c, nil
}

// newGoogleDriveService creates a production Google Drive service
func newGoogleDriveService(ctx context.Context, credentialsPath string) (*GoogleDriveService, error) {
	b, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read credentials file: %w", err)
	}

	config, err := google.JWTConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		return nil, fmt.Errorf("unable to parse credentials: %w", err)
	}

	client := config.Client(ctx)
	srv, err := drive.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("unable to create drive service: %w", err)
	}

	return &GoogleDriveService{service: srv}, nil
}

// ListFiles implements distribution.DriveClient
func (c *Client) ListFiles(ctx context.Context, folderID string) ([]distribution.FileInfo, error) {
	query := fmt.Sprintf("'%s' in parents and trashed = false", folderID)
	files, err := c.driveService.ListFiles(ctx, query, "id, name, mimeType, size, createdTime", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	var result []distribution.FileInfo
	for _, f := range files {
		createdTime := parseTime(f.CreatedTime)
		result = append(result, distribution.FileInfo{
			ID:          f.Id,
			Name:        f.Name,
			MimeType:    f.MimeType,
			Size:        f.Size,
			CreatedTime: createdTime,
		})
	}
	return result, nil
}

// FindFileByName implements distribution.DriveClient
// Returns nil, nil if no file with the given name exists
func (c *Client) FindFileByName(ctx context.Context, folderID, fileName string) (*distribution.FileInfo, error) {
	// Use Drive API query to filter by exact name
	query := fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false", folderID, fileName)
	files, err := c.driveService.ListFiles(ctx, query, "id, name, mimeType, size, createdTime", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to find file by name: %w", err)
	}

	if len(files) == 0 {
		return nil, nil // Not found is not an error
	}

	// Return first match (should only be one with exact name match)
	f := files[0]
	return &distribution.FileInfo{
		ID:          f.Id,
		Name:        f.Name,
		MimeType:    f.MimeType,
		Size:        f.Size,
		CreatedTime: parseTime(f.CreatedTime),
	}, nil
}

// parseTime parses a Google Drive timestamp string
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// GetStorageQuota implements distribution.DriveClient
func (c *Client) GetStorageQuota(ctx context.Context) (*distribution.StorageInfo, error) {
	about, err := c.driveService.GetAbout(ctx, "storageQuota")
	if err != nil {
		return nil, fmt.Errorf("unable to get storage info: %w", err)
	}

	total := about.StorageQuota.Limit
	used := about.StorageQuota.Usage

	return &distribution.StorageInfo{
		TotalBytes:     total,
		UsedBytes:      used,
		AvailableBytes: total - used,
	}, nil
}

// ListMP4Files implements distribution.DriveClient
// Returns MP4 files sorted by filename (oldest first)
// Handles both "YYYY-MM-DD.mp4" and "YYYY-MM-DD HH-MM-SS.mp4" formats
func (c *Client) ListMP4Files(ctx context.Context, folderID string) ([]distribution.FileInfo, error) {
	query := fmt.Sprintf("'%s' in parents and mimeType='video/mp4' and trashed=false", folderID)
	files, err := c.driveService.ListFiles(ctx, query, "id, name, mimeType, size, createdTime", "name")
	if err != nil {
		return nil, fmt.Errorf("failed to list mp4 files: %w", err)
	}

	var result []distribution.FileInfo
	for _, f := range files {
		createdTime := parseTime(f.CreatedTime)
		result = append(result, distribution.FileInfo{
			ID:          f.Id,
			Name:        f.Name,
			MimeType:    f.MimeType,
			Size:        f.Size,
			CreatedTime: createdTime,
		})
	}

	// Files are already sorted by name from Google Drive API
	// Both formats (YYYY-MM-DD.mp4 and YYYY-MM-DD HH-MM-SS.mp4) sort correctly alphabetically
	return result, nil
}

// DeletePermanently implements distribution.DriveClient
func (c *Client) DeletePermanently(ctx context.Context, fileID string) error {
	if err := c.driveService.DeleteFile(ctx, fileID); err != nil {
		return fmt.Errorf("unable to delete file: %w", err)
	}
	return nil
}

// EmptyTrash implements distribution.DriveClient
func (c *Client) EmptyTrash(ctx context.Context) error {
	if err := c.driveService.EmptyTrash(ctx); err != nil {
		return fmt.Errorf("unable to empty trash: %w", err)
	}
	return nil
}

// Upload implements distribution.DriveClient
func (c *Client) Upload(ctx context.Context, req distribution.UploadRequest) (*distribution.UploadResult, error) {
	file, err := c.driveService.UploadFile(ctx, req.FileName, req.MimeType, req.FolderID, req.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file: %w", err)
	}

	return &distribution.UploadResult{
		FileID:       file.Id,
		FileName:     file.Name,
		ShareableURL: file.WebViewLink,
		Size:         file.Size,
	}, nil
}

// SetPublicSharing implements distribution.DriveClient
func (c *Client) SetPublicSharing(ctx context.Context, fileID string) error {
	permission := &drive.Permission{
		Type: "anyone",
		Role: "reader",
	}

	if err := c.driveService.CreatePermission(ctx, fileID, permission); err != nil {
		return fmt.Errorf("unable to set sharing permission: %w", err)
	}
	return nil
}

// UploadAndShare implements distribution.DriveClient
func (c *Client) UploadAndShare(ctx context.Context, req distribution.UploadRequest) (*distribution.UploadResult, error) {
	result, err := c.Upload(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := c.SetPublicSharing(ctx, result.FileID); err != nil {
		return nil, fmt.Errorf("uploaded but failed to set sharing: %w", err)
	}

	// Generate proper sharing URL
	result.ShareableURL = fmt.Sprintf("https://drive.google.com/file/d/%s/view?usp=sharing", result.FileID)
	return result, nil
}

// Ensure Client implements distribution.DriveClient
var _ distribution.DriveClient = (*Client)(nil)

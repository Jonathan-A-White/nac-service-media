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

// parseTime parses a Google Drive timestamp string
func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Ensure Client implements distribution.DriveClient
var _ distribution.DriveClient = (*Client)(nil)

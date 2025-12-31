//go:build manual

package drive

import (
	"context"
	"fmt"
	"os"
	"testing"

	"nac-service-media/domain/distribution"
)

// TestRealDriveConnectivity tests real Google Drive connectivity
// Run with: go test -tags=manual -v ./infrastructure/drive/... -run TestRealDriveConnectivity
func TestRealDriveConnectivity(t *testing.T) {
	credentialsPath := "../../credentials.json"
	folderID := "1dPV078FlLsWUFGjjoq3-epJiY_tBGXC8" // Services folder from epic

	// Check credentials file exists
	if _, err := os.Stat(credentialsPath); os.IsNotExist(err) {
		t.Skip("credentials.json not found - skipping real Drive test")
	}

	ctx := context.Background()

	// Create real client
	client, err := NewClient(ctx, credentialsPath)
	if err != nil {
		t.Fatalf("Failed to create Drive client: %v", err)
	}

	// List files in Services folder
	files, err := client.ListFiles(ctx, folderID)
	if err != nil {
		t.Fatalf("Failed to list files: %v", err)
	}

	fmt.Printf("\n=== Google Drive Connectivity Test ===\n")
	fmt.Printf("Successfully connected to Google Drive!\n")
	fmt.Printf("Found %d files in Services folder:\n\n", len(files))

	for _, f := range files {
		sizeMB := float64(f.Size) / 1024 / 1024
		fmt.Printf("  - %s (%s, %.2f MB)\n", f.Name, f.MimeType, sizeMB)
	}
	fmt.Println()
}

// TestRealDriveUploadAndShare tests real Google Drive upload with sharing
// Run with: go test -tags=manual -v ./infrastructure/drive/... -run TestRealDriveUploadAndShare
// This test uploads a small test file, verifies sharing works, then deletes it
func TestRealDriveUploadAndShare(t *testing.T) {
	credentialsPath := "../../oauth_credentials.json"
	tokenPath := "../../drive_token.json"
	folderID := "1dPV078FlLsWUFGjjoq3-epJiY_tBGXC8" // Services folder from epic

	// Check credentials file exists
	if _, err := os.Stat(credentialsPath); os.IsNotExist(err) {
		t.Skip("oauth_credentials.json not found - skipping real Drive upload test")
	}

	ctx := context.Background()

	// Create real client with OAuth
	client, err := NewClientWithOAuth(ctx, credentialsPath, tokenPath)
	if err != nil {
		t.Fatalf("Failed to create Drive client: %v", err)
	}

	// Create a small test file
	testFilePath := "/tmp/test-upload-integration.txt"
	testContent := []byte("Test upload from nac-service-media integration test")
	if err := os.WriteFile(testFilePath, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(testFilePath)

	fmt.Printf("\n=== Google Drive Upload Test ===\n")

	// Test upload
	fmt.Printf("Uploading test file...\n")
	uploadReq := distribution.UploadRequest{
		LocalPath: testFilePath,
		FileName:  "test-upload-integration.txt",
		FolderID:  folderID,
		MimeType:  "text/plain",
	}

	result, err := client.Upload(ctx, uploadReq)
	if err != nil {
		t.Fatalf("Failed to upload file: %v", err)
	}
	fmt.Printf("Upload successful! File ID: %s\n", result.FileID)

	// Test set public sharing
	fmt.Printf("Setting public sharing permission...\n")
	if err := client.SetPublicSharing(ctx, result.FileID); err != nil {
		// Clean up even on failure
		client.DeletePermanently(ctx, result.FileID)
		t.Fatalf("Failed to set sharing: %v", err)
	}
	fmt.Printf("Sharing permission set successfully!\n")
	fmt.Printf("Shareable URL: https://drive.google.com/file/d/%s/view?usp=sharing\n", result.FileID)

	// Clean up - delete the test file
	fmt.Printf("Cleaning up test file...\n")
	if err := client.DeletePermanently(ctx, result.FileID); err != nil {
		t.Errorf("Failed to clean up test file: %v", err)
	}
	fmt.Printf("Test file deleted successfully!\n\n")
}

// TestRealDriveUploadVideo tests uploading a real video file
// Run with: go test -tags=manual -v ./infrastructure/drive/... -run TestRealDriveUploadVideo
// Note: This uploads the actual test video, so use with caution!
func TestRealDriveUploadVideo(t *testing.T) {
	credentialsPath := "../../oauth_credentials.json"
	tokenPath := "../../drive_token.json"
	folderID := "1dPV078FlLsWUFGjjoq3-epJiY_tBGXC8" // Services folder from epic

	// Use the real trimmed video from test data
	videoPath := "/mnt/c/Users/jonat/Downloads/Videos/Trimmed/2025-12-28.mp4"

	// Check credentials file exists
	if _, err := os.Stat(credentialsPath); os.IsNotExist(err) {
		t.Skip("oauth_credentials.json not found - skipping real Drive video upload test")
	}

	// Check video file exists
	if _, err := os.Stat(videoPath); os.IsNotExist(err) {
		t.Skipf("Video file not found at %s - skipping real Drive video upload test", videoPath)
	}

	ctx := context.Background()

	// Create real client with OAuth
	client, err := NewClientWithOAuth(ctx, credentialsPath, tokenPath)
	if err != nil {
		t.Fatalf("Failed to create Drive client: %v", err)
	}

	fmt.Printf("\n=== Google Drive Video Upload Test ===\n")

	// Test upload and share
	fmt.Printf("Uploading video: %s...\n", videoPath)
	uploadReq := distribution.UploadRequest{
		LocalPath: videoPath,
		FileName:  "2025-12-28.mp4",
		FolderID:  folderID,
		MimeType:  distribution.MimeTypeMP4,
	}

	result, err := client.UploadAndShare(ctx, uploadReq)
	if err != nil {
		t.Fatalf("Failed to upload and share video: %v", err)
	}

	fmt.Printf("Video upload successful!\n")
	fmt.Printf("  File ID: %s\n", result.FileID)
	fmt.Printf("  Size: %.2f MB\n", float64(result.Size)/1024/1024)
	fmt.Printf("  Shareable URL: %s\n", result.ShareableURL)
	fmt.Println()

	// Note: This test does NOT delete the file - it's a real upload for testing
}

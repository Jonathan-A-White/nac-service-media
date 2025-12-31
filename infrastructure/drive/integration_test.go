//go:build manual

package drive

import (
	"context"
	"fmt"
	"os"
	"testing"
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

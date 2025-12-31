package distribution

// UploadRequest contains the parameters needed to upload a file to Google Drive
type UploadRequest struct {
	LocalPath string // Full path to the local file
	FileName  string // Target filename in Google Drive
	FolderID  string // Target folder ID in Google Drive
	MimeType  string // MIME type of the file
}

// UploadResult contains the result of a successful upload
type UploadResult struct {
	FileID       string // Google Drive file ID
	FileName     string // Name of the uploaded file
	ShareableURL string // URL for sharing the file
	Size         int64  // Size of the uploaded file in bytes
}

// MIME type constants for common media formats
const (
	MimeTypeMP4 = "video/mp4"
	MimeTypeMP3 = "audio/mpeg"
)

package distribution

// CleanupResult contains information about files deleted during cleanup
type CleanupResult struct {
	DeletedFiles []DeletedFile
	FreedBytes   int64
}

// DeletedFile represents a file that was deleted
type DeletedFile struct {
	Name string
	Size int64
}

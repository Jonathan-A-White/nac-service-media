package filesystem

// DiskChecker reports filesystem disk usage
type DiskChecker interface {
	// UsagePercent returns the percentage of disk used (0-100) for the
	// filesystem containing the given path
	UsagePercent(path string) (float64, error)
}

// FileRemover deletes files from the filesystem
type FileRemover interface {
	// Remove deletes the file at the given path
	Remove(path string) error
}

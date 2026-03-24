package filesystem

import (
	"syscall"

	domainfs "nac-service-media/domain/filesystem"
)

// DiskUsageChecker implements filesystem.DiskChecker using syscall.Statfs
type DiskUsageChecker struct{}

// NewDiskUsageChecker creates a new DiskUsageChecker
func NewDiskUsageChecker() *DiskUsageChecker {
	return &DiskUsageChecker{}
}

// UsagePercent returns the disk usage percentage for the filesystem containing path
func (d *DiskUsageChecker) UsagePercent(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	total := stat.Blocks * uint64(stat.Bsize)
	if total == 0 {
		return 0, nil
	}
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free
	return float64(used) / float64(total) * 100, nil
}

// Ensure DiskUsageChecker implements the domain interface
var _ domainfs.DiskChecker = (*DiskUsageChecker)(nil)

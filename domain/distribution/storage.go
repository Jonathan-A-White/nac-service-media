package distribution

// StorageInfo represents Google Drive storage quota information
type StorageInfo struct {
	TotalBytes     int64
	UsedBytes      int64
	AvailableBytes int64
}

// HasSpaceFor returns true if there's enough space for the given bytes
func (s StorageInfo) HasSpaceFor(bytes int64) bool {
	return s.AvailableBytes >= bytes
}

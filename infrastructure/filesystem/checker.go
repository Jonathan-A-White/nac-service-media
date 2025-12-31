package filesystem

import (
	"os"

	"nac-service-media/domain/video"
)

// Checker implements video.FileChecker using the os package
type Checker struct{}

// NewChecker creates a new filesystem checker
func NewChecker() *Checker {
	return &Checker{}
}

// Exists returns true if the file exists
func (c *Checker) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Ensure Checker implements video.FileChecker
var _ video.FileChecker = (*Checker)(nil)

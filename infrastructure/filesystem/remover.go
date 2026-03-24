package filesystem

import (
	"os"

	domainfs "nac-service-media/domain/filesystem"
)

// Remover implements filesystem.FileRemover using os.Remove
type Remover struct{}

// NewRemover creates a new Remover
func NewRemover() *Remover {
	return &Remover{}
}

// Remove deletes the file at the given path
func (r *Remover) Remove(path string) error {
	return os.Remove(path)
}

// Ensure Remover implements the domain interface
var _ domainfs.FileRemover = (*Remover)(nil)

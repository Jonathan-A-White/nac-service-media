// Package lock provides a process-level file lock so only one instance of a
// long-running command runs at a time. Used by the process command to defend
// against duplicate scheduled-task invocations firing concurrently.
package lock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ErrAlreadyHeld is returned when the lock is held by another process.
var ErrAlreadyHeld = errors.New("lock already held by another process")

// FileLock holds an advisory flock on a file. Release on process exit; the
// kernel also releases the lock automatically if the process is killed.
type FileLock struct {
	file *os.File
}

// Acquire opens path (creating it if needed) and takes an exclusive,
// non-blocking advisory lock. Returns ErrAlreadyHeld if another process holds
// the lock.
func Acquire(path string) (*FileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create lock directory: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrAlreadyHeld
		}
		return nil, fmt.Errorf("acquire flock: %w", err)
	}

	return &FileLock{file: f}, nil
}

// Release unlocks and closes the lock file. Safe to call multiple times.
func (l *FileLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	err := l.file.Close()
	l.file = nil
	return err
}

package lock

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquire_Succeeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	l, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer l.Release()
}

func TestAcquire_BlocksSecondCaller(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	first, err := Acquire(path)
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	defer first.Release()

	second, err := Acquire(path)
	if err == nil {
		second.Release()
		t.Fatal("expected ErrAlreadyHeld, got nil")
	}
	if !errors.Is(err, ErrAlreadyHeld) {
		t.Errorf("got error %v, want ErrAlreadyHeld", err)
	}
}

func TestAcquire_AfterRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	first, err := Acquire(path)
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	second, err := Acquire(path)
	if err != nil {
		t.Fatalf("second Acquire() after release error = %v", err)
	}
	defer second.Release()
}

func TestRelease_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")

	l, err := Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("first Release() error = %v", err)
	}
	if err := l.Release(); err != nil {
		t.Errorf("second Release() error = %v, want nil", err)
	}
}

func TestRelease_NilSafe(t *testing.T) {
	var l *FileLock
	if err := l.Release(); err != nil {
		t.Errorf("Release() on nil error = %v, want nil", err)
	}
}

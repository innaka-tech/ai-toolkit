// Package fsx provides atomic file writes and a cross-process lock.
package fsx

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// WriteFile writes data atomically: temp file in the same directory, fsync, rename.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".aitk-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return rename(name, path)
}

// WriteFileKeep writes data atomically to an existing user-owned file without changing how it
// is set up: a symlink is followed and its target updated, and the file keeps its permissions.
// New files get perm.
func WriteFileKeep(path string, data []byte, perm os.FileMode) error {
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}
	if info, err := os.Stat(target); err == nil {
		perm = info.Mode().Perm()
	}
	return WriteFile(target, data, perm)
}

// Exists reports whether path exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Lock is an exclusive, kernel-managed file lock (flock on Unix, LockFileEx on Windows).
// The kernel releases it when the process exits, so there are no stale locks to break.
type Lock struct{ f *os.File }

// ErrLocked is returned when another process holds the lock past the timeout.
var ErrLocked = errors.New("locked by another aitk process")

// Acquire takes the lock at path, waiting up to timeout.
func Acquire(path string, timeout time.Duration) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		ok, err := tryLock(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		if ok {
			return &Lock{f: f}, nil
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, ErrLocked
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Release drops the lock.
func (l *Lock) Release() {
	unlock(l.f)
	l.f.Close()
}

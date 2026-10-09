package fsx

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestLockSerializesReadModifyWrite: 50 goroutines append through read-modify-write;
// with a correct lock no update is lost.
func TestLockSerializesReadModifyWrite(t *testing.T) {
	dir := t.TempDir()
	lock, data := filepath.Join(dir, "lock"), filepath.Join(dir, "data")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			l, err := Acquire(lock, 30*time.Second)
			if err != nil {
				t.Error(err)
				return
			}
			defer l.Release()
			b, _ := os.ReadFile(data)
			if err := WriteFile(data, append(b, []byte(strconv.Itoa(i)+"\n")...), 0o644); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	b, _ := os.ReadFile(data)
	if n := len(strings.Fields(string(b))); n != 50 {
		t.Fatalf("lost updates: %d of 50 lines", n)
	}
}

func TestLockTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	l, err := Acquire(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()
	if _, err := Acquire(path, 50*time.Millisecond); err != ErrLocked {
		t.Fatalf("expected ErrLocked, got %v", err)
	}
}

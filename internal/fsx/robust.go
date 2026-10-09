package fsx

import (
	"os"
	"runtime"
	"time"
)

// On Windows, antivirus and indexers briefly hold handles to new files, so
// rename and remove fail transiently with "Access is denied". Retry with
// backoff, as cmd/go's robustio does. Other platforms do not retry.
const retryFor = 2 * time.Second

func retry(op func() error) error {
	err := op()
	if err == nil || runtime.GOOS != "windows" {
		return err
	}
	deadline := time.Now().Add(retryFor)
	for wait := 5 * time.Millisecond; time.Now().Before(deadline); wait *= 2 {
		time.Sleep(wait)
		if err = op(); err == nil {
			return nil
		}
		if wait > 200*time.Millisecond {
			wait = 200 * time.Millisecond
		}
	}
	return err
}

func rename(from, to string) error { return retry(func() error { return os.Rename(from, to) }) }

func remove(path string) error {
	return retry(func() error {
		err := os.Remove(path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	})
}

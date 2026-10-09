//go:build windows

package fsx

// processAlive is conservative on Windows: locks are only broken by age.
func processAlive(pid int) bool { return true }

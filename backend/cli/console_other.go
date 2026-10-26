//go:build !windows
// +build !windows

package cli

// AttachConsoleIfNeeded is a no-op on non-Windows platforms.
func AttachConsoleIfNeeded() {}

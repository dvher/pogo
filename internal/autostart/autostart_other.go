//go:build !linux && !windows

package autostart

// Supported reports whether this platform can start Pogo at login.
func Supported() bool { return false }

// Enabled reports whether Pogo starts at login.
func Enabled() (bool, error) { return false, nil }

// Set adds or removes the autostart entry.
func Set(bool) error { return ErrUnsupported }

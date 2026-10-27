// Package autostart starts Pogo when the user logs in. The OS entry itself
// is the source of truth, so the setting follows the user removing it by
// hand (e.g. in GNOME's Startup Applications or Windows' Task Manager).
package autostart

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrUnsupported is returned by Set on platforms without autostart support.
var ErrUnsupported = errors.New("starting with the computer isn't supported on this platform")

// executable returns the absolute path of the running binary.
func executable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Abs(exe)
}

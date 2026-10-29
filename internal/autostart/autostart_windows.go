package autostart

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName = "Pogo"
	// Task Manager's "Startup apps" disables entries here rather than
	// removing them from the Run key.
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

// Supported reports whether this platform can start Pogo at login.
func Supported() bool { return true }

// Enabled reports whether Pogo starts at login.
func Enabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false, err
	}
	defer k.Close()
	if _, _, err := k.GetStringValue(valueName); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return !disabledInTaskManager(), nil
}

// disabledInTaskManager reports whether the user turned the entry off in Task
// Manager. The first byte of its StartupApproved value is odd when disabled.
func disabledInTaskManager() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetBinaryValue(valueName)
	return err == nil && len(v) > 0 && v[0]&1 == 1
}

// Set adds or removes the Run entry.
func Set(enabled bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	// Drop any Task Manager override so the new choice takes effect.
	if a, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.SET_VALUE); err == nil {
		a.DeleteValue(valueName)
		a.Close()
	}
	if !enabled {
		if err := k.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	return k.SetStringValue(valueName, `"`+exe+`"`)
}

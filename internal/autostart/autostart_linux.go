package autostart

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Supported reports whether this platform can start Pogo at login.
func Supported() bool { return true }

// entryPath is the XDG autostart entry, which GNOME, KDE and most other
// desktops (and niri via niri-session) run at login.
func entryPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "autostart", "pogo.desktop"), nil
}

// Enabled reports whether Pogo starts at login.
func Enabled() (bool, error) {
	p, err := entryPath()
	if err != nil {
		return false, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Desktops treat Hidden=true or a disabled GNOME flag as "removed".
	for line := range strings.Lines(string(data)) {
		switch strings.TrimSpace(line) {
		case "Hidden=true", "X-GNOME-Autostart-enabled=false":
			return false, nil
		}
	}
	return true, nil
}

// Set adds or removes the autostart entry.
func Set(enabled bool) error {
	p, err := entryPath()
	if err != nil {
		return err
	}
	if !enabled {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	exe, err := executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(desktopEntry(exe)), 0o644)
}

func desktopEntry(exe string) string {
	return `[Desktop Entry]
Type=Application
Name=Pogo
Comment=Markdown sticky notes that float above your windows
Exec=` + quoteExec(exe) + `
Icon=pogo
Terminal=false
X-GNOME-Autostart-enabled=true
`
}

// quoteExec quotes a path for a desktop entry's Exec key, following the
// Desktop Entry Specification.
func quoteExec(path string) string {
	path = strings.ReplaceAll(path, "%", "%%")
	if !strings.ContainsAny(path, " \t\n\"'\\><~|&;$*?#()`") {
		return path
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`)
	return `"` + r.Replace(path) + `"`
}

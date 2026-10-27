package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetAndEnabled(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	entry := filepath.Join(cfg, "autostart", "pogo.desktop")

	if on, err := Enabled(); err != nil || on {
		t.Fatalf("Enabled() before Set = %v, %v; want false", on, err)
	}
	if err := Set(true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := executable()
	if !strings.Contains(string(data), "\nExec="+quoteExec(exe)+"\n") {
		t.Errorf("entry doesn't run %s:\n%s", exe, data)
	}
	if on, err := Enabled(); err != nil || !on {
		t.Fatalf("Enabled() after Set(true) = %v, %v; want true", on, err)
	}

	// Turned off in the desktop's own startup settings.
	os.WriteFile(entry, append(data, "Hidden=true\n"...), 0o644)
	if on, _ := Enabled(); on {
		t.Error("Enabled() = true for a Hidden entry")
	}

	if err := Set(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Errorf("entry still exists after Set(false): %v", err)
	}
	if err := Set(false); err != nil {
		t.Errorf("Set(false) twice: %v", err)
	}
}

func TestQuoteExec(t *testing.T) {
	for in, want := range map[string]string{
		"/usr/local/bin/pogo":   "/usr/local/bin/pogo",
		"/home/me/My Apps/pogo": `"/home/me/My Apps/pogo"`,
		`/opt/a$b"c/pogo`:       `"/opt/a\$b\"c/pogo"`,
		"/opt/100%/pogo":        "/opt/100%%/pogo",
	} {
		if got := quoteExec(in); got != want {
			t.Errorf("quoteExec(%q) = %s, want %s", in, got, want)
		}
	}
}

package main

import (
	"log"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/dvher/pogo/internal/niri"
)

// chooseGDKBackend decides whether GTK should use Xwayland. It must run before
// application.New, which initialises GTK. It returns a function that restores
// the environment afterwards, so apps we launch (e.g. the browser for links)
// don't inherit the override.
//
// Notes need to stay on top and remember their position. Most Wayland
// compositors (GNOME, KDE) don't let apps do either, but their Xwayland
// server does. niri is handled natively over IPC instead: its
// xwayland-satellite can't position windows. Set POGO_NATIVE_WAYLAND=1 to
// opt out, e.g. for sharper text with fractional scaling.
func chooseGDKBackend() (restore func()) {
	noop := func() {}
	switch {
	case os.Getenv("GDK_BACKEND") != "",
		os.Getenv("POGO_NATIVE_WAYLAND") == "1",
		os.Getenv("WAYLAND_DISPLAY") == "", // X11 session
		os.Getenv("DISPLAY") == "",         // no Xwayland available
		niri.Available():
		return noop
	}
	slog.Info("using Xwayland so notes can stay on top (set POGO_NATIVE_WAYLAND=1 to disable)")
	os.Setenv("GDK_BACKEND", "x11")
	return func() { os.Unsetenv("GDK_BACKEND") }
}

// checkWebKitSandbox stops with a readable message when WebKit's sandbox
// can't start, instead of the SIGTRAP and goroutine dump WebKit produces.
// Ubuntu 23.10+ only lets unprivileged programs create user namespaces if an
// AppArmor profile allows it; the .deb installs one, but a binary run from
// elsewhere (e.g. bin/pogo) isn't covered by it. We ask bwrap directly, as
// WebKit does, so the check sees exactly what WebKit will.
func checkWebKitSandbox() {
	if os.Getenv("WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS") != "" {
		return
	}
	restricted, err := os.ReadFile("/proc/sys/kernel/apparmor_restrict_unprivileged_userns")
	if err != nil || strings.TrimSpace(string(restricted)) != "1" {
		return
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		return
	}
	out, err := exec.Command(bwrap, "--unshare-user", "--ro-bind", "/", "/", "true").CombinedOutput()
	if err == nil {
		return
	}
	exe, _ := os.Executable()
	log.Fatalf(`WebKit's sandbox can't start: %s
AppArmor blocks user namespaces for %s. Either:
  - install Pogo from the .deb, which adds an AppArmor profile for /usr/local/bin/pogo, or
  - run with WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1 (turns off WebKit's sandbox)`,
		strings.TrimSpace(string(out)), exe)
}

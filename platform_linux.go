package main

import (
	"log/slog"
	"os"

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

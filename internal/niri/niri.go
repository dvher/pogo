// Package niri is a minimal client for the niri compositor's IPC socket.
//
// On niri the app cannot position its own windows or keep them above others
// through GTK, so note windows are moved to niri's floating layer (which sits
// above tiled windows) and placed with MoveFloatingWindow.
package niri

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

// Available reports whether we are running under niri.
func Available() bool { return os.Getenv("NIRI_SOCKET") != "" }

type Layout struct {
	TileSize               [2]float64  `json:"tile_size"`
	WindowSize             [2]int      `json:"window_size"`
	TilePosInWorkspaceView *[2]float64 `json:"tile_pos_in_workspace_view"`
}

type Window struct {
	ID          uint64  `json:"id"`
	Title       *string `json:"title"`
	AppID       *string `json:"app_id"`
	PID         *int    `json:"pid"`
	WorkspaceID *uint64 `json:"workspace_id"`
	IsFloating  bool    `json:"is_floating"`
	IsFocused   bool    `json:"is_focused"`
	Layout      Layout  `json:"layout"`
}

// request sends one request on a fresh connection and decodes the Ok payload.
func request(req any, out any) error {
	conn, err := net.DialTimeout("unix", os.Getenv("NIRI_SOCKET"), 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(3 * time.Second))
	b, _ := json.Marshal(req)
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return err
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return err
	}
	var reply struct {
		Ok  json.RawMessage `json:"Ok"`
		Err *string         `json:"Err"`
	}
	if err := json.Unmarshal(line, &reply); err != nil {
		return err
	}
	if reply.Err != nil {
		return errors.New("niri: " + *reply.Err)
	}
	if out != nil {
		return json.Unmarshal(reply.Ok, out)
	}
	return nil
}

func action(a map[string]any) error {
	var ok json.RawMessage
	return request(map[string]any{"Action": a}, &ok)
}

// Windows lists all windows.
func Windows() ([]Window, error) {
	var out struct {
		Windows []Window `json:"Windows"`
	}
	err := request("Windows", &out)
	return out.Windows, err
}

// FindByTitle returns the window with the given title.
func FindByTitle(title string) (Window, bool) {
	ws, err := Windows()
	if err != nil {
		return Window{}, false
	}
	for _, w := range ws {
		if w.Title != nil && *w.Title == title {
			return w, true
		}
	}
	return Window{}, false
}

// MakeFloating moves a window to the floating layout.
func MakeFloating(id uint64) error {
	return action(map[string]any{"MoveWindowToFloating": map[string]any{"id": id}})
}

// MoveFloating places a floating window at x, y (logical pixels, relative to
// the working area of its output).
func MoveFloating(id uint64, x, y int) error {
	return action(map[string]any{"MoveFloatingWindow": map[string]any{
		"id": id,
		"x":  map[string]any{"SetFixed": float64(x)},
		"y":  map[string]any{"SetFixed": float64(y)},
	}})
}

// SetSize sets a window's size in logical pixels.
func SetSize(id uint64, w, h int) error {
	if err := action(map[string]any{"SetWindowWidth": map[string]any{
		"id": id, "change": map[string]any{"SetFixed": w},
	}}); err != nil {
		return err
	}
	return action(map[string]any{"SetWindowHeight": map[string]any{
		"id": id, "change": map[string]any{"SetFixed": h},
	}})
}

// FocusWindow focuses a window.
func FocusWindow(id uint64) error {
	return action(map[string]any{"FocusWindow": map[string]any{"id": id}})
}

// Position returns a floating window's position, if known.
func (w Window) Position() (x, y int, ok bool) {
	p := w.Layout.TilePosInWorkspaceView
	if p == nil {
		return 0, 0, false
	}
	return int(p[0]), int(p[1]), true
}

// LayoutChange is one entry of a WindowLayoutsChanged event.
type LayoutChange struct {
	ID     uint64
	Layout Layout
}

// WatchLayouts streams window layout changes (e.g. when the user drags a
// floating window) until stop is closed. It reconnects on errors.
func WatchLayouts(stop <-chan struct{}, fn func([]LayoutChange)) {
	for {
		err := watchOnce(stop, fn)
		select {
		case <-stop:
			return
		case <-time.After(2 * time.Second):
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "niri event stream:", err)
		}
	}
}

func watchOnce(stop <-chan struct{}, fn func([]LayoutChange)) error {
	conn, err := net.Dial("unix", os.Getenv("NIRI_SOCKET"))
	if err != nil {
		return err
	}
	go func() { <-stop; conn.Close() }()
	defer conn.Close()
	if _, err := conn.Write([]byte("\"EventStream\"\n")); err != nil {
		return err
	}
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var ev struct {
			WindowLayoutsChanged *struct {
				Changes [][2]json.RawMessage `json:"changes"`
			} `json:"WindowLayoutsChanged"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.WindowLayoutsChanged == nil {
			continue
		}
		var changes []LayoutChange
		for _, c := range ev.WindowLayoutsChanged.Changes {
			var lc LayoutChange
			if json.Unmarshal(c[0], &lc.ID) == nil && json.Unmarshal(c[1], &lc.Layout) == nil {
				changes = append(changes, lc)
			}
		}
		if len(changes) > 0 {
			fn(changes)
		}
	}
	return sc.Err()
}

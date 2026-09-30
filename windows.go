package main

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/dvher/pogo/internal/gtktitle"
	"github.com/dvher/pogo/internal/niri"
	"github.com/dvher/pogo/pkg/palette"
	"github.com/dvher/pogo/pkg/store"
)

// WindowManager owns one frameless, always-on-top window per visible note.
type WindowManager struct {
	app   *application.App
	store *store.Store

	mu      sync.Mutex
	windows map[string]*application.WebviewWindow // note id -> window
	niriIDs map[uint64]string                     // niri window id -> note id
	placing map[string]bool                       // note id -> still being placed on niri

	// quitting is set while the app shuts down so closing windows does not
	// mark their notes as hidden.
	quitting atomic.Bool
	niriStop chan struct{}
}

func NewWindowManager(s *store.Store) *WindowManager {
	return &WindowManager{
		store:    s,
		windows:  map[string]*application.WebviewWindow{},
		niriIDs:  map[uint64]string{},
		placing:  map[string]bool{},
		niriStop: make(chan struct{}),
	}
}

func (wm *WindowManager) windowTitle(id string) string { return "Pogo note " + id }

// Start opens all visible notes and, on niri, starts tracking window moves.
func (wm *WindowManager) Start() {
	if niri.Available() {
		go niri.WatchLayouts(wm.niriStop, wm.onNiriLayouts)
	}
	notes, err := wm.store.List()
	if err != nil {
		slog.Error("list notes", "error", err)
		return
	}
	for _, n := range notes {
		if !n.Hidden {
			wm.Open(n)
		}
	}
}

func (wm *WindowManager) Stop() {
	wm.quitting.Store(true)
	select {
	case <-wm.niriStop:
	default:
		close(wm.niriStop)
	}
}

// Open shows the window for n, creating it if needed.
func (wm *WindowManager) Open(n store.Note) {
	wm.mu.Lock()
	if w, ok := wm.windows[n.ID]; ok {
		wm.mu.Unlock()
		w.Show()
		w.Focus()
		return
	}
	rgb := palette.Get(n.Color).BG
	opts := application.WebviewWindowOptions{
		Name:             "note-" + n.ID,
		Title:            wm.windowTitle(n.ID),
		URL:              "/?note=" + n.ID,
		Width:            n.W,
		Height:           n.H,
		MinWidth:         140,
		MinHeight:        90,
		Frameless:        true,
		AlwaysOnTop:      true,
		DisableResize:    n.Anchored,
		BackgroundColour: application.NewRGB(rgb[0], rgb[1], rgb[2]),
		Windows:          application.WindowsWindow{HiddenOnTaskbar: true},
		Linux:            application.LinuxWindow{WindowDidMoveDebounceMS: 300},
	}
	if n.Placed && !niri.Available() {
		opts.InitialPosition = application.WindowXY
		opts.X, opts.Y = n.X, n.Y
	}
	w := wm.app.Window.NewWithOptions(opts)
	wm.windows[n.ID] = w
	if niri.Available() {
		wm.placing[n.ID] = true
	}
	wm.mu.Unlock()

	id := n.ID
	w.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		wm.mu.Lock()
		_, tracked := wm.windows[id]
		delete(wm.windows, id)
		for nid, noteID := range wm.niriIDs {
			if noteID == id {
				delete(wm.niriIDs, nid)
			}
		}
		wm.mu.Unlock()
		// Closed by the user or compositor (not via Close/quit): treat as hide.
		if tracked && !wm.quitting.Load() {
			wm.store.SetHidden(id, true)
			wm.app.Event.Emit(EventListChanged)
		}
	})
	w.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) {
		if wm.isPlacing(id) {
			return
		}
		width, height := w.Size()
		if width > 0 && height > 0 {
			wm.store.SetSize(id, width, height)
		}
	})
	if niri.Available() {
		go wm.placeOnNiri(w, id, n)
	} else {
		w.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) {
			x, y := w.Position()
			wm.store.SetPosition(id, x, y)
		})
	}
}

// placeOnNiri waits for the window to be mapped, moves it to the floating
// layer (above tiled windows) and restores its saved position.
func (wm *WindowManager) placeOnNiri(w *application.WebviewWindow, id string, n store.Note) {
	defer func() {
		wm.mu.Lock()
		delete(wm.placing, id)
		wm.mu.Unlock()
	}()
	title := wm.windowTitle(id)
	titled := false
	for range 50 {
		if !titled {
			if ptr := w.NativeWindow(); ptr != nil {
				application.InvokeSync(func() { gtktitle.Set(ptr, title) })
				titled = true
			}
		}
		if nw, ok := niri.FindByTitle(title); ok {
			wm.mu.Lock()
			wm.niriIDs[nw.ID] = id
			wm.mu.Unlock()
			if !nw.IsFloating {
				if err := niri.MakeFloating(nw.ID); err != nil {
					slog.Warn("niri float", "error", err)
					return
				}
			}
			// niri applies its own floating size/position shortly after the
			// window becomes floating, so apply ours until it sticks. Saved
			// positions are in workspace-view coordinates while moves are
			// relative to the working area (e.g. below a bar), so correct the
			// target by the offset we observe.
			cmdX, cmdY := n.X, n.Y
			for range 10 {
				time.Sleep(80 * time.Millisecond)
				niri.SetSize(nw.ID, n.W, n.H)
				if n.Placed {
					niri.MoveFloating(nw.ID, cmdX, cmdY)
				}
				time.Sleep(80 * time.Millisecond)
				cur, ok := niri.FindByTitle(title)
				if !ok {
					return
				}
				x, y, _ := cur.Position()
				sized := abs(cur.Layout.WindowSize[0]-n.W) <= 2 && abs(cur.Layout.WindowSize[1]-n.H) <= 2
				placed := !n.Placed || (abs(x-n.X) <= 2 && abs(y-n.Y) <= 2)
				if sized && placed {
					break
				}
				if n.Placed {
					cmdX += n.X - x
					cmdY += n.Y - y
				}
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	slog.Warn("niri: note window not found", "note", id)
}

func (wm *WindowManager) onNiriLayouts(changes []niri.LayoutChange) {
	for _, c := range changes {
		wm.mu.Lock()
		id, ok := wm.niriIDs[c.ID]
		placing := wm.placing[id]
		wm.mu.Unlock()
		if !ok || placing {
			continue
		}
		if x, y, ok := (niri.Window{Layout: c.Layout}).Position(); ok {
			wm.store.SetPosition(id, x, y)
		}
	}
}

func (wm *WindowManager) isPlacing(id string) bool {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	return wm.placing[id]
}

// Close closes a note's window without marking it hidden.
func (wm *WindowManager) Close(id string) {
	wm.mu.Lock()
	w, ok := wm.windows[id]
	delete(wm.windows, id)
	wm.mu.Unlock()
	if ok {
		w.Close()
	}
}

// IsOpen reports whether a note currently has a window.
func (wm *WindowManager) IsOpen(id string) bool {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	_, ok := wm.windows[id]
	return ok
}

// SetAnchored locks or unlocks resizing of a note's window. Dragging is
// controlled by the frontend.
func (wm *WindowManager) SetAnchored(id string, anchored bool) {
	wm.mu.Lock()
	w, ok := wm.windows[id]
	wm.mu.Unlock()
	if ok {
		w.SetResizable(!anchored)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

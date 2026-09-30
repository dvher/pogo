package niri

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeNiri listens on a socket in NIRI_SOCKET and answers each request with
// reply(request). It records the requests it got.
type fakeNiri struct {
	mu   sync.Mutex
	reqs []string
}

func (f *fakeNiri) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.reqs...)
}

func startFake(t *testing.T, reply func(req string) string) *fakeNiri {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "niri.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	t.Setenv("NIRI_SOCKET", sock)
	f := &fakeNiri{}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				line, err := bufio.NewReader(conn).ReadString('\n')
				if err != nil {
					return
				}
				f.mu.Lock()
				f.reqs = append(f.reqs, line[:len(line)-1])
				f.mu.Unlock()
				// niri replies with one line of JSON.
				out := []byte(reply(line[:len(line)-1]))
				var compact bytes.Buffer
				if json.Compact(&compact, out) == nil {
					out = compact.Bytes()
				}
				conn.Write(append(out, '\n'))
			}()
		}
	}()
	return f
}

func TestAvailable(t *testing.T) {
	t.Setenv("NIRI_SOCKET", "")
	if Available() {
		t.Fatal("available without NIRI_SOCKET")
	}
	t.Setenv("NIRI_SOCKET", "/run/user/1000/niri.sock")
	if !Available() {
		t.Fatal("not available with NIRI_SOCKET")
	}
}

func TestWindows(t *testing.T) {
	startFake(t, func(string) string {
		return `{"Ok":{"Windows":[
			{"id":1,"title":"Firefox","app_id":"firefox","is_floating":false,"layout":{"tile_size":[800,600],"window_size":[800,600],"tile_pos_in_workspace_view":null}},
			{"id":7,"title":"pogo-note-abc","app_id":"pogo","is_floating":true,"layout":{"tile_size":[260,260],"window_size":[260,260],"tile_pos_in_workspace_view":[120.6,48.2]}},
			{"id":9,"title":null,"layout":{}}
		]}}`
	})
	ws, err := Windows()
	if err != nil || len(ws) != 3 {
		t.Fatalf("windows: %v %v", ws, err)
	}
	w, ok := FindByTitle("pogo-note-abc")
	if !ok || w.ID != 7 || !w.IsFloating {
		t.Fatalf("find: %+v %v", w, ok)
	}
	if x, y, ok := w.Position(); !ok || x != 120 || y != 48 {
		t.Fatalf("position: %d,%d %v", x, y, ok)
	}
	if _, _, ok := ws[0].Position(); ok {
		t.Fatal("tiled window has a floating position")
	}
	if _, ok := FindByTitle("missing"); ok {
		t.Fatal("found a missing window")
	}
}

func TestActions(t *testing.T) {
	f := startFake(t, func(string) string { return `{"Ok":"Handled"}` })
	if err := MakeFloating(7); err != nil {
		t.Fatal(err)
	}
	if err := MoveFloating(7, 100, -20); err != nil {
		t.Fatal(err)
	}
	if err := SetSize(7, 300, 200); err != nil {
		t.Fatal(err)
	}
	if err := FocusWindow(7); err != nil {
		t.Fatal(err)
	}
	want := []string{
		`{"Action":{"MoveWindowToFloating":{"id":7}}}`,
		`{"Action":{"MoveFloatingWindow":{"id":7,"x":{"SetFixed":100},"y":{"SetFixed":-20}}}}`,
		`{"Action":{"SetWindowWidth":{"change":{"SetFixed":300},"id":7}}}`,
		`{"Action":{"SetWindowHeight":{"change":{"SetFixed":200},"id":7}}}`,
		`{"Action":{"FocusWindow":{"id":7}}}`,
	}
	got := f.requests()
	if len(got) != len(want) {
		t.Fatalf("requests:\n%v", got)
	}
	for i := range want {
		if !sameJSON(got[i], want[i]) {
			t.Errorf("request %d:\n got %s\nwant %s", i, got[i], want[i])
		}
	}
}

func TestErrors(t *testing.T) {
	startFake(t, func(string) string { return `{"Err":"no window with id 7"}` })
	if err := FocusWindow(7); err == nil || err.Error() != "niri: no window with id 7" {
		t.Fatalf("niri error: %v", err)
	}
	if _, ok := FindByTitle("x"); ok {
		t.Fatal("found a window despite an error")
	}

	startFake(t, func(string) string { return `garbage` })
	if _, err := Windows(); err == nil {
		t.Fatal("garbage reply accepted")
	}

	t.Setenv("NIRI_SOCKET", filepath.Join(t.TempDir(), "absent.sock"))
	if err := FocusWindow(1); err == nil {
		t.Fatal("no error without a socket")
	}
}

func TestWatchLayouts(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "niri.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	t.Setenv("NIRI_SOCKET", sock)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		if line, _ := bufio.NewReader(conn).ReadString('\n'); line != "\"EventStream\"\n" {
			conn.Write([]byte(`{"Err":"unexpected request"}` + "\n"))
			return
		}
		for _, ev := range []string{
			`{"Ok":"Handled"}`,
			`{"WorkspacesChanged":{"workspaces":[]}}`,
			`not json`,
			`{"WindowLayoutsChanged":{"changes":[[7,{"tile_size":[260,260],"window_size":[260,260],"tile_pos_in_workspace_view":[10,20]}],["bad",{}]]}}`,
		} {
			conn.Write([]byte(ev + "\n"))
		}
		time.Sleep(time.Minute) // hold the stream open until stop
	}()

	got := make(chan []LayoutChange, 1)
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		WatchLayouts(stop, func(c []LayoutChange) { got <- c })
		close(done)
	}()
	select {
	case changes := <-got:
		if len(changes) != 1 || changes[0].ID != 7 {
			t.Fatalf("changes: %+v", changes)
		}
		if x, y, ok := (Window{Layout: changes[0].Layout}).Position(); !ok || x != 10 || y != 20 {
			t.Fatalf("position %d,%d", x, y)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no layout change")
	}
	close(stop)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WatchLayouts did not stop")
	}
}

func sameJSON(a, b string) bool {
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return string(ja) == string(jb)
}

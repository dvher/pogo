package pogosync

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dvher/pogo/pkg/palette"
	"github.com/dvher/pogo/pkg/secure"
	"github.com/dvher/pogo/pkg/store"
	"github.com/dvher/pogo/pkg/syncclient"
)

const testToken = "pogo_test"

// pad is an in-memory Pogo Pad: last-write-wins by updated_at then
// device_id, revs per change, and pages of pageSize (see server/API.md).
type pad struct {
	mu       sync.Mutex
	notes    map[string]syncclient.Note
	rev      int64
	e2e      *syncclient.E2EParams
	pageSize int
	batches  []int // number of changes in each sync request
	url      *httptest.Server
}

func newPad(t *testing.T) *pad {
	p := &pad{notes: map[string]syncclient.Note{}, pageSize: 500}
	p.url = httptest.NewServer(http.HandlerFunc(p.serve))
	t.Cleanup(p.url.Close)
	return p
}

func (p *pad) serve(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.URL.Path == "/api/v1/health" {
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "version": "fake"})
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+testToken {
		w.WriteHeader(401)
		return
	}
	switch r.Method + " " + r.URL.Path {
	case "POST /api/v1/sync":
		var req syncclient.SyncRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Changes == nil {
			w.WriteHeader(400)
			return
		}
		p.batches = append(p.batches, len(req.Changes))
		for _, in := range req.Changes {
			cur, ok := p.notes[in.ID]
			if ok && (in.UpdatedAt < cur.UpdatedAt || in.UpdatedAt == cur.UpdatedAt && in.DeviceID <= cur.DeviceID) {
				continue
			}
			p.rev++
			in.Rev = p.rev
			p.notes[in.ID] = in
		}
		var all []syncclient.Note
		for _, n := range p.notes {
			if n.Rev > req.Cursor {
				all = append(all, n)
			}
		}
		sort.Slice(all, func(i, j int) bool { return all[i].Rev < all[j].Rev })
		resp := syncclient.SyncResponse{Cursor: req.Cursor, Changes: []syncclient.Note{}}
		if len(all) > p.pageSize {
			all, resp.More = all[:p.pageSize], true
		}
		for _, n := range all {
			resp.Changes = append(resp.Changes, n)
			resp.Cursor = n.Rev
		}
		json.NewEncoder(w).Encode(resp)
	case "GET /api/v1/e2e":
		if p.e2e == nil {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(p.e2e)
	case "PUT /api/v1/e2e":
		if p.e2e != nil && r.URL.Query().Get("force") != "1" {
			w.WriteHeader(409)
			return
		}
		p.e2e = &syncclient.E2EParams{}
		json.NewDecoder(r.Body).Decode(p.e2e)
		w.WriteHeader(200)
	case "DELETE /api/v1/e2e":
		p.e2e = nil
		w.WriteHeader(204)
	default:
		w.WriteHeader(404)
	}
}

func (p *pad) note(id string) syncclient.Note {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.notes[id]
}

func (p *pad) put(n syncclient.Note) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rev++
	n.Rev = p.rev
	p.notes[n.ID] = n
}

func (p *pad) settings() Settings {
	host, port, _ := net.SplitHostPort(strings.TrimPrefix(p.url.URL, "http://"))
	return Settings{Scheme: "http", Host: host, Port: port, Token: testToken, Enabled: true, IntervalSec: 30}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	c, _ := secure.NewCipher(secure.NewKey())
	st, err := store.Open(t.TempDir(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// newEngine returns an engine for a fresh store, connected to p if not nil.
func newEngine(t *testing.T, p *pad, device string) *Engine {
	t.Helper()
	e := New(openStore(t), device)
	if p != nil {
		if _, err := e.SaveSettings(p.settings()); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func add(t *testing.T, e *Engine, id, content string) {
	t.Helper()
	now := time.Now().UnixMilli()
	if err := e.store.Insert(store.Note{ID: id, Content: content, Color: "blue", CreatedAt: now, UpdatedAt: now, DeviceID: e.DeviceID(), W: 260, H: 260}); err != nil {
		t.Fatal(err)
	}
}

func content(t *testing.T, e *Engine, id string) string {
	t.Helper()
	n, err := e.store.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return n.Content
}

func mustSync(t *testing.T, e *Engine) {
	t.Helper()
	if err := e.SyncNow(); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func TestDeviceID(t *testing.T) {
	st := openStore(t)
	id := DeviceID(st)
	if len(id) != 36 || DeviceID(st) != id {
		t.Fatalf("device id %q is not a stable UUID", id)
	}
	if other := DeviceID(openStore(t)); other == id {
		t.Fatal("two installs share a device id")
	}
}

func TestNormalize(t *testing.T) {
	got, err := normalize(Settings{Scheme: "ftp", Host: " pad.lan ", Port: " 8080 ", Token: " pogo_x\n", IntervalSec: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got.Scheme != "http" || got.Host != "pad.lan" || got.Port != "8080" || got.Token != "pogo_x" || got.IntervalSec != MinInterval {
		t.Fatalf("normalize: %+v", got)
	}
	if got, _ := normalize(Settings{Scheme: "https"}); got.Scheme != "https" {
		t.Fatalf("https became %q", got.Scheme)
	}
	// Incomplete settings are fine while sync is off.
	if _, err := normalize(Settings{}); err != nil {
		t.Fatalf("disabled, empty: %v", err)
	}
	for name, s := range map[string]Settings{
		"port zero":     {Port: "0"},
		"port too high": {Port: "65536"},
		"port not int":  {Port: "http"},
		"no host":       {Enabled: true, Token: "t"},
		"no token":      {Enabled: true, Host: "pad.lan"},
	} {
		if _, err := normalize(s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	e := newEngine(t, nil, "d")
	if s := e.Settings(); s.Enabled || s.Host != "" || s.Port != "8080" || s.IntervalSec != DefaultInterval {
		t.Fatalf("defaults: %+v", s)
	}
	in := Settings{Scheme: "https", Host: "pad.example.com", Port: "443", Token: "pogo_x", Enabled: true, IntervalSec: 60}
	if _, err := e.SaveSettings(in); err != nil {
		t.Fatal(err)
	}
	if got := e.Settings(); got != in {
		t.Fatalf("saved %+v, read %+v", in, got)
	}
	if e.interval() != time.Minute {
		t.Fatalf("interval %v", e.interval())
	}
	// The token is a secret setting, not stored in plaintext.
	if raw := e.store.Setting(keyToken, ""); strings.Contains(raw, "pogo_x") {
		t.Fatal("token stored as a plain setting")
	}
	if _, err := e.SaveSettings(Settings{Enabled: true, Token: "t"}); err == nil {
		t.Fatal("saved settings without a host")
	}
}

func TestDisabled(t *testing.T) {
	e := newEngine(t, nil, "d")
	if err := e.SyncNow(); !errors.Is(err, ErrDisabled) {
		t.Fatalf("sync with no server: %v", err)
	}
	p := newPad(t)
	s := p.settings()
	s.Enabled = false
	e.SaveSettings(s)
	if err := e.SyncNow(); !errors.Is(err, ErrDisabled) {
		t.Fatalf("sync while disabled: %v", err)
	}
	if st := e.Status(); st.Enabled || st.LastSync != 0 {
		t.Fatalf("status: %+v", st)
	}
}

func TestSyncTwoDevices(t *testing.T) {
	p := newPad(t)
	a, b := newEngine(t, p, "device-a"), newEngine(t, p, "device-b")
	var applied []Applied
	b.OnApplied = func(got []Applied) { applied = got }
	var statuses []Status
	a.OnStatus = func(s Status) { statuses = append(statuses, s) }

	add(t, a, "n1", "- [ ] milk")
	add(t, a, "n2", "eggs")
	mustSync(t, a)
	if last := len(statuses) - 1; last < 1 || !statuses[0].Syncing || statuses[last].Syncing || statuses[last].LastSync == 0 {
		t.Fatalf("status updates: %+v", statuses)
	}
	if dirty, _ := a.store.Dirty(); len(dirty) != 0 {
		t.Fatalf("still dirty after sync: %v", dirty)
	}
	if got := p.note("n1"); got.Content != "- [ ] milk" || got.Color != "blue" || got.DeviceID != "device-a" {
		t.Fatalf("server has %+v", got)
	}

	mustSync(t, b)
	if content(t, b, "n1") != "- [ ] milk" || content(t, b, "n2") != "eggs" {
		t.Fatal("B did not receive A's notes")
	}
	if len(applied) != 2 || applied[0] != (Applied{"n1", true}) || applied[1] != (Applied{"n2", true}) {
		t.Fatalf("applied: %+v", applied)
	}

	// An edit on B reaches A as an update, not a new note.
	text := "- [x] milk"
	if _, err := b.store.Edit("n1", b.DeviceID(), &text, nil, false); err != nil {
		t.Fatal(err)
	}
	mustSync(t, b)
	var aApplied []Applied
	a.OnApplied = func(got []Applied) { aApplied = got }
	mustSync(t, a)
	if content(t, a, "n1") != text || len(aApplied) != 1 || aApplied[0].Created {
		t.Fatalf("A: %q, applied %+v", content(t, a, "n1"), aApplied)
	}

	// Nothing new: OnApplied is not called.
	aApplied = nil
	mustSync(t, a)
	if aApplied != nil {
		t.Fatalf("applied without changes: %+v", aApplied)
	}

	// A remote delete of a note this device never had is ignored.
	p.put(syncclient.Note{ID: "gone", Deleted: true, UpdatedAt: 1, DeviceID: "c"})
	mustSync(t, a)
	if _, err := a.store.Get("gone"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted note created locally: %v", err)
	}
}

func TestLocalEditWinsOverOlderRemote(t *testing.T) {
	p := newPad(t)
	a := newEngine(t, p, "device-a")
	add(t, a, "n1", "local")
	n, _ := a.store.Get("n1")
	p.put(syncclient.Note{ID: "n1", Content: "stale", Color: "pink", UpdatedAt: n.UpdatedAt - 1000, DeviceID: "device-z"})
	mustSync(t, a)
	if content(t, a, "n1") != "local" || p.note("n1").Content != "local" {
		t.Fatalf("local %q, server %q", content(t, a, "n1"), p.note("n1").Content)
	}
}

func TestBatchingAndPaging(t *testing.T) {
	defer func(old int) { uploadBatch = old }(uploadBatch)
	uploadBatch = 20
	p := newPad(t)
	p.pageSize = 8
	a := newEngine(t, p, "device-a")
	const n = 30
	for i := range n {
		add(t, a, fmt.Sprintf("n%04d", i), fmt.Sprint(i))
	}
	mustSync(t, a)
	if len(p.batches) < 2 || p.batches[0] != uploadBatch || p.batches[1] != n-uploadBatch {
		t.Fatalf("upload batches: %v", p.batches)
	}
	if len(p.notes) != n {
		t.Fatalf("server has %d notes", len(p.notes))
	}

	// A new device pages through everything.
	p.batches = nil
	b := newEngine(t, p, "device-b")
	var applied []Applied
	b.OnApplied = func(got []Applied) { applied = got }
	mustSync(t, b)
	if len(applied) != n || len(p.batches) != (n+p.pageSize-1)/p.pageSize {
		t.Fatalf("applied %d notes in %d requests", len(applied), len(p.batches))
	}
	if notes, _ := b.store.List(); len(notes) != n {
		t.Fatalf("B has %d notes", len(notes))
	}
	if cursor := b.store.IntSetting(keyCursor, 0); cursor != p.rev {
		t.Fatalf("cursor %d, server rev %d", cursor, p.rev)
	}
}

func TestChangingServerReuploads(t *testing.T) {
	p1, p2 := newPad(t), newPad(t)
	a := newEngine(t, p1, "device-a")
	add(t, a, "n1", "hello")
	mustSync(t, a)
	if a.store.IntSetting(keyCursor, 0) == 0 {
		t.Fatal("cursor not saved")
	}
	// Same server, other settings: nothing is re-queued.
	s := p1.settings()
	s.IntervalSec = 120
	a.SaveSettings(s)
	if dirty, _ := a.store.Dirty(); len(dirty) != 0 {
		t.Fatalf("interval change queued %d notes", len(dirty))
	}
	a.SaveSettings(p2.settings())
	if a.store.IntSetting(keyCursor, -1) != 0 {
		t.Fatal("cursor not reset")
	}
	mustSync(t, a)
	if p2.note("n1").Content != "hello" {
		t.Fatal("notes not uploaded to the new server")
	}
}

func TestErrorsAreReported(t *testing.T) {
	p := newPad(t)
	a := newEngine(t, p, "device-a")
	s := p.settings()
	s.Token = "pogo_wrong"
	a.SaveSettings(s)
	add(t, a, "n1", "x")
	if err := a.SyncNow(); !errors.Is(err, syncclient.ErrUnauthorized) {
		t.Fatalf("bad token: %v", err)
	}
	if st := a.Status(); st.LastError == "" || st.Syncing {
		t.Fatalf("status after failure: %+v", st)
	}
	if dirty, _ := a.store.Dirty(); len(dirty) != 1 {
		t.Fatal("failed upload was marked clean")
	}
	a.SaveSettings(p.settings())
	if a.Status().LastError != "" {
		t.Fatal("saving settings did not clear the error")
	}
	mustSync(t, a)
}

func TestTestConnection(t *testing.T) {
	p := newPad(t)
	e := newEngine(t, nil, "d")
	s := p.settings()
	if msg, err := e.TestConnection(s); err != nil || !strings.Contains(msg, "encryption is off") {
		t.Fatalf("plain: %q %v", msg, err)
	}
	s.Token, s.Enabled = "", false
	if msg, err := e.TestConnection(s); err != nil || !strings.Contains(msg, "Add a token") {
		t.Fatalf("no token: %q %v", msg, err)
	}
	s.Token, s.Enabled = "pogo_wrong", true
	if _, err := e.TestConnection(s); !errors.Is(err, syncclient.ErrUnauthorized) {
		t.Fatalf("bad token: %v", err)
	}
	p.e2e = &syncclient.E2EParams{Salt: "x", Check: "y"}
	if msg, err := e.TestConnection(p.settings()); err != nil || !strings.Contains(msg, "end-to-end encrypted") {
		t.Fatalf("e2e: %q %v", msg, err)
	}
	if _, err := e.TestConnection(Settings{Host: "pad.lan", Port: "99999"}); err == nil {
		t.Fatal("bad port accepted")
	}
	// TestConnection does not save anything.
	if e.Settings().Host != "" {
		t.Fatal("TestConnection saved settings")
	}
}

func TestEndToEndEncryption(t *testing.T) {
	p := newPad(t)
	a, b := newEngine(t, p, "device-a"), newEngine(t, p, "device-b")
	add(t, a, "n1", "secret plan")
	mustSync(t, a)
	mustSync(t, b)

	if err := a.EnableE2E("short"); err == nil {
		t.Fatal("short passphrase accepted")
	}
	if err := a.EnableE2E("correct horse"); err != nil {
		t.Fatal(err)
	}
	if !a.Status().E2EEnabled || p.e2e == nil {
		t.Fatal("E2E not enabled")
	}
	srv := p.note("n1")
	if !strings.HasPrefix(srv.Content, secure.E2EPrefix) || strings.Contains(srv.Content, "secret") || srv.Color != "" {
		t.Fatalf("server note is not sealed: %+v", srv)
	}

	// B cannot read or upload anything until it has the passphrase.
	text := "B's edit"
	b.store.Edit("n1", b.DeviceID(), &text, nil, false)
	if err := b.SyncNow(); !errors.Is(err, ErrLocked) {
		t.Fatalf("B sync: %v", err)
	}
	if st := b.Status(); !st.E2EServer || !st.E2ELocked || st.E2EEnabled {
		t.Fatalf("B status: %+v", st)
	}
	if strings.Contains(p.note("n1").Content, "B's edit") {
		t.Fatal("locked device uploaded plaintext")
	}
	if err := b.EnableE2E("wrong horse"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	if err := b.EnableE2E("correct horse"); err != nil {
		t.Fatal(err)
	}
	mustSync(t, a)
	if content(t, a, "n1") != "B's edit" {
		t.Fatalf("A got %q", content(t, a, "n1"))
	}
	if n, _ := a.store.Get("n1"); n.Color != "blue" {
		t.Fatalf("color lost through E2E: %q", n.Color)
	}

	// Deleting a note does not upload its old content.
	a.store.Edit("n1", a.DeviceID(), nil, nil, true)
	mustSync(t, a)
	if srv := p.note("n1"); !srv.Deleted || srv.Content != "" {
		t.Fatalf("deleted note on server: %+v", srv)
	}

	// Turning E2E off from A: plaintext again, and B forgets its key.
	add(t, a, "n2", "public")
	if err := b.DisableE2E(); err != nil {
		t.Fatal(err)
	}
	mustSync(t, a)
	if p.e2e != nil || strings.HasPrefix(p.note("n2").Content, secure.E2EPrefix) {
		t.Fatalf("E2E still on: %+v %+v", p.e2e, p.note("n2"))
	}
	if a.Status().E2EEnabled {
		t.Fatal("A kept its E2E key")
	}
	if err := a.DisableE2E(); err == nil {
		t.Fatal("DisableE2E without a key succeeded")
	}
}

func TestEnableE2EJoinsExisting(t *testing.T) {
	// Another device already set up E2E: the passphrase must match it.
	p := newPad(t)
	a := newEngine(t, p, "device-a")
	salt := secure.NewSalt()
	c, _ := secure.NewCipher(secure.DeriveE2EKey("correct horse", salt))
	other := syncclient.E2EParams{KDF: secure.E2EKDF, Salt: base64.RawStdEncoding.EncodeToString(salt), Check: c.MakeCheck()}
	p.e2e = &other
	if err := a.EnableE2E("correct horse"); err != nil {
		t.Fatal(err)
	}
	if *p.e2e != other {
		t.Fatal("existing E2E setup was replaced")
	}
	p.e2e = &syncclient.E2EParams{Salt: "!!not base64"}
	a.store.SetSecretSetting(keyE2EKey, "")
	if err := a.EnableE2E("correct horse"); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed salt: %v", err)
	}
}

func TestDecodeNote(t *testing.T) {
	c, _ := secure.NewCipher(secure.NewKey())
	for _, color := range []string{"purple", "chartreuse", ""} {
		for _, e2e := range []*secure.Cipher{nil, c} {
			in := store.Note{ID: "n1", Content: "hi", Color: color, UpdatedAt: 5, DeviceID: "d"}
			r, err := decodeNote(encodeNote(in, e2e), e2e)
			want := color
			if !palette.Valid(color) {
				want = palette.Default
			}
			if err != nil || r.Content != "hi" || r.Color != want || r.UpdatedAt != 5 || r.DeviceID != "d" {
				t.Errorf("color %q, e2e %v: %+v %v", color, e2e != nil, r, err)
			}
		}
	}
	sealed := encodeNote(store.Note{ID: "n1", Content: "hi"}, c)
	if _, err := decodeNote(sealed, nil); !errors.Is(err, ErrLocked) {
		t.Errorf("sealed note without a key: %v", err)
	}
	other, _ := secure.NewCipher(secure.NewKey())
	if _, err := decodeNote(sealed, other); err == nil {
		t.Error("sealed note opened with the wrong key")
	}
	garbage := syncclient.Note{Content: secure.E2EPrefix + c.Seal([]byte("not json"))}
	if _, err := decodeNote(garbage, c); err == nil {
		t.Error("undecodable payload accepted")
	}
}

func TestUndecodableNotesAreSkipped(t *testing.T) {
	p := newPad(t)
	a := newEngine(t, p, "device-a")
	key := make([]byte, 32)
	rand.Read(key)
	c, _ := secure.NewCipher(key)
	p.put(syncclient.Note{ID: "bad", Content: secure.E2EPrefix + c.Seal([]byte("{")), UpdatedAt: 1, DeviceID: "x"})
	p.put(syncclient.Note{ID: "good", Content: "fine", Color: "green", UpdatedAt: 1, DeviceID: "x"})
	mustSync(t, a)
	if content(t, a, "good") != "fine" {
		t.Fatal("good note not applied")
	}
	if _, err := a.store.Get("bad"); err == nil {
		t.Fatal("undecodable note applied")
	}
}

func TestLoop(t *testing.T) {
	p := newPad(t)
	a := newEngine(t, p, "device-a")
	add(t, a, "n1", "x")
	done := make(chan struct{})
	a.OnStatus = func(s Status) {
		if !s.Syncing && s.LastSync != 0 {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.Loop(ctx)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Loop did not sync")
	}
	cancel()
	if p.note("n1").Content != "x" {
		t.Fatal("note not uploaded by Loop")
	}
}

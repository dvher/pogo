package main

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite" // reads the server's database

	"github.com/dvher/pogo/pkg/pogosync"
	"github.com/dvher/pogo/pkg/secure"
	"github.com/dvher/pogo/pkg/store"
)

// startServer builds and runs ../server and returns its port, a token and
// the path of its database.
func startServer(t *testing.T) (port, token, dbPath string) {
	t.Helper()
	var serverDir string
	for _, dir := range []string{"../server", "../pogo_pad"} {
		if _, err := os.Stat(filepath.Join(dir, "cmd", "pogo-pad")); err == nil {
			serverDir, _ = filepath.Abs(dir)
			break
		}
	}
	if serverDir == "" {
		t.Skip("Pogo Pad repo not found at ../server or ../pogo_pad")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "pogo-pad")
	build := exec.Command("go", "build", "-o", bin, "./cmd/pogo-pad")
	build.Dir = serverDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build server: %v\n%s", err, out)
	}
	dbPath = filepath.Join(dir, "server.db")
	out, err := exec.Command(bin, "token", "create", "--db", dbPath, "--name", "test").Output()
	if err != nil {
		t.Fatal(err)
	}
	token = strings.TrimSpace(string(out))

	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port = fmt.Sprint(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	cmd := exec.Command(bin, "serve", "--addr", "127.0.0.1:"+port, "--db", dbPath)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	for range 50 {
		if resp, err := http.Get("http://127.0.0.1:" + port + "/api/v1/health"); err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("server did not start")
	return
}

type device struct {
	store    *store.Store
	sync     *SyncService
	settings *SettingsService
	notes    *NoteService
}

func newDevice(t *testing.T, name, port, token string) *device {
	t.Helper()
	key := make([]byte, 32)
	rand.Read(key)
	c, _ := secure.NewCipher(key)
	st, err := store.Open(t.TempDir(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	sy := NewSyncService(st, nil, name)
	d := &device{store: st, sync: sy, settings: &SettingsService{sync: sy}}
	if _, err := d.settings.Save(pogosync.Settings{Scheme: "http", Host: "127.0.0.1", Port: port, Token: token, Enabled: true, IntervalSec: 30}); err != nil {
		t.Fatal(err)
	}
	return d
}

func (d *device) add(t *testing.T, id, content string) {
	t.Helper()
	now := time.Now().UnixMilli()
	if err := d.store.Insert(store.Note{ID: id, Content: content, Color: "pink", CreatedAt: now, UpdatedAt: now, DeviceID: d.sync.engine.DeviceID(), W: 260, H: 260}); err != nil {
		t.Fatal(err)
	}
}

func (d *device) edit(t *testing.T, id, content string) {
	t.Helper()
	if _, err := d.store.Edit(id, d.sync.engine.DeviceID(), &content, nil, false); err != nil {
		t.Fatal(err)
	}
}

func (d *device) mustSync(t *testing.T) {
	t.Helper()
	if err := d.sync.SyncNow(); err != nil {
		t.Fatalf("sync: %v", err)
	}
}

func (d *device) content(t *testing.T, id string) string {
	t.Helper()
	n, err := d.store.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return n.Content
}

func serverContents(t *testing.T, dbPath string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT content FROM notes WHERE deleted = 0`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		out = append(out, c)
	}
	return out
}

func TestSyncEndToEnd(t *testing.T) {
	port, token, dbPath := startServer(t)
	a := newDevice(t, "device-a", port, token)
	b := newDevice(t, "device-b", port, token)

	// Create on A, appears on B.
	a.add(t, "n1", "- [ ] milk")
	a.mustSync(t)
	b.mustSync(t)
	if got := b.content(t, "n1"); got != "- [ ] milk" {
		t.Fatalf("B got %q", got)
	}
	if n, _ := b.store.Get("n1"); n.Color != "pink" {
		t.Fatalf("color not synced: %q", n.Color)
	}

	// Edit on B, appears on A.
	b.edit(t, "n1", "- [x] milk")
	b.mustSync(t)
	a.mustSync(t)
	if got := a.content(t, "n1"); got != "- [x] milk" {
		t.Fatalf("A got %q", got)
	}

	// Concurrent edits: the later one wins on both devices.
	a.edit(t, "n1", "from A")
	time.Sleep(5 * time.Millisecond)
	b.edit(t, "n1", "from B (later)")
	a.mustSync(t)
	b.mustSync(t)
	a.mustSync(t)
	if a.content(t, "n1") != "from B (later)" || b.content(t, "n1") != "from B (later)" {
		t.Fatalf("LWW: A=%q B=%q", a.content(t, "n1"), b.content(t, "n1"))
	}

	// Enable E2E on A: server content becomes ciphertext.
	if err := a.settings.EnableE2E("correct horse"); err != nil {
		t.Fatalf("enable e2e: %v", err)
	}
	for _, c := range serverContents(t, dbPath) {
		if !strings.HasPrefix(c, secure.E2EPrefix) {
			t.Fatalf("plaintext on server after enabling E2E: %q", c)
		}
	}

	// B is locked until it enters the passphrase; it must not upload plaintext.
	b.edit(t, "n1", "B while locked")
	if err := b.sync.SyncNow(); err == nil || !b.sync.Status().E2ELocked {
		t.Fatalf("B should be locked, err=%v status=%+v", err, b.sync.Status())
	}
	for _, c := range serverContents(t, dbPath) {
		if strings.Contains(c, "B while locked") {
			t.Fatal("locked device leaked plaintext")
		}
	}
	if err := b.settings.EnableE2E("wrong passphrase"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	if err := b.settings.EnableE2E("correct horse"); err != nil {
		t.Fatalf("unlock: %v", err)
	}

	// Encrypted notes still sync both ways.
	a.add(t, "n2", "secret plan")
	a.mustSync(t)
	b.mustSync(t)
	if got := b.content(t, "n2"); got != "secret plan" {
		t.Fatalf("B decrypted %q", got)
	}
	a.mustSync(t)
	if got := a.content(t, "n1"); got != "B while locked" {
		t.Fatalf("A got %q", got)
	}

	// Delete propagates.
	if _, err := a.store.Edit("n2", "device-a", nil, nil, true); err != nil {
		t.Fatal(err)
	}
	a.mustSync(t)
	b.mustSync(t)
	if n, _ := b.store.Get("n2"); !n.Deleted {
		t.Fatal("delete did not propagate")
	}

	// Disable E2E from B: server content is plaintext again, A follows.
	if err := b.settings.DisableE2E(); err != nil {
		t.Fatalf("disable: %v", err)
	}
	for _, c := range serverContents(t, dbPath) {
		if strings.HasPrefix(c, secure.E2EPrefix) {
			t.Fatalf("ciphertext left after disabling: %q", c)
		}
	}
	a.mustSync(t)
	if a.sync.Status().E2EEnabled {
		t.Fatal("A still has E2E key after it was disabled")
	}

	// A bad token is reported clearly.
	if _, err := a.settings.TestConnection(pogosync.Settings{Host: "127.0.0.1", Port: port, Token: "pogo_wrong"}); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("bad token: %v", err)
	}
	if msg, err := a.settings.TestConnection(a.settings.Get()); err != nil || !strings.Contains(msg, "Token OK") {
		t.Fatalf("good token: %q %v", msg, err)
	}
}

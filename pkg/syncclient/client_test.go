package syncclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBaseURL(t *testing.T) {
	cases := []struct{ scheme, host, port, want string }{
		{"http", "192.168.1.10", "8080", "http://192.168.1.10:8080"},
		{"https", " pad.example.com ", "", "https://pad.example.com"},
		{"https", "pad.example.com", " 443 ", "https://pad.example.com:443"},
		{"", "localhost", "8080", "http://localhost:8080"},
		{"gopher", "localhost", "", "http://localhost"},
		// A pasted URL overrides the scheme field.
		{"http", "https://pad.example.com", "", "https://pad.example.com"},
		{"https", "http://10.0.0.2:9000", "", "http://10.0.0.2:9000"},
		// IPv6 literals need brackets once a port is added.
		{"http", "::1", "8080", "http://[::1]:8080"},
		{"http", "[::1]", "8080", "http://[::1]:8080"},
		{"http", "fe80::1", "", "http://[fe80::1]"},
	}
	for _, c := range cases {
		got, err := BaseURL(c.scheme, c.host, c.port)
		if err != nil || got != c.want {
			t.Errorf("BaseURL(%q, %q, %q) = %q, %v; want %q", c.scheme, c.host, c.port, got, err, c.want)
		}
	}
	for _, c := range []struct{ scheme, host, port string }{
		{"http", "", "8080"},
		{"http", "   ", ""},
		{"http", "http://pad.example.com:8080", "9000"},
		{"http", "pad example.com", ""},
		{"http", "ftp://pad.example.com", ""},
	} {
		if got, err := BaseURL(c.scheme, c.host, c.port); err == nil {
			t.Errorf("BaseURL(%q, %q, %q) = %q, want an error", c.scheme, c.host, c.port, got)
		}
	}
}

// fake answers each request with the next canned status and body, and
// records what it was sent.
type fake struct {
	status int
	body   string
	got    []*http.Request
	bodies []string
}

func (f *fake) client(t *testing.T) *Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.got = append(f.got, r)
		f.bodies = append(f.bodies, string(b))
		w.WriteHeader(f.status)
		io.WriteString(w, f.body)
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL+"/", "pogo_secret")
}

func TestRequests(t *testing.T) {
	ctx := context.Background()
	f := &fake{status: 200, body: `{"cursor":7,"changes":[{"id":"n1","content":"hi","updated_at":5,"rev":7}],"more":true}`}
	c := f.client(t)

	resp, err := c.Sync(ctx, SyncRequest{Cursor: 3})
	if err != nil || resp.Cursor != 7 || !resp.More || len(resp.Changes) != 1 || resp.Changes[0].Content != "hi" {
		t.Fatalf("sync: %+v, %v", resp, err)
	}
	r := f.got[0]
	if r.Method != "POST" || r.URL.Path != "/api/v1/sync" {
		t.Errorf("sync request: %s %s", r.Method, r.URL.Path)
	}
	if r.Header.Get("Authorization") != "Bearer pogo_secret" || r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("headers: %v", r.Header)
	}
	// The server rejects a null change list, so nil is sent as [].
	var sent map[string]any
	json.Unmarshal([]byte(f.bodies[0]), &sent)
	if changes, ok := sent["changes"].([]any); !ok || len(changes) != 0 || sent["cursor"] != 3.0 {
		t.Errorf("sync body: %s", f.bodies[0])
	}

	f.status, f.body = 204, ""
	if err := c.PutE2E(ctx, E2EParams{KDF: "argon2id", Salt: "s", Check: "c"}, true); err != nil {
		t.Fatal(err)
	}
	if r := f.got[1]; r.Method != "PUT" || r.URL.RequestURI() != "/api/v1/e2e?force=1" || !strings.Contains(f.bodies[1], `"salt":"s"`) {
		t.Errorf("put e2e: %s %s %s", r.Method, r.URL.RequestURI(), f.bodies[1])
	}
	if err := c.PutE2E(ctx, E2EParams{}, false); err != nil || f.got[2].URL.RequestURI() != "/api/v1/e2e" {
		t.Errorf("put e2e without force: %v %s", err, f.got[2].URL.RequestURI())
	}
	if err := c.DeleteE2E(ctx); err != nil || f.got[3].Method != "DELETE" {
		t.Errorf("delete e2e: %v", err)
	}

	f.status, f.body = 200, `{"kdf":"argon2id","salt":"s","check":"c"}`
	if p, err := c.GetE2E(ctx); err != nil || p.Salt != "s" || p.Check != "c" {
		t.Errorf("get e2e: %+v %v", p, err)
	}
}

func TestErrors(t *testing.T) {
	ctx := context.Background()
	for status, want := range map[int]error{401: ErrUnauthorized, 404: ErrNotFound, 409: ErrConflict} {
		f := &fake{status: status, body: `{"error":"nope"}`}
		if _, err := f.client(t).GetE2E(ctx); !errors.Is(err, want) {
			t.Errorf("status %d: %v, want %v", status, err, want)
		}
	}

	f := &fake{status: 413, body: `{"error":"quota exceeded"}`}
	if _, err := f.client(t).Sync(ctx, SyncRequest{}); err == nil || !strings.Contains(err.Error(), "quota exceeded") {
		t.Errorf("server message not surfaced: %v", err)
	}
	f = &fake{status: 502, body: `<html>Bad Gateway</html>`}
	if _, err := f.client(t).Sync(ctx, SyncRequest{}); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("non-JSON error: %v", err)
	}

	if _, err := New("http://127.0.0.1:1", "").Health(ctx); err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Errorf("unreachable: %v", err)
	}
}

func TestHealth(t *testing.T) {
	ctx := context.Background()
	f := &fake{status: 200, body: `{"ok":true,"version":"v1.2.3"}`}
	c := f.client(t)
	if v, err := c.Health(ctx); err != nil || v != "v1.2.3" {
		t.Fatalf("health: %q %v", v, err)
	}
	if f.got[0].URL.Path != "/api/v1/health" {
		t.Errorf("path %q", f.got[0].URL.Path)
	}
	f.body = `{"ok":false}`
	if _, err := c.Health(ctx); err == nil {
		t.Error("unhealthy server accepted")
	}
	// Some other web server at that address.
	f.status, f.body = 404, "not found"
	if _, err := c.Health(ctx); err == nil || !strings.Contains(err.Error(), "not a Pogo Pad") {
		t.Errorf("wrong server: %v", err)
	}
}

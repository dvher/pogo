package notefile

import (
	"bytes"
	"crypto/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dvher/pogo/pkg/secure"
	"github.com/dvher/pogo/pkg/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	key := make([]byte, 32)
	rand.Read(key)
	c, _ := secure.NewCipher(key)
	s, err := store.Open(t.TempDir(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMarkdownRoundTrip(t *testing.T) {
	n := store.Note{
		ID: "3f0c-id", Content: "---\nnot frontmatter\n\n# Plan\n- [ ] a\n", Color: "#12ab34",
		CreatedAt: 1761800000123, UpdatedAt: 1761800500456,
	}
	it := ParseMarkdown("Plan.md", Markdown(n))
	if it.ID != n.ID || it.Color != n.Color || it.Content != n.Content ||
		it.CreatedAt != n.CreatedAt || it.UpdatedAt != n.UpdatedAt {
		t.Fatalf("round trip: %+v", it)
	}
}

func TestParseObsidian(t *testing.T) {
	it := ParseMarkdown("a.md", []byte("plain note\n"))
	if it.ID != "" || it.Content != "plain note\n" {
		t.Fatalf("plain: %+v", it)
	}
	// Other frontmatter stays in the content; Pogo's keys are taken out.
	src := "---\ntags:\n  - work\nid: n1\nupdated: 2026-10-30\n---\nbody\n"
	it = ParseMarkdown("b.md", []byte(src))
	if it.ID != "n1" || it.Content != "---\ntags:\n  - work\n---\nbody\n" || it.UpdatedAt == 0 {
		t.Fatalf("obsidian: %+v", it)
	}
	// An id that Pogo can't use is kept as a property.
	it = ParseMarkdown("c.md", []byte("---\nid: has spaces\n---\nx"))
	if it.ID != "" || !strings.Contains(it.Content, "id: has spaces") {
		t.Fatalf("bad id: %+v", it)
	}
}

func TestFilename(t *testing.T) {
	used := map[string]bool{}
	for _, c := range []struct{ title, want string }{
		{"Shopping: milk/eggs?", "Shopping milk eggs.md"},
		{"shopping milk eggs", "shopping milk eggs 2.md"},
		{"  ...  ", "Untitled.md"},
		{"[[Link]] #tag", "Link tag.md"},
		{strings.Repeat("é", 100), strings.Repeat("é", 80) + ".md"},
	} {
		if got := Filename(c.title, ".md", used); got != c.want {
			t.Errorf("Filename(%q) = %q, want %q", c.title, got, c.want)
		}
	}
}

func TestTitle(t *testing.T) {
	if got := Title("\n# **Groceries**\n- [ ] milk"); got != "Groceries" {
		t.Fatal(got)
	}
	if got := Title("- [x] [done](http://x)"); got != "done" {
		t.Fatal(got)
	}
}

func TestHTML(t *testing.T) {
	out, err := HTML(store.Note{Content: "# Hi <b>\n\n- [x] done\n- [ ] todo\n\nline one\nline two\n<script>alert(1)</script>\n", Color: "pink"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{"<title>Hi &lt;b&gt;</title>", `checked="" disabled="" type="checkbox"`, "line one<br>", "#f8bbd0"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(s, "<script>") {
		t.Error("raw HTML rendered")
	}
}

func TestZipRoundTrip(t *testing.T) {
	notes := []store.Note{
		{ID: "a", Content: "# Same\n1", Color: "yellow", CreatedAt: 1000, UpdatedAt: 2000},
		{ID: "b", Content: "# Same\n2", Color: "blue", CreatedAt: 3000, UpdatedAt: 4000},
	}
	var buf bytes.Buffer
	if err := WriteZip(&buf, notes); err != nil {
		t.Fatal(err)
	}
	items, err := Parse("pogo.zip", buf.Bytes(), time.Time{})
	if err != nil || len(items) != 2 {
		t.Fatalf("parse: %v %+v", err, items)
	}
	names := map[string]bool{}
	byID := map[string]store.Note{"a": notes[0], "b": notes[1]}
	for i, it := range items {
		names[it.Name] = true
		n := byID[it.ID]
		if it.ID != n.ID || it.Content != n.Content || it.Color != n.Color || it.UpdatedAt != n.UpdatedAt {
			t.Errorf("item %d: %+v", i, it)
		}
	}
	if !names["Same.md"] || !names["Same 2.md"] {
		t.Errorf("names: %v", names)
	}
}

func TestParseFolder(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".obsidian"), 0o755)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, ".obsidian", "x.md"), []byte("config"), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "n.MD"), []byte("nested"), 0o644)
	os.WriteFile(filepath.Join(dir, "img.png"), []byte("png"), 0o644)
	items, err := ParseFS(os.DirFS(dir))
	if err != nil || len(items) != 1 || items[0].Content != "nested" || items[0].UpdatedAt == 0 {
		t.Fatalf("folder: %v %+v", err, items)
	}
}

func TestImport(t *testing.T) {
	st := openStore(t)
	items := []Item{
		{ID: "a", Content: "from pogo", Color: "blue", CreatedAt: 1, UpdatedAt: 2},
		{Content: "from obsidian"},
		{Content: "bad color", Color: "chartreuse"},
		{Content: strings.Repeat("x", MaxContent+1)},
	}
	r, err := Import(st, "dev", items)
	if err != nil || r != (Report{Created: 3, Skipped: 1}) {
		t.Fatalf("first import: %+v %v", r, err)
	}
	// Importing the same files again changes nothing.
	r, _ = Import(st, "dev", items[:3])
	if r != (Report{Skipped: 3}) {
		t.Fatalf("second import: %+v", r)
	}
	// A newer copy of a Pogo note updates it.
	r, _ = Import(st, "dev", []Item{{ID: "a", Content: "edited", Color: "blue", UpdatedAt: 3}})
	if n, _ := st.Get("a"); r.Updated != 1 || n.Content != "edited" {
		t.Fatalf("update: %+v %+v", r, n)
	}
	notes, _ := st.List()
	for _, n := range notes {
		if n.Content == "bad color" && n.Color != "yellow" {
			t.Errorf("invalid color kept: %q", n.Color)
		}
	}
}

// Package notefile turns notes into files and back: Markdown with YAML
// frontmatter (one note per file, readable by Obsidian and other Markdown
// apps), standalone HTML for reading and printing, and zip archives of every
// note that another Pogo install can import.
package notefile

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dvher/pogo/pkg/store"
)

// MaxContent is the largest note content Pogo accepts, as in the editor and
// the sync server.
const MaxContent = 1 << 20

// Item is a note read from a file. ID is empty when the file did not come
// from Pogo; times are unix ms, or 0 when unknown.
type Item struct {
	Name      string
	ID        string
	Content   string
	Color     string
	CreatedAt int64
	UpdatedAt int64
}

const timeLayout = "2006-01-02T15:04:05.000Z07:00"

// Markdown returns the note as a Markdown file: Pogo's fields as frontmatter,
// then the content unchanged.
func Markdown(n store.Note) []byte {
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "id: %s\n", n.ID)
	fmt.Fprintf(&b, "color: %q\n", n.Color)
	fmt.Fprintf(&b, "created: %s\n", time.UnixMilli(n.CreatedAt).UTC().Format(timeLayout))
	fmt.Fprintf(&b, "updated: %s\n", time.UnixMilli(n.UpdatedAt).UTC().Format(timeLayout))
	b.WriteString("---\n")
	b.WriteString(n.Content)
	return b.Bytes()
}

var (
	pogoKey = regexp.MustCompile(`^(id|color|created|updated):[ \t]*(.*?)[ \t]*$`)
	validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

var timeLayouts = []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"}

func parseTime(s string) int64 {
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.UnixMilli()
		}
	}
	return 0
}

// ParseMarkdown reads a Markdown file. Pogo's frontmatter keys are taken out;
// any other frontmatter (such as Obsidian tags) stays in the content.
func ParseMarkdown(name string, data []byte) Item {
	it := Item{Name: name}
	src := string(data)
	nl := "\n"
	if strings.HasPrefix(src, "---\r\n") {
		nl = "\r\n"
	}
	if !strings.HasPrefix(src, "---"+nl) {
		it.Content = src
		return it
	}
	lines := strings.SplitAfter(src[len("---"+nl):], nl)
	end := -1
	for i, l := range lines {
		if t := strings.TrimRight(l, "\r\n"); t == "---" || t == "..." {
			end = i
			break
		}
	}
	if end < 0 {
		it.Content = src
		return it
	}
	var kept []string
	for _, l := range lines[:end] {
		m := pogoKey.FindStringSubmatch(strings.TrimRight(l, "\r\n"))
		if m == nil {
			kept = append(kept, l)
			continue
		}
		v := unquote(m[2])
		switch m[1] {
		case "id":
			if !validID.MatchString(v) {
				kept = append(kept, l)
				continue
			}
			it.ID = v
		case "color":
			it.Color = v
		case "created":
			it.CreatedAt = parseTime(v)
		case "updated":
			it.UpdatedAt = parseTime(v)
		}
	}
	body := strings.Join(lines[end+1:], "")
	if strings.TrimSpace(strings.Join(kept, "")) != "" {
		body = "---" + nl + strings.Join(kept, "") + "---" + nl + body
	}
	it.Content = body
	return it
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		return s[1 : len(s)-1]
	}
	return s
}

var titlePrefix = regexp.MustCompile(`^\s*(#+\s*|(?:[-*+]|\d+[.)])\s+)(\[[ xX]\]\s*)?`)

// Title returns the first meaningful line of a note, without Markdown.
func Title(src string) string {
	for _, l := range strings.Split(src, "\n") {
		if t := StripInline(strings.TrimSpace(titlePrefix.ReplaceAllString(l, ""))); t != "" {
			return t
		}
	}
	return "Empty note"
}

var inlineRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`), "$1"},              // images
	{regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`), "$1"},               // links
	{regexp.MustCompile("`([^`]+)`"), "$1"},                           // code
	{regexp.MustCompile(`\*\*([^*]+)\*\*`), "$1"},                     // bold
	{regexp.MustCompile(`__([^_]+)__`), "$1"},                         // bold
	{regexp.MustCompile(`~~([^~]+)~~`), "$1"},                         // strikethrough
	{regexp.MustCompile(`\*([^*\s][^*]*)\*`), "$1"},                   // italic
	{regexp.MustCompile(`(^|\W)_([^_\s][^_]*)_(\W|$)`), "$1$2$3"},     // italic, not snake_case
	{regexp.MustCompile(`\\([\\*_\[\]()#+\-.!` + "`" + `~>])`), "$1"}, // escapes
}

// StripInline removes inline Markdown (links, emphasis, code) from a line.
func StripInline(s string) string {
	for _, r := range inlineRules {
		s = r.re.ReplaceAllString(s, r.repl)
	}
	return s
}

// unsafeName matches characters that are invalid in file names on some
// system, or that Obsidian does not allow in note names.
var unsafeName = regexp.MustCompile(`[/\\:*?"<>|#^\[\]\x00-\x1f\x7f]+`)

// Filename returns a safe file name for a note titled title, with extension
// ext (such as ".md"). If used is not nil, the name is made unique among the
// names in it (case-insensitively) and added to it.
func Filename(title, ext string, used map[string]bool) string {
	base := strings.Join(strings.Fields(unsafeName.ReplaceAllString(title, " ")), " ")
	base = strings.Trim(base, ". ")
	if r := []rune(base); len(r) > 80 {
		base = strings.TrimRight(string(r[:80]), ". ")
	}
	if base == "" {
		base = "Untitled"
	}
	name := base + ext
	if used == nil {
		return name
	}
	for i := 2; used[strings.ToLower(name)]; i++ {
		name = fmt.Sprintf("%s %d%s", base, i, ext)
	}
	used[strings.ToLower(name)] = true
	return name
}

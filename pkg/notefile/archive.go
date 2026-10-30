package notefile

import (
	"archive/zip"
	"bytes"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/dvher/pogo/pkg/store"
)

// WriteZip writes every note as a Markdown file at the root of a zip archive.
// Unzipped, the archive is a folder that Obsidian can open as a vault.
func WriteZip(w io.Writer, notes []store.Note) error {
	zw := zip.NewWriter(w)
	used := map[string]bool{}
	for _, n := range notes {
		f, err := zw.CreateHeader(&zip.FileHeader{
			Name:     Filename(Title(n.Content), ".md", used),
			Method:   zip.Deflate,
			Modified: time.UnixMilli(n.UpdatedAt),
		})
		if err != nil {
			return err
		}
		if _, err := f.Write(Markdown(n)); err != nil {
			return err
		}
	}
	return zw.Close()
}

// Parse reads notes from a file: a zip archive (every Markdown file inside)
// or a single Markdown file. modTime is the file's modification time, used
// for notes without timestamps; it may be zero.
func Parse(name string, data []byte, modTime time.Time) ([]Item, error) {
	if strings.EqualFold(path.Ext(name), ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		return ParseFS(zr)
	}
	it := ParseMarkdown(name, data)
	fillTimes(&it, modTime)
	return []Item{it}, nil
}

// ParseFS reads every Markdown file in fsys, such as an unzipped export or an
// Obsidian vault. Hidden folders (.obsidian, .trash) are skipped.
func ParseFS(fsys fs.FS) ([]Item, error) {
	var items []Item
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != "." && (strings.HasPrefix(name, ".") || name == "__MACOSX") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !strings.EqualFold(path.Ext(name), ".md") {
			return nil
		}
		f, err := fsys.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		// Read one byte past the limit so Import can tell the note is too long
		// without a huge file filling memory.
		data, err := io.ReadAll(io.LimitReader(f, MaxContent+1024))
		if err != nil {
			return err
		}
		var mod time.Time
		if info, err := d.Info(); err == nil {
			mod = info.ModTime()
		}
		it := ParseMarkdown(p, data)
		fillTimes(&it, mod)
		items = append(items, it)
		return nil
	})
	return items, err
}

func fillTimes(it *Item, mod time.Time) {
	if it.UpdatedAt == 0 && !mod.IsZero() {
		it.UpdatedAt = mod.UnixMilli()
	}
	if it.CreatedAt == 0 {
		it.CreatedAt = it.UpdatedAt
	}
}

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/dvher/pogo/pkg/notefile"
	"github.com/dvher/pogo/pkg/store"
)

// ExportNote saves one note as "md" or "html" through a save dialog, or opens
// a print window for "pdf", where the system print dialog can save a PDF.
// It returns the saved path, or "" if the user cancelled.
func (s *NoteService) ExportNote(id, format string) (string, error) {
	n, err := s.store.Get(id)
	if err != nil {
		return "", err
	}
	var data []byte
	var filter string
	switch format {
	case "md":
		data, filter = notefile.Markdown(n), "Markdown"
	case "html":
		if data, err = notefile.HTML(n); err != nil {
			return "", err
		}
		filter = "Web page"
	case "pdf":
		s.windows.OpenPrint(n)
		return "", nil
	default:
		return "", fmt.Errorf("unknown format %q", format)
	}
	name := notefile.Filename(notefile.Title(n.Content), "."+format, nil)
	return s.saveFile(name, filter, "*."+format, data)
}

// ExportAll saves every note as Markdown files in a zip archive. Unzipped, it
// opens as an Obsidian vault; Import reads it back.
func (s *NoteService) ExportAll() (string, error) {
	notes, err := s.store.List()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := notefile.WriteZip(&buf, notes); err != nil {
		return "", err
	}
	name := "pogo-notes-" + time.Now().Format("2006-01-02") + ".zip"
	return s.saveFile(name, "Zip archive", "*.zip", buf.Bytes())
}

func (s *NoteService) saveFile(name, filterName, pattern string, data []byte) (string, error) {
	path, err := s.app.Dialog.SaveFile().
		SetFilename(name).
		AddFilter(filterName, pattern).
		CanCreateDirectories(true).
		PromptForSingleSelection()
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o644)
}

// Import adds notes from zip archives or Markdown files picked by the user.
func (s *NoteService) Import() (notefile.Report, error) {
	paths, err := s.app.Dialog.OpenFile().
		SetTitle("Import notes").
		AddFilter("Notes (zip, Markdown)", "*.zip;*.md").
		PromptForMultipleSelection()
	if err != nil || len(paths) == 0 {
		return notefile.Report{}, err
	}
	var items []notefile.Item
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return notefile.Report{}, err
		}
		var mod time.Time
		if info, err := os.Stat(p); err == nil {
			mod = info.ModTime()
		}
		got, err := notefile.Parse(filepath.Base(p), data, mod)
		if err != nil {
			return notefile.Report{}, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		items = append(items, got...)
	}
	return s.importItems(items)
}

// ImportFolder adds every Markdown file in a folder picked by the user, such
// as an unzipped export or an Obsidian vault.
func (s *NoteService) ImportFolder() (notefile.Report, error) {
	dir, err := s.app.Dialog.OpenFile().
		SetTitle("Import a folder of notes").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		PromptForSingleSelection()
	if err != nil || dir == "" {
		return notefile.Report{}, err
	}
	items, err := notefile.ParseFS(os.DirFS(dir))
	if err != nil {
		return notefile.Report{}, err
	}
	return s.importItems(items)
}

func (s *NoteService) importItems(items []notefile.Item) (notefile.Report, error) {
	r, err := notefile.Import(s.store, s.deviceID, items)
	if r.Created+r.Updated > 0 {
		// Imported notes start hidden; refresh the open ones that changed.
		if notes, err := s.store.List(); err == nil {
			for _, n := range notes {
				if s.windows.IsOpen(n.ID) {
					s.app.Event.Emit(EventNoteChanged, n)
				}
			}
		}
		s.app.Event.Emit(EventListChanged)
		s.sync.Nudge()
	}
	return r, err
}

// PrintHTML returns a note as a printable web page, for the print window.
func (s *NoteService) PrintHTML(id string) (string, error) {
	n, err := s.store.Get(id)
	if err != nil {
		return "", err
	}
	html, err := notefile.HTML(n)
	return string(html), err
}

// OpenPrint opens a window showing n as a printable page; the page starts
// the system print dialog, which can save a PDF.
func (wm *WindowManager) OpenPrint(n store.Note) {
	w := wm.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "print-" + n.ID + "-" + fmt.Sprint(time.Now().UnixNano()),
		Title:  notefile.Title(n.Content) + " – Pogo",
		URL:    "/?print=" + n.ID,
		Width:  760,
		Height: 900,
	})
	w.Show()
	w.Focus()
}

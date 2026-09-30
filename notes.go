package main

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"

	"notes-desktop/internal/store"
)

// Events emitted to the frontend.
const (
	EventNoteChanged = "note:changed"  // data: store.Note
	EventListChanged = "notes:changed" // any note was added, removed, shown or hidden
	EventSyncStatus  = "sync:status"   // data: SyncStatus
)

func init() {
	application.RegisterEvent[store.Note](EventNoteChanged)
	application.RegisterEvent[application.Void](EventListChanged)
	application.RegisterEvent[SyncStatus](EventSyncStatus)
}

const defaultNoteContent = "# New note\n\n- [ ] something to do\n"

// NoteService is the frontend's API for reading and editing notes.
type NoteService struct {
	app      *application.App
	store    *store.Store
	windows  *WindowManager
	sync     *SyncService
	deviceID string
	manager  *application.WebviewWindow
}

func (s *NoteService) List() ([]store.Note, error) { return s.store.List() }

func (s *NoteService) Get(id string) (store.Note, error) { return s.store.Get(id) }

// Create adds a new note and opens its window.
func (s *NoteService) Create() (store.Note, error) {
	now := time.Now().UnixMilli()
	n := store.Note{
		ID: uuid.NewString(), Content: defaultNoteContent, Color: "yellow",
		CreatedAt: now, UpdatedAt: now, DeviceID: s.deviceID, W: 260, H: 260,
	}
	if err := s.store.Insert(n); err != nil {
		return n, err
	}
	s.windows.Open(n)
	s.changed(n, true)
	return n, nil
}

func (s *NoteService) SetContent(id, content string) (store.Note, error) {
	if len(content) > 1<<20 {
		return store.Note{}, fmt.Errorf("note is too long (max 1 MiB)")
	}
	n, err := s.store.Edit(id, s.deviceID, &content, nil, false)
	if err == nil {
		s.changed(n, false)
	}
	return n, err
}

func (s *NoteService) SetColor(id, color string) (store.Note, error) {
	if _, ok := noteColors[color]; !ok {
		return store.Note{}, fmt.Errorf("unknown color %q", color)
	}
	n, err := s.store.Edit(id, s.deviceID, nil, &color, false)
	if err == nil {
		s.changed(n, false)
	}
	return n, err
}

// SetAnchored locks or unlocks a note in place.
func (s *NoteService) SetAnchored(id string, anchored bool) (store.Note, error) {
	if err := s.store.SetAnchored(id, anchored); err != nil {
		return store.Note{}, err
	}
	s.windows.SetAnchored(id, anchored)
	n, err := s.store.Get(id)
	if err == nil {
		s.app.Event.Emit(EventNoteChanged, n)
	}
	return n, err
}

// Hide closes a note's window; it stays in the manager.
func (s *NoteService) Hide(id string) error {
	if err := s.store.SetHidden(id, true); err != nil {
		return err
	}
	s.windows.Close(id)
	s.app.Event.Emit(EventListChanged)
	return nil
}

// Show opens a note's window.
func (s *NoteService) Show(id string) error {
	if err := s.store.SetHidden(id, false); err != nil {
		return err
	}
	n, err := s.store.Get(id)
	if err != nil {
		return err
	}
	s.windows.Open(n)
	s.app.Event.Emit(EventListChanged)
	return nil
}

func (s *NoteService) ShowAll() error { return s.setAllHidden(false) }
func (s *NoteService) HideAll() error { return s.setAllHidden(true) }

func (s *NoteService) setAllHidden(hidden bool) error {
	notes, err := s.store.List()
	if err != nil {
		return err
	}
	for _, n := range notes {
		if hidden {
			err = s.Hide(n.ID)
		} else {
			err = s.Show(n.ID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Delete removes a note everywhere (a tombstone is synced).
func (s *NoteService) Delete(id string) error {
	n, err := s.store.Edit(id, s.deviceID, nil, nil, true)
	if err != nil {
		return err
	}
	s.windows.Close(id)
	s.changed(n, true)
	return nil
}

// OpenManager shows the note manager window.
func (s *NoteService) OpenManager() {
	s.manager.Show()
	s.manager.Focus()
}

func (s *NoteService) changed(n store.Note, listChanged bool) {
	s.app.Event.Emit(EventNoteChanged, n)
	if listChanged {
		s.app.Event.Emit(EventListChanged)
	}
	s.sync.Nudge()
}

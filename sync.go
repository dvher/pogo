package main

import (
	"context"
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/dvher/pogo/pkg/pogosync"
	"github.com/dvher/pogo/pkg/store"
)

// Local settings keys (sync settings live in pkg/pogosync).
const keyWelcomed = "welcomed"

// SyncService exposes the sync engine to the frontend and keeps note
// windows in step with incoming changes.
type SyncService struct {
	app     *application.App
	store   *store.Store
	windows *WindowManager
	engine  *pogosync.Engine
}

func NewSyncService(s *store.Store, wm *WindowManager, deviceID string) *SyncService {
	svc := &SyncService{store: s, windows: wm, engine: pogosync.New(s, deviceID)}
	svc.engine.OnStatus = func(st pogosync.Status) { svc.emit(EventSyncStatus, st) }
	svc.engine.OnApplied = svc.applied
	return svc
}

// Status returns the current sync status.
func (s *SyncService) Status() pogosync.Status { return s.engine.Status() }

// SyncNow runs a sync immediately and returns its error, if any.
func (s *SyncService) SyncNow() error { return s.engine.SyncNow() }

// Nudge schedules a sync shortly after a local edit.
func (s *SyncService) Nudge() { s.engine.Nudge() }

// Loop runs until ctx is cancelled.
func (s *SyncService) Loop(ctx context.Context) { s.engine.Loop(ctx) }

// emit sends an event to the frontend (a no-op in tests, where there is no app).
func (s *SyncService) emit(name string, data ...any) {
	if s.app != nil {
		s.app.Event.Emit(name, data...)
	}
}

func (s *SyncService) applied(changes []pogosync.Applied) {
	for _, c := range changes {
		// Notes from other devices start hidden; the user shows them from
		// the manager.
		if c.Created {
			if err := s.store.SetHidden(c.ID, true); err != nil {
				slog.Error("hide synced note", "note", c.ID, "error", err)
			}
		}
		n, err := s.store.Get(c.ID)
		if err != nil || s.windows == nil {
			continue
		}
		if n.Deleted {
			s.windows.Close(c.ID)
		} else {
			s.emit(EventNoteChanged, n)
		}
	}
	s.emit(EventListChanged)
}

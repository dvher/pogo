package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"notes-desktop/internal/secure"
	"notes-desktop/internal/store"
	"notes-desktop/internal/syncclient"
)

// Settings keys.
const (
	keyScheme     = "server_scheme"
	keyHost       = "server_host"
	keyPort       = "server_port"
	keyToken      = "server_token" // secret
	keyEnabled    = "sync_enabled"
	keyInterval   = "sync_interval"
	keyCursor     = "sync_cursor"
	keyE2EKey     = "e2e_key" // secret, hex
	keyDeviceID   = "device_id"
	uploadBatch   = 1000
	nudgeDelay    = 2 * time.Second
	minInterval   = 10
	defaultPeriod = 30
)

var errLocked = errors.New("notes on this server are end-to-end encrypted: enter the passphrase in Sync settings")

// SyncStatus is shown in the manager's Sync tab.
type SyncStatus struct {
	Enabled    bool   `json:"enabled"`
	Syncing    bool   `json:"syncing"`
	LastSync   int64  `json:"lastSync"` // unix ms of last success
	LastError  string `json:"lastError"`
	E2EServer  bool   `json:"e2eServer"`  // server has E2E set up
	E2ELocked  bool   `json:"e2eLocked"`  // ...but this device has no valid key
	E2EEnabled bool   `json:"e2eEnabled"` // this device encrypts uploads
}

// SyncService periodically exchanges changes with the configured server.
type SyncService struct {
	app      *application.App
	store    *store.Store
	windows  *WindowManager
	deviceID string

	runMu  sync.Mutex // serialises sync runs
	mu     sync.Mutex
	status SyncStatus
	nudge  *time.Timer
	kick   chan struct{}
}

func NewSyncService(s *store.Store, wm *WindowManager, deviceID string) *SyncService {
	return &SyncService{store: s, windows: wm, deviceID: deviceID, kick: make(chan struct{}, 1)}
}

// Status returns the current sync status.
func (s *SyncService) Status() SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	st.Enabled = s.store.Setting(keyEnabled, "0") == "1"
	st.E2EEnabled = s.store.SecretSetting(keyE2EKey) != ""
	return st
}

// SyncNow runs a sync immediately and returns its error, if any.
func (s *SyncService) SyncNow() error { return s.run(context.Background()) }

// Nudge schedules a sync shortly after a local edit.
func (s *SyncService) Nudge() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nudge != nil {
		s.nudge.Stop()
	}
	s.nudge = time.AfterFunc(nudgeDelay, func() {
		select {
		case s.kick <- struct{}{}:
		default:
		}
	})
}

func (s *SyncService) interval() time.Duration {
	sec := max(s.store.IntSetting(keyInterval, defaultPeriod), minInterval)
	return time.Duration(sec) * time.Second
}

// Loop runs until ctx is cancelled.
func (s *SyncService) Loop(ctx context.Context) {
	timer := time.NewTimer(time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-s.kick:
		}
		if err := s.run(ctx); err != nil && !errors.Is(err, errDisabled) {
			slog.Warn("sync failed", "error", err)
		}
		timer.Reset(s.interval())
	}
}

var errDisabled = errors.New("sync is disabled")

func (s *SyncService) client() (*syncclient.Client, error) {
	base, err := syncclient.BaseURL(s.store.Setting(keyScheme, "http"), s.store.Setting(keyHost, ""), s.store.Setting(keyPort, ""))
	if err != nil {
		return nil, err
	}
	return syncclient.New(base, s.store.SecretSetting(keyToken)), nil
}

func (s *SyncService) setStatus(f func(*SyncStatus)) {
	s.mu.Lock()
	f(&s.status)
	s.mu.Unlock()
	s.emit(EventSyncStatus, s.Status())
}

// emit sends an event to the frontend (a no-op in tests, where there is no app).
func (s *SyncService) emit(name string, data ...any) {
	if s.app != nil {
		s.app.Event.Emit(name, data...)
	}
}

func (s *SyncService) run(ctx context.Context) error {
	if s.store.Setting(keyEnabled, "0") != "1" || s.store.Setting(keyHost, "") == "" {
		return errDisabled
	}
	s.runMu.Lock()
	defer s.runMu.Unlock()

	s.setStatus(func(st *SyncStatus) { st.Syncing = true })
	err := s.exchange(ctx)
	s.setStatus(func(st *SyncStatus) {
		st.Syncing = false
		if err != nil {
			st.LastError = err.Error()
		} else {
			st.LastError, st.LastSync = "", time.Now().UnixMilli()
		}
	})
	return err
}

// e2eCipher checks the server's E2E setup against the local key.
func (s *SyncService) e2eCipher(ctx context.Context, c *syncclient.Client) (*secure.Cipher, error) {
	params, err := c.GetE2E(ctx)
	if errors.Is(err, syncclient.ErrNotFound) {
		s.setStatus(func(st *SyncStatus) { st.E2EServer, st.E2ELocked = false, false })
		if s.store.SecretSetting(keyE2EKey) != "" {
			// Turned off from another device.
			s.store.SetSecretSetting(keyE2EKey, "")
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cipher := s.localE2ECipher()
	if cipher == nil || !cipher.VerifyCheck(params.Check) {
		s.store.SetSecretSetting(keyE2EKey, "")
		s.setStatus(func(st *SyncStatus) { st.E2EServer, st.E2ELocked = true, true })
		return nil, errLocked
	}
	s.setStatus(func(st *SyncStatus) { st.E2EServer, st.E2ELocked = true, false })
	return cipher, nil
}

func (s *SyncService) localE2ECipher() *secure.Cipher {
	key, err := hex.DecodeString(s.store.SecretSetting(keyE2EKey))
	if err != nil || len(key) != 32 {
		return nil
	}
	c, _ := secure.NewCipher(key)
	return c
}

type e2ePayload struct {
	Content string `json:"content"`
	Color   string `json:"color"`
}

func encodeNote(n store.Note, e2e *secure.Cipher) syncclient.Note {
	out := syncclient.Note{ID: n.ID, Content: n.Content, Color: n.Color, Deleted: n.Deleted, UpdatedAt: n.UpdatedAt, DeviceID: n.DeviceID}
	if e2e != nil {
		out.Color = ""
		if !n.Deleted {
			b, _ := json.Marshal(e2ePayload{n.Content, n.Color})
			out.Content = secure.E2EPrefix + e2e.Seal(b)
		}
	}
	return out
}

func decodeNote(n syncclient.Note, e2e *secure.Cipher) (store.Remote, error) {
	r := store.Remote{ID: n.ID, Content: n.Content, Color: n.Color, Deleted: n.Deleted, UpdatedAt: n.UpdatedAt, DeviceID: n.DeviceID}
	if sealed, ok := strings.CutPrefix(n.Content, secure.E2EPrefix); ok {
		if e2e == nil {
			return r, errLocked
		}
		b, err := e2e.Open(sealed)
		if err != nil {
			return r, err
		}
		var p e2ePayload
		if err := json.Unmarshal(b, &p); err != nil {
			return r, err
		}
		r.Content, r.Color = p.Content, p.Color
	}
	if _, ok := noteColors[r.Color]; !ok {
		r.Color = "yellow"
	}
	return r, nil
}

func (s *SyncService) exchange(ctx context.Context) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	e2e, err := s.e2eCipher(ctx, c)
	if err != nil {
		return err
	}

	dirty, err := s.store.Dirty()
	if err != nil {
		return err
	}
	cursor := s.store.IntSetting(keyCursor, 0)
	type result struct {
		changed, created bool
	}
	results := map[string]result{}

	for {
		batch := dirty[:min(len(dirty), uploadBatch)]
		dirty = dirty[len(batch):]
		req := syncclient.SyncRequest{Cursor: cursor, Changes: make([]syncclient.Note, 0, len(batch))}
		for _, n := range batch {
			req.Changes = append(req.Changes, encodeNote(n, e2e))
		}
		resp, err := c.Sync(ctx, req)
		if err != nil {
			return err
		}
		for _, n := range batch {
			s.store.MarkClean(n.ID, n.UpdatedAt)
		}
		for _, rn := range resp.Changes {
			r, err := decodeNote(rn, e2e)
			if err != nil {
				slog.Warn("skipping undecodable note", "id", rn.ID, "error", err)
				continue
			}
			changed, created, err := s.store.ApplyRemote(r)
			if err != nil {
				return err
			}
			if changed {
				prev := results[r.ID]
				results[r.ID] = result{true, prev.created || created}
			}
		}
		cursor = resp.Cursor
		if err := s.store.SetIntSetting(keyCursor, cursor); err != nil {
			return err
		}
		if !resp.More && len(dirty) == 0 {
			break
		}
	}

	for id, r := range results {
		n, err := s.store.Get(id)
		if err != nil || s.windows == nil {
			continue
		}
		switch {
		case n.Deleted:
			s.windows.Close(id)
		case r.created && !n.Hidden:
			s.windows.Open(n)
		default:
			s.emit(EventNoteChanged, n)
		}
	}
	if len(results) > 0 {
		s.emit(EventListChanged)
	}
	return nil
}

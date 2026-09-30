// Package pogosync syncs a local store with a Pogo Pad server: settings,
// the sync loop, last-write-wins merging and optional end-to-end encryption.
// It has no UI dependencies and is shared by the desktop and mobile apps.
package pogosync

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/dvher/pogo/pkg/palette"
	"github.com/dvher/pogo/pkg/secure"
	"github.com/dvher/pogo/pkg/store"
	"github.com/dvher/pogo/pkg/syncclient"
)

// Settings keys.
const (
	keyScheme   = "server_scheme"
	keyHost     = "server_host"
	keyPort     = "server_port"
	keyToken    = "server_token" // secret
	keyEnabled  = "sync_enabled"
	keyInterval = "sync_interval"
	keyCursor   = "sync_cursor"
	keyE2EKey   = "e2e_key" // secret, hex
	keyDeviceID = "device_id"

	uploadBatch = 1000
	nudgeDelay  = 2 * time.Second

	// MinInterval and DefaultInterval are sync periods in seconds.
	MinInterval     = 10
	DefaultInterval = 30
)

var (
	// ErrLocked means the server uses end-to-end encryption and this device
	// has no valid key; nothing is uploaded until the passphrase is entered.
	ErrLocked = errors.New("notes on this server are end-to-end encrypted: enter the passphrase in Sync settings")
	// ErrDisabled means sync is off or no server is configured.
	ErrDisabled = errors.New("sync is disabled")
)

// DeviceID returns this install's device id, creating it on first use.
func DeviceID(st *store.Store) string {
	id := st.Setting(keyDeviceID, "")
	if id == "" {
		id = uuid.NewString()
		st.SetSetting(keyDeviceID, id)
	}
	return id
}

// Status describes the last sync and the end-to-end state.
type Status struct {
	Enabled    bool   `json:"enabled"`
	Syncing    bool   `json:"syncing"`
	LastSync   int64  `json:"lastSync"` // unix ms of last success
	LastError  string `json:"lastError"`
	E2EServer  bool   `json:"e2eServer"`  // server has E2E set up
	E2ELocked  bool   `json:"e2eLocked"`  // ...but this device has no valid key
	E2EEnabled bool   `json:"e2eEnabled"` // this device encrypts uploads
}

// Applied is a note that a sync changed locally.
type Applied struct {
	ID      string
	Created bool // the note did not exist locally before
}

// Engine syncs one store. Set the callbacks before the first sync.
type Engine struct {
	store    *store.Store
	deviceID string

	// OnStatus is called whenever Status changes.
	OnStatus func(Status)
	// OnApplied is called after a sync that changed local notes.
	OnApplied func([]Applied)

	runMu  sync.Mutex // serialises sync runs
	mu     sync.Mutex
	status Status
	nudge  *time.Timer
	kick   chan struct{}
}

func New(st *store.Store, deviceID string) *Engine {
	return &Engine{store: st, deviceID: deviceID, kick: make(chan struct{}, 1)}
}

// DeviceID returns the id this engine stamps on edits.
func (e *Engine) DeviceID() string { return e.deviceID }

// Status returns the current sync status.
func (e *Engine) Status() Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.status
	st.Enabled = e.store.Setting(keyEnabled, "0") == "1"
	st.E2EEnabled = e.store.SecretSetting(keyE2EKey) != ""
	return st
}

// SyncNow runs a sync immediately and returns its error, if any.
func (e *Engine) SyncNow() error { return e.run(context.Background()) }

// Nudge schedules a sync in Loop shortly after a local edit.
func (e *Engine) Nudge() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.nudge != nil {
		e.nudge.Stop()
	}
	e.nudge = time.AfterFunc(nudgeDelay, func() {
		select {
		case e.kick <- struct{}{}:
		default:
		}
	})
}

func (e *Engine) interval() time.Duration {
	sec := max(e.store.IntSetting(keyInterval, DefaultInterval), MinInterval)
	return time.Duration(sec) * time.Second
}

// Loop syncs periodically and after nudges until ctx is cancelled.
func (e *Engine) Loop(ctx context.Context) {
	timer := time.NewTimer(time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-e.kick:
		}
		if err := e.run(ctx); err != nil && !errors.Is(err, ErrDisabled) {
			slog.Warn("sync failed", "error", err)
		}
		timer.Reset(e.interval())
	}
}

func (e *Engine) client() (*syncclient.Client, error) {
	base, err := syncclient.BaseURL(e.store.Setting(keyScheme, "http"), e.store.Setting(keyHost, ""), e.store.Setting(keyPort, ""))
	if err != nil {
		return nil, err
	}
	return syncclient.New(base, e.store.SecretSetting(keyToken)), nil
}

func (e *Engine) setStatus(f func(*Status)) {
	e.mu.Lock()
	f(&e.status)
	e.mu.Unlock()
	if e.OnStatus != nil {
		e.OnStatus(e.Status())
	}
}

func (e *Engine) run(ctx context.Context) error {
	if e.store.Setting(keyEnabled, "0") != "1" || e.store.Setting(keyHost, "") == "" {
		return ErrDisabled
	}
	e.runMu.Lock()
	defer e.runMu.Unlock()

	e.setStatus(func(st *Status) { st.Syncing = true })
	err := e.exchange(ctx)
	e.setStatus(func(st *Status) {
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
func (e *Engine) e2eCipher(ctx context.Context, c *syncclient.Client) (*secure.Cipher, error) {
	params, err := c.GetE2E(ctx)
	if errors.Is(err, syncclient.ErrNotFound) {
		e.setStatus(func(st *Status) { st.E2EServer, st.E2ELocked = false, false })
		if e.store.SecretSetting(keyE2EKey) != "" {
			// Turned off from another device.
			e.store.SetSecretSetting(keyE2EKey, "")
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cipher := e.localE2ECipher()
	if cipher == nil || !cipher.VerifyCheck(params.Check) {
		e.store.SetSecretSetting(keyE2EKey, "")
		e.setStatus(func(st *Status) { st.E2EServer, st.E2ELocked = true, true })
		return nil, ErrLocked
	}
	e.setStatus(func(st *Status) { st.E2EServer, st.E2ELocked = true, false })
	return cipher, nil
}

func (e *Engine) localE2ECipher() *secure.Cipher {
	key, err := hex.DecodeString(e.store.SecretSetting(keyE2EKey))
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
			return r, ErrLocked
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
	if !palette.Valid(r.Color) {
		r.Color = palette.Default
	}
	return r, nil
}

func (e *Engine) exchange(ctx context.Context) error {
	c, err := e.client()
	if err != nil {
		return err
	}
	e2e, err := e.e2eCipher(ctx, c)
	if err != nil {
		return err
	}

	dirty, err := e.store.Dirty()
	if err != nil {
		return err
	}
	cursor := e.store.IntSetting(keyCursor, 0)
	created := map[string]bool{}
	var order []string

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
			e.store.MarkClean(n.ID, n.UpdatedAt)
		}
		for _, rn := range resp.Changes {
			r, err := decodeNote(rn, e2e)
			if err != nil {
				slog.Warn("skipping undecodable note", "id", rn.ID, "error", err)
				continue
			}
			changed, isNew, err := e.store.ApplyRemote(r)
			if err != nil {
				return err
			}
			if changed {
				if _, seen := created[r.ID]; !seen {
					order = append(order, r.ID)
				}
				created[r.ID] = created[r.ID] || isNew
			}
		}
		cursor = resp.Cursor
		if err := e.store.SetIntSetting(keyCursor, cursor); err != nil {
			return err
		}
		if !resp.More && len(dirty) == 0 {
			break
		}
	}

	if len(order) > 0 && e.OnApplied != nil {
		applied := make([]Applied, len(order))
		for i, id := range order {
			applied[i] = Applied{ID: id, Created: created[id]}
		}
		e.OnApplied(applied)
	}
	return nil
}

// Package store keeps notes and settings in a local SQLite database.
// Note content and secret settings are encrypted at rest.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/dvher/pogo/pkg/secure"
)

var ErrNotFound = errors.New("note not found")

// Note is a note plus its device-local window state.
type Note struct {
	ID        string `json:"id"`
	Content   string `json:"content"`
	Color     string `json:"color"`
	Deleted   bool   `json:"deleted"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"` // unix ms of the last synced-field edit
	DeviceID  string `json:"-"`
	Dirty     bool   `json:"-"`

	// Local only, never synced.
	X        int  `json:"x"`
	Y        int  `json:"y"`
	W        int  `json:"w"`
	H        int  `json:"h"`
	Placed   bool `json:"-"` // X/Y hold a real saved position
	Anchored bool `json:"anchored"`
	Hidden   bool `json:"hidden"`
}

const schema = `
CREATE TABLE IF NOT EXISTS notes (
	id         TEXT PRIMARY KEY,
	content    TEXT    NOT NULL, -- encrypted
	color      TEXT    NOT NULL DEFAULT 'yellow',
	deleted    INTEGER NOT NULL DEFAULT 0,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	device_id  TEXT    NOT NULL DEFAULT '',
	dirty      INTEGER NOT NULL DEFAULT 1,
	x INTEGER NOT NULL DEFAULT 0,
	y INTEGER NOT NULL DEFAULT 0,
	w INTEGER NOT NULL DEFAULT 260,
	h INTEGER NOT NULL DEFAULT 260,
	placed   INTEGER NOT NULL DEFAULT 0,
	anchored INTEGER NOT NULL DEFAULT 0,
	hidden   INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS settings (
	key   TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

type Store struct {
	db     *sql.DB
	cipher *secure.Cipher
}

// DefaultDataDir returns $POGO_DATA_DIR or the per-user config directory.
func DefaultDataDir() (string, error) {
	if d := os.Getenv("POGO_DATA_DIR"); d != "" {
		return filepath.Abs(d)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "pogo"), nil
}

// Open opens the database in dir, encrypting with c.
func Open(dir string, c *secure.Cipher) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open(driverName, dsn(filepath.Join(dir, "notes.db"))+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db, cipher: c}, nil
}

func (s *Store) Close() error { return s.db.Close() }

const cols = `id, content, color, deleted, created_at, updated_at, device_id, dirty, x, y, w, h, placed, anchored, hidden`

type scanner interface{ Scan(...any) error }

func (s *Store) scan(r scanner) (Note, error) {
	var n Note
	var enc string
	err := r.Scan(&n.ID, &enc, &n.Color, &n.Deleted, &n.CreatedAt, &n.UpdatedAt, &n.DeviceID, &n.Dirty,
		&n.X, &n.Y, &n.W, &n.H, &n.Placed, &n.Anchored, &n.Hidden)
	if err != nil {
		return n, err
	}
	plain, err := s.cipher.Open(enc)
	if err != nil {
		return n, fmt.Errorf("note %s: %w", n.ID, err)
	}
	n.Content = string(plain)
	return n, nil
}

// List returns all notes that are not deleted, oldest first.
func (s *Store) List() ([]Note, error) {
	return s.query(`SELECT ` + cols + ` FROM notes WHERE deleted = 0 ORDER BY created_at`)
}

// Empty reports whether the database has never held a note (deleted ones count).
func (s *Store) Empty() bool {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&n)
	return n == 0
}

// Dirty returns notes (including deletions) with unsynced edits.
func (s *Store) Dirty() ([]Note, error) {
	return s.query(`SELECT ` + cols + ` FROM notes WHERE dirty = 1`)
}

func (s *Store) query(q string, args ...any) ([]Note, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		n, err := s.scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Get returns a note by id, including deleted ones.
func (s *Store) Get(id string) (Note, error) {
	n, err := s.scan(s.db.QueryRow(`SELECT `+cols+` FROM notes WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return n, ErrNotFound
	}
	return n, err
}

// Insert adds a new note. It is marked dirty so it gets uploaded.
func (s *Store) Insert(n Note) error {
	_, err := s.db.Exec(`INSERT INTO notes (`+cols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		n.ID, s.cipher.Seal([]byte(n.Content)), n.Color, n.Deleted, n.CreatedAt, n.UpdatedAt, n.DeviceID, true,
		n.X, n.Y, n.W, n.H, n.Placed, n.Anchored, n.Hidden)
	return err
}

// Edit changes the synced fields of a note: nil pointers leave a field as is.
// It bumps updated_at (monotonically) and marks the note dirty.
func (s *Store) Edit(id, deviceID string, content, color *string, deleted bool) (Note, error) {
	n, err := s.Get(id)
	if err != nil {
		return n, err
	}
	if content != nil {
		n.Content = *content
	}
	if color != nil {
		n.Color = *color
	}
	if deleted {
		n.Deleted, n.Content = true, ""
	}
	n.UpdatedAt = max(time.Now().UnixMilli(), n.UpdatedAt+1)
	n.DeviceID = deviceID
	n.Dirty = true
	_, err = s.db.Exec(`UPDATE notes SET content=?, color=?, deleted=?, updated_at=?, device_id=?, dirty=1 WHERE id=?`,
		s.cipher.Seal([]byte(n.Content)), n.Color, n.Deleted, n.UpdatedAt, n.DeviceID, id)
	return n, err
}

// SetGeometry stores a note's window position and size.
func (s *Store) SetGeometry(id string, x, y, w, h int) error {
	_, err := s.db.Exec(`UPDATE notes SET x=?, y=?, w=?, h=?, placed=1 WHERE id=?`, x, y, w, h, id)
	return err
}

// SetPosition stores only a note's window position.
func (s *Store) SetPosition(id string, x, y int) error {
	_, err := s.db.Exec(`UPDATE notes SET x=?, y=?, placed=1 WHERE id=?`, x, y, id)
	return err
}

// SetSize stores only a note's window size.
func (s *Store) SetSize(id string, w, h int) error {
	_, err := s.db.Exec(`UPDATE notes SET w=?, h=? WHERE id=?`, w, h, id)
	return err
}

func (s *Store) SetAnchored(id string, v bool) error {
	_, err := s.db.Exec(`UPDATE notes SET anchored=? WHERE id=?`, v, id)
	return err
}

func (s *Store) SetHidden(id string, v bool) error {
	_, err := s.db.Exec(`UPDATE notes SET hidden=? WHERE id=?`, v, id)
	return err
}

// MarkClean clears the dirty flag if the note was not edited again since
// the version with updatedAt was uploaded.
func (s *Store) MarkClean(id string, updatedAt int64) error {
	_, err := s.db.Exec(`UPDATE notes SET dirty=0 WHERE id=? AND updated_at=?`, id, updatedAt)
	return err
}

// MarkAllDirty queues every note for re-upload with a bumped timestamp, so the
// server accepts it even though it already holds this version (used when the
// server or the end-to-end key changes).
func (s *Store) MarkAllDirty() error {
	_, err := s.db.Exec(`UPDATE notes SET dirty=1, updated_at=updated_at+1`)
	return err
}

// Remote is a note received from the sync server, already decrypted.
type Remote struct {
	ID        string
	Content   string
	Color     string
	Deleted   bool
	UpdatedAt int64
	DeviceID  string
}

// ApplyRemote stores r if it wins last-write-wins against the local copy.
// It reports whether anything changed and whether the note is new locally.
func (s *Store) ApplyRemote(r Remote) (changed, created bool, err error) {
	local, err := s.Get(r.ID)
	switch {
	case errors.Is(err, ErrNotFound):
		if r.Deleted {
			return false, false, nil
		}
		err = s.Insert(Note{
			ID: r.ID, Content: r.Content, Color: r.Color, CreatedAt: r.UpdatedAt,
			UpdatedAt: r.UpdatedAt, DeviceID: r.DeviceID, W: 260, H: 260,
		})
		if err == nil {
			err = s.MarkClean(r.ID, r.UpdatedAt)
		}
		return err == nil, true, err
	case err != nil:
		return false, false, err
	}
	if local.UpdatedAt > r.UpdatedAt || (local.UpdatedAt == r.UpdatedAt && local.DeviceID >= r.DeviceID) {
		return false, false, nil
	}
	_, err = s.db.Exec(`UPDATE notes SET content=?, color=?, deleted=?, updated_at=?, device_id=?, dirty=0 WHERE id=?`,
		s.cipher.Seal([]byte(r.Content)), r.Color, r.Deleted, r.UpdatedAt, r.DeviceID, r.ID)
	return err == nil, false, err
}

// Setting returns a plain setting, or def if unset.
func (s *Store) Setting(key, def string) string {
	var v string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return def
	}
	return v
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) IntSetting(key string, def int64) int64 {
	v, err := strconv.ParseInt(s.Setting(key, ""), 10, 64)
	if err != nil {
		return def
	}
	return v
}

func (s *Store) SetIntSetting(key string, v int64) error {
	return s.SetSetting(key, strconv.FormatInt(v, 10))
}

// SecretSetting returns an encrypted setting, or "" if unset.
func (s *Store) SecretSetting(key string) string {
	v := s.Setting(key, "")
	if v == "" {
		return ""
	}
	plain, err := s.cipher.Open(v)
	if err != nil {
		return ""
	}
	return string(plain)
}

func (s *Store) SetSecretSetting(key, value string) error {
	if value == "" {
		return s.SetSetting(key, "")
	}
	return s.SetSetting(key, s.cipher.Seal([]byte(value)))
}

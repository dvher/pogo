package main

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"notes-desktop/internal/secure"
	"notes-desktop/internal/store"
	"notes-desktop/internal/syncclient"
)

// Settings is the Sync tab's form.
type Settings struct {
	Scheme      string `json:"scheme"` // http or https
	Host        string `json:"host"`   // IP or hostname
	Port        string `json:"port"`
	Token       string `json:"token"`
	Enabled     bool   `json:"enabled"`
	IntervalSec int    `json:"intervalSec"`
}

// SettingsService manages sync settings and end-to-end encryption.
type SettingsService struct {
	store *store.Store
	sync  *SyncService
}

func (s *SettingsService) Get() Settings {
	return Settings{
		Scheme:      s.store.Setting(keyScheme, "http"),
		Host:        s.store.Setting(keyHost, ""),
		Port:        s.store.Setting(keyPort, "8080"),
		Token:       s.store.SecretSetting(keyToken),
		Enabled:     s.store.Setting(keyEnabled, "0") == "1",
		IntervalSec: int(s.store.IntSetting(keyInterval, defaultPeriod)),
	}
}

func normalize(in Settings) (Settings, error) {
	in.Host = strings.TrimSpace(in.Host)
	in.Port = strings.TrimSpace(in.Port)
	in.Token = strings.TrimSpace(in.Token)
	if in.Scheme != "https" {
		in.Scheme = "http"
	}
	if in.Port != "" {
		if p, err := strconv.Atoi(in.Port); err != nil || p < 1 || p > 65535 {
			return in, errors.New("port must be a number between 1 and 65535")
		}
	}
	if in.IntervalSec < minInterval {
		in.IntervalSec = minInterval
	}
	if in.Enabled {
		if in.Host == "" {
			return in, errors.New("enter the server's IP address or hostname")
		}
		if in.Token == "" {
			return in, errors.New("enter the API token (run `notes-server token create --name <device>` on the server)")
		}
	}
	return in, nil
}

// Save stores settings. Pointing at a different server resets the sync
// cursor and queues every note for upload.
func (s *SettingsService) Save(in Settings) (Settings, error) {
	in, err := normalize(in)
	if err != nil {
		return in, err
	}
	old := s.Get()
	serverChanged := old.Scheme != in.Scheme || old.Host != in.Host || old.Port != in.Port

	s.store.SetSetting(keyScheme, in.Scheme)
	s.store.SetSetting(keyHost, in.Host)
	s.store.SetSetting(keyPort, in.Port)
	s.store.SetSecretSetting(keyToken, in.Token)
	s.store.SetSetting(keyEnabled, map[bool]string{true: "1", false: "0"}[in.Enabled])
	s.store.SetIntSetting(keyInterval, int64(in.IntervalSec))
	if serverChanged {
		s.store.SetIntSetting(keyCursor, 0)
		s.store.SetSecretSetting(keyE2EKey, "")
		s.store.MarkAllDirty()
	}
	s.sync.setStatus(func(st *SyncStatus) { st.LastError = "" })
	s.sync.Nudge()
	return in, nil
}

// TestConnection checks the given (unsaved) settings against the server.
func (s *SettingsService) TestConnection(in Settings) (string, error) {
	in, err := normalize(in)
	if err != nil {
		return "", err
	}
	base, err := syncclient.BaseURL(in.Scheme, in.Host, in.Port)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c := syncclient.New(base, in.Token)
	version, err := c.Health(ctx)
	if err != nil {
		return "", err
	}
	if in.Token == "" {
		return fmt.Sprintf("Reached notes-server %s at %s. Add a token to sync.", version, base), nil
	}
	_, err = c.GetE2E(ctx)
	switch {
	case errors.Is(err, syncclient.ErrNotFound):
		return fmt.Sprintf("Connected to notes-server %s. Token OK. End-to-end encryption is off.", version), nil
	case err != nil:
		return "", err
	}
	return fmt.Sprintf("Connected to notes-server %s. Token OK. Notes on this server are end-to-end encrypted.", version), nil
}

// EnableE2E turns on end-to-end encryption with passphrase. If the server
// already uses E2E the passphrase must match; otherwise E2E is set up on the
// server and all notes are re-uploaded encrypted.
func (s *SettingsService) EnableE2E(passphrase string) error {
	if len(passphrase) < 8 {
		return errors.New("use a passphrase of at least 8 characters")
	}
	c, err := s.sync.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	params, err := c.GetE2E(ctx)
	switch {
	case err == nil:
		return s.unlock(params, passphrase)
	case !errors.Is(err, syncclient.ErrNotFound):
		return err
	}

	// Pull everything first so re-uploading does not clobber newer remote edits.
	if err := s.sync.SyncNow(); err != nil {
		return fmt.Errorf("sync before enabling encryption failed: %w", err)
	}
	salt := secure.NewSalt()
	key := secure.DeriveE2EKey(passphrase, salt)
	cipher, _ := secure.NewCipher(key)
	params = syncclient.E2EParams{KDF: secure.E2EKDF, Salt: base64.RawStdEncoding.EncodeToString(salt), Check: cipher.MakeCheck()}
	if err := c.PutE2E(ctx, params, false); errors.Is(err, syncclient.ErrConflict) {
		// Another device enabled it meanwhile.
		if params, err = c.GetE2E(ctx); err != nil {
			return err
		}
		return s.unlock(params, passphrase)
	} else if err != nil {
		return err
	}
	s.store.SetSecretSetting(keyE2EKey, hex.EncodeToString(key))
	s.store.MarkAllDirty()
	return s.sync.SyncNow()
}

func (s *SettingsService) unlock(params syncclient.E2EParams, passphrase string) error {
	salt, err := base64.RawStdEncoding.DecodeString(params.Salt)
	if err != nil {
		return errors.New("server has malformed encryption settings")
	}
	key := secure.DeriveE2EKey(passphrase, salt)
	cipher, _ := secure.NewCipher(key)
	if !cipher.VerifyCheck(params.Check) {
		return errors.New("wrong passphrase")
	}
	s.store.SetSecretSetting(keyE2EKey, hex.EncodeToString(key))
	return s.sync.SyncNow()
}

// DisableE2E turns off end-to-end encryption for every device on the server
// and re-uploads notes in plaintext.
func (s *SettingsService) DisableE2E() error {
	if s.sync.localE2ECipher() == nil {
		return errors.New("unlock end-to-end encryption on this device first")
	}
	if err := s.sync.SyncNow(); err != nil {
		return fmt.Errorf("sync before disabling encryption failed: %w", err)
	}
	c, err := s.sync.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.DeleteE2E(ctx); err != nil {
		return err
	}
	s.store.SetSecretSetting(keyE2EKey, "")
	s.store.MarkAllDirty()
	return s.sync.SyncNow()
}

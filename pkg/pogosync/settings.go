package pogosync

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dvher/pogo/pkg/secure"
	"github.com/dvher/pogo/pkg/syncclient"
)

// Settings is the sync settings form.
type Settings struct {
	Scheme      string `json:"scheme"` // http or https
	Host        string `json:"host"`   // IP or hostname
	Port        string `json:"port"`
	Token       string `json:"token"`
	Enabled     bool   `json:"enabled"`
	IntervalSec int    `json:"intervalSec"`
}

// Settings returns the saved sync settings.
func (e *Engine) Settings() Settings {
	return Settings{
		Scheme:      e.store.Setting(keyScheme, "http"),
		Host:        e.store.Setting(keyHost, ""),
		Port:        e.store.Setting(keyPort, "8080"),
		Token:       e.store.SecretSetting(keyToken),
		Enabled:     e.store.Setting(keyEnabled, "0") == "1",
		IntervalSec: int(e.store.IntSetting(keyInterval, DefaultInterval)),
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
	if in.IntervalSec < MinInterval {
		in.IntervalSec = MinInterval
	}
	if in.Enabled {
		if in.Host == "" {
			return in, errors.New("enter the server's IP address or hostname")
		}
		if in.Token == "" {
			return in, errors.New("enter the API token (run `pogo-pad token create --name <device>` on the server)")
		}
	}
	return in, nil
}

// SaveSettings stores settings. Pointing at a different server resets the
// sync cursor and queues every note for upload.
func (e *Engine) SaveSettings(in Settings) (Settings, error) {
	in, err := normalize(in)
	if err != nil {
		return in, err
	}
	old := e.Settings()
	serverChanged := old.Scheme != in.Scheme || old.Host != in.Host || old.Port != in.Port

	e.store.SetSetting(keyScheme, in.Scheme)
	e.store.SetSetting(keyHost, in.Host)
	e.store.SetSetting(keyPort, in.Port)
	e.store.SetSecretSetting(keyToken, in.Token)
	e.store.SetSetting(keyEnabled, map[bool]string{true: "1", false: "0"}[in.Enabled])
	e.store.SetIntSetting(keyInterval, int64(in.IntervalSec))
	if serverChanged {
		e.store.SetIntSetting(keyCursor, 0)
		e.store.SetSecretSetting(keyE2EKey, "")
		e.store.MarkAllDirty()
	}
	e.setStatus(func(st *Status) { st.LastError = "" })
	e.Nudge()
	return in, nil
}

// TestConnection checks the given (unsaved) settings against the server.
func (e *Engine) TestConnection(in Settings) (string, error) {
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
		return fmt.Sprintf("Reached Pogo Pad %s at %s. Add a token to sync.", version, base), nil
	}
	_, err = c.GetE2E(ctx)
	switch {
	case errors.Is(err, syncclient.ErrNotFound):
		return fmt.Sprintf("Connected to Pogo Pad %s. Token OK. End-to-end encryption is off.", version), nil
	case err != nil:
		return "", err
	}
	return fmt.Sprintf("Connected to Pogo Pad %s. Token OK. Notes on this server are end-to-end encrypted.", version), nil
}

// EnableE2E turns on end-to-end encryption with passphrase. If the server
// already uses E2E the passphrase must match; otherwise E2E is set up on the
// server and all notes are re-uploaded encrypted.
func (e *Engine) EnableE2E(passphrase string) error {
	if len(passphrase) < 8 {
		return errors.New("use a passphrase of at least 8 characters")
	}
	c, err := e.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	params, err := c.GetE2E(ctx)
	switch {
	case err == nil:
		return e.unlock(params, passphrase)
	case !errors.Is(err, syncclient.ErrNotFound):
		return err
	}

	// Pull everything first so re-uploading does not clobber newer remote edits.
	if err := e.SyncNow(); err != nil {
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
		return e.unlock(params, passphrase)
	} else if err != nil {
		return err
	}
	e.store.SetSecretSetting(keyE2EKey, hex.EncodeToString(key))
	e.store.MarkAllDirty()
	return e.SyncNow()
}

func (e *Engine) unlock(params syncclient.E2EParams, passphrase string) error {
	salt, err := base64.RawStdEncoding.DecodeString(params.Salt)
	if err != nil {
		return errors.New("server has malformed encryption settings")
	}
	key := secure.DeriveE2EKey(passphrase, salt)
	cipher, _ := secure.NewCipher(key)
	if !cipher.VerifyCheck(params.Check) {
		return errors.New("wrong passphrase")
	}
	e.store.SetSecretSetting(keyE2EKey, hex.EncodeToString(key))
	return e.SyncNow()
}

// DisableE2E turns off end-to-end encryption for every device on the server
// and re-uploads notes in plaintext.
func (e *Engine) DisableE2E() error {
	if e.localE2ECipher() == nil {
		return errors.New("unlock end-to-end encryption on this device first")
	}
	if err := e.SyncNow(); err != nil {
		return fmt.Errorf("sync before disabling encryption failed: %w", err)
	}
	c, err := e.client()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := c.DeleteE2E(ctx); err != nil {
		return err
	}
	e.store.SetSecretSetting(keyE2EKey, "")
	e.store.MarkAllDirty()
	return e.SyncNow()
}

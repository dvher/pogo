// Package localkey keeps the at-rest database key in the OS keyring.
package localkey

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"

	"github.com/dvher/pogo/pkg/secure"
)

const keyringService = "pogo"

// Get loads the at-rest key for dataDir from the OS keyring, creating it
// on first run. If no keyring is available it falls back to a 0600 key file in
// dataDir, which still keeps the database unreadable on its own (e.g. in
// backups) but not against someone with access to the whole home directory.
func Get(dataDir string) ([]byte, error) {
	sum := sha256.Sum256([]byte(dataDir))
	account := "local-key-" + hex.EncodeToString(sum[:8])
	keyFile := filepath.Join(dataDir, "local.key")

	if v, err := keyring.Get(keyringService, account); err == nil {
		return decodeKey(v)
	} else if !errors.Is(err, keyring.ErrNotFound) {
		slog.Warn("OS keyring unavailable, using key file", "error", err)
		return fileKey(keyFile)
	}

	// Migrate a key file left from a run without a keyring, else create a key.
	var key []byte
	if b, err := os.ReadFile(keyFile); err == nil {
		if key, err = decodeKey(string(b)); err != nil {
			return nil, err
		}
	} else {
		key = secure.NewKey()
	}
	if err := keyring.Set(keyringService, account, hex.EncodeToString(key)); err != nil {
		slog.Warn("could not store key in OS keyring, using key file", "error", err)
		return fileKey(keyFile)
	}
	os.Remove(keyFile)
	return key, nil
}

func fileKey(path string) ([]byte, error) {
	if b, err := os.ReadFile(path); err == nil {
		return decodeKey(string(b))
	}
	key := secure.NewKey()
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func decodeKey(s string) ([]byte, error) {
	key, err := hex.DecodeString(s)
	if err != nil || len(key) != 32 {
		return nil, errors.New("stored encryption key is malformed")
	}
	return key, nil
}

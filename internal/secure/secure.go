// Package secure handles encryption: the local at-rest key kept in the OS
// keyring, and the optional end-to-end sync key derived from a passphrase.
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/argon2"
)

const keyringService = "notes-desktop"

// ErrDecrypt is returned when ciphertext cannot be decrypted with the key.
var ErrDecrypt = errors.New("decryption failed (wrong key or corrupted data)")

// Cipher encrypts and decrypts with AES-256-GCM.
type Cipher struct {
	aead cipher.AEAD
}

// NewCipher returns a Cipher for a 32-byte key.
func NewCipher(key []byte) (*Cipher, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("key must be 32 bytes, got %d", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Seal encrypts plaintext and returns base64(nonce || ciphertext).
func (c *Cipher) Seal(plaintext []byte) string {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	return base64.RawStdEncoding.EncodeToString(c.aead.Seal(nonce, nonce, plaintext, nil))
}

// Open reverses Seal.
func (c *Cipher) Open(sealed string) ([]byte, error) {
	raw, err := base64.RawStdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < c.aead.NonceSize() {
		return nil, ErrDecrypt
	}
	n := c.aead.NonceSize()
	out, err := c.aead.Open(nil, raw[:n], raw[n:], nil)
	if err != nil {
		return nil, ErrDecrypt
	}
	return out, nil
}

// LocalKey loads the at-rest key for dataDir from the OS keyring, creating it
// on first run. If no keyring is available it falls back to a 0600 key file in
// dataDir, which still keeps the database unreadable on its own (e.g. in
// backups) but not against someone with access to the whole home directory.
func LocalKey(dataDir string) ([]byte, error) {
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
		key = randomKey()
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
	key := randomKey()
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func randomKey() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	return key
}

func decodeKey(s string) ([]byte, error) {
	key, err := hex.DecodeString(s)
	if err != nil || len(key) != 32 {
		return nil, errors.New("stored encryption key is malformed")
	}
	return key, nil
}

// E2E parameters, shared with the server's /api/v1/e2e endpoint.
const (
	E2EPrefix   = "e2e:v1:"
	E2EKDF      = "argon2id"
	e2eCheckMsg = "notes-e2e-check"
)

// DeriveE2EKey derives the end-to-end key from a passphrase and salt.
func DeriveE2EKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, 3, 64*1024, 4, 32)
}

// NewSalt returns a random 16-byte salt.
func NewSalt() []byte {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		panic(err)
	}
	return salt
}

// MakeCheck returns the value stored on the server to verify passphrases.
func (c *Cipher) MakeCheck() string { return c.Seal([]byte(e2eCheckMsg)) }

// VerifyCheck reports whether check was produced by the same key.
func (c *Cipher) VerifyCheck(check string) bool {
	b, err := c.Open(check)
	return err == nil && string(b) == e2eCheckMsg
}

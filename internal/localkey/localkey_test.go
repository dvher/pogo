package localkey

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeyringKeyIsStable(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	k1, err := Get(dir)
	if err != nil || len(k1) != 32 {
		t.Fatalf("first key: %x %v", k1, err)
	}
	k2, err := Get(dir)
	if err != nil || !bytes.Equal(k1, k2) {
		t.Fatalf("key changed between runs: %x %x %v", k1, k2, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "local.key")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("key file written although the keyring works")
	}
	// Another data directory gets another key.
	other, _ := Get(t.TempDir())
	if bytes.Equal(k1, other) {
		t.Fatal("two data directories share a key")
	}
}

func TestMigratesKeyFile(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	want := bytes.Repeat([]byte{0xab}, 32)
	keyFile := filepath.Join(dir, "local.key")
	os.WriteFile(keyFile, []byte(hex.EncodeToString(want)), 0o600)

	got, err := Get(dir)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("migrated key: %x %v", got, err)
	}
	if _, err := os.Stat(keyFile); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("key file kept after moving it to the keyring")
	}
	if again, _ := Get(dir); !bytes.Equal(again, want) {
		t.Fatal("key not kept in the keyring")
	}
}

func TestFallsBackToKeyFile(t *testing.T) {
	keyring.MockInitWithError(errors.New("no Secret Service"))
	dir := t.TempDir()
	k1, err := Get(dir)
	if err != nil || len(k1) != 32 {
		t.Fatalf("key: %x %v", k1, err)
	}
	info, err := os.Stat(filepath.Join(dir, "local.key"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key file mode %o", perm)
	}
	if k2, _ := Get(dir); !bytes.Equal(k1, k2) {
		t.Fatal("key file not reused")
	}
}

func TestMalformedKey(t *testing.T) {
	keyring.MockInitWithError(errors.New("no Secret Service"))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "local.key"), []byte("not hex"), 0o600)
	if _, err := Get(dir); err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("malformed key file: %v", err)
	}

	keyring.MockInit()
	if _, err := Get(dir); err == nil {
		t.Fatal("malformed key file migrated")
	}
	for _, bad := range []string{"zz", hex.EncodeToString([]byte("short"))} {
		if _, err := decodeKey(bad); err == nil {
			t.Errorf("decodeKey(%q) accepted", bad)
		}
	}
}

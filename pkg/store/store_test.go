package store

import (
	"crypto/rand"
	"strings"
	"testing"

	"github.com/dvher/pogo/pkg/secure"
)

func open(t *testing.T) *Store {
	t.Helper()
	key := make([]byte, 32)
	rand.Read(key)
	c, _ := secure.NewCipher(key)
	s, err := Open(t.TempDir(), c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestContentEncryptedAtRest(t *testing.T) {
	s := open(t)
	s.Insert(Note{ID: "n1", Content: "secret plans", Color: "yellow", UpdatedAt: 1})
	var raw string
	s.db.QueryRow(`SELECT content FROM notes WHERE id='n1'`).Scan(&raw)
	if strings.Contains(raw, "secret") {
		t.Fatalf("content stored in plaintext: %q", raw)
	}
	n, err := s.Get("n1")
	if err != nil || n.Content != "secret plans" {
		t.Fatalf("get: %+v %v", n, err)
	}

	s.SetSecretSetting("token", "pogo_abc")
	if s.Setting("token", "") == "pogo_abc" || s.SecretSetting("token") != "pogo_abc" {
		t.Fatal("secret setting not encrypted or not readable")
	}
}

func TestEditAndSyncState(t *testing.T) {
	s := open(t)
	s.Insert(Note{ID: "n1", Content: "a", UpdatedAt: 1})
	s.MarkClean("n1", 1)

	txt := "b"
	n, _ := s.Edit("n1", "dev", &txt, nil, false)
	if !n.Dirty || n.UpdatedAt <= 1 {
		t.Fatalf("edit: %+v", n)
	}
	// An upload of an older version must not clear a newer edit.
	s.MarkClean("n1", 1)
	if d, _ := s.Dirty(); len(d) != 1 {
		t.Fatal("stale MarkClean cleared dirty flag")
	}
	s.MarkClean("n1", n.UpdatedAt)
	if d, _ := s.Dirty(); len(d) != 0 {
		t.Fatal("MarkClean did not clear")
	}
}

func TestApplyRemote(t *testing.T) {
	s := open(t)
	s.Insert(Note{ID: "n1", Content: "local", UpdatedAt: 100, DeviceID: "b"})

	if changed, _, _ := s.ApplyRemote(Remote{ID: "n1", Content: "old", UpdatedAt: 50, DeviceID: "z"}); changed {
		t.Fatal("older remote applied")
	}
	if changed, _, _ := s.ApplyRemote(Remote{ID: "n1", Content: "new", UpdatedAt: 200, DeviceID: "a"}); !changed {
		t.Fatal("newer remote not applied")
	}
	if n, _ := s.Get("n1"); n.Content != "new" || n.Dirty {
		t.Fatalf("after apply: %+v", n)
	}
	if _, created, _ := s.ApplyRemote(Remote{ID: "n2", Content: "x", UpdatedAt: 1}); !created {
		t.Fatal("new remote note not created")
	}
	if changed, _, _ := s.ApplyRemote(Remote{ID: "n3", Deleted: true, UpdatedAt: 1}); changed {
		t.Fatal("unknown tombstone created a note")
	}
	var count int
	s.db.QueryRow(`SELECT COUNT(*) FROM notes WHERE id='n3'`).Scan(&count)
	if count != 0 {
		t.Fatal("tombstone row inserted")
	}
}

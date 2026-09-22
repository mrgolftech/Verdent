package apikey

import (
	"path/filepath"
	"testing"
)

func TestManagerCreateListValidDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keys.json")
	m, err := NewManager(FileStore{Path: path})
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if len(m.List()) != 0 {
		t.Fatalf("expected empty initial list")
	}

	k, err := m.Create("app-one")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if k.Value == "" || k.ID == "" {
		t.Fatalf("created key missing id/value: %#v", k)
	}
	if !m.Valid(k.Value) {
		t.Fatalf("created key should validate")
	}
	if m.Valid("nope") {
		t.Fatalf("unknown key must not validate")
	}
	if m.Valid("") {
		t.Fatalf("empty key must not validate")
	}

	// Persistence: a fresh manager on the same file sees the key.
	m2, err := NewManager(FileStore{Path: path})
	if err != nil {
		t.Fatalf("reload manager: %v", err)
	}
	list := m2.List()
	if len(list) != 1 || list[0].Value != k.Value {
		t.Fatalf("reloaded keys mismatch: %#v", list)
	}
	if !m2.Valid(k.Value) {
		t.Fatalf("reloaded key should validate")
	}

	if !m2.Delete(k.ID) {
		t.Fatalf("delete should succeed")
	}
	if m2.Valid(k.Value) {
		t.Fatalf("deleted key must not validate")
	}
	if m2.Delete(k.ID) {
		t.Fatalf("second delete should report false")
	}

	m3, err := NewManager(FileStore{Path: path})
	if err != nil {
		t.Fatalf("reload after delete: %v", err)
	}
	if len(m3.List()) != 0 {
		t.Fatalf("expected empty list after delete, got %d", len(m3.List()))
	}
}

func TestManagerMissingFileIsEmpty(t *testing.T) {
	m, err := NewManager(FileStore{Path: filepath.Join(t.TempDir(), "absent.json")})
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(m.List()) != 0 {
		t.Fatalf("expected empty list")
	}
}

func TestKeysAreUnique(t *testing.T) {
	m, _ := NewManager(FileStore{Path: filepath.Join(t.TempDir(), "keys.json")})
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		k, err := m.Create("k")
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		if seen[k.Value] {
			t.Fatalf("duplicate generated key: %s", k.Value)
		}
		seen[k.Value] = true
	}
}

// Package apikey manages downstream API keys used to authenticate /v1 requests.
//
// Keys are generated server-side and persisted to a JSON file (default
// data/keys.json). A legacy static key configured via VERDENT_API_KEY keeps
// working alongside managed keys.
package apikey

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Key is a single managed API key.
type Key struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Value     string `json:"key"`
	CreatedAt int64  `json:"created_at"`
}

type storedKeys struct {
	Keys []Key `json:"keys"`
}

// FileStore persists keys as a JSON document.
type FileStore struct {
	Path string
}

// Load reads the key file; a missing file yields an empty list.
func (s FileStore) Load() ([]Key, error) {
	if s.Path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read keys file: %w", err)
	}
	var wrapped storedKeys
	if err := json.Unmarshal(data, &wrapped); err != nil {
		var direct []Key
		if err2 := json.Unmarshal(data, &direct); err2 != nil {
			return nil, fmt.Errorf("decode keys file: %w", err)
		}
		wrapped.Keys = direct
	}
	out := make([]Key, 0, len(wrapped.Keys))
	for _, item := range wrapped.Keys {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		item.Value = strings.TrimSpace(item.Value)
		if item.ID == "" || item.Value == "" {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// Save atomically writes the key file with 0600 permissions.
func (s FileStore) Save(keys []Key) error {
	if s.Path == "" {
		return nil
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create keys directory: %w", err)
	}
	payload := storedKeys{Keys: make([]Key, 0, len(keys))}
	for _, k := range keys {
		if k.ID == "" || k.Value == "" {
			continue
		}
		payload.Keys = append(payload.Keys, k)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode keys: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".keys-*.tmp")
	if err != nil {
		return fmt.Errorf("create keys temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure keys temp file: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("write keys temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync keys temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close keys temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		return fmt.Errorf("replace keys file: %w", err)
	}
	return os.Chmod(s.Path, 0o600)
}

// Manager owns the in-memory key set and its backing store.
type Manager struct {
	mu    sync.RWMutex
	store FileStore
	keys  []Key
}

// NewManager loads persisted keys into a manager.
func NewManager(store FileStore) (*Manager, error) {
	keys, err := store.Load()
	if err != nil {
		return nil, err
	}
	return &Manager{store: store, keys: keys}, nil
}

// Path returns the backing store path.
func (m *Manager) Path() string {
	if m == nil {
		return ""
	}
	return m.store.Path
}

// List returns a copy of the managed keys.
func (m *Manager) List() []Key {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Key, len(m.keys))
	copy(out, m.keys)
	return out
}

// Create generates and persists a new key.
func (m *Manager) Create(name string) (Key, error) {
	value, err := generateKey()
	if err != nil {
		return Key{}, err
	}
	key := Key{ID: newID(), Name: strings.TrimSpace(name), Value: value, CreatedAt: time.Now().Unix()}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys = append(m.keys, key)
	if err := m.store.Save(m.keys); err != nil {
		m.keys = m.keys[:len(m.keys)-1]
		return Key{}, err
	}
	return key, nil
}

// Delete removes a key by ID. It reports whether a key was removed.
func (m *Manager) Delete(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	idx := -1
	for i := range m.keys {
		if m.keys[i].ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false
	}
	removed := m.keys[idx]
	m.keys = append(m.keys[:idx], m.keys[idx+1:]...)
	if err := m.store.Save(m.keys); err != nil {
		// roll back on persistence failure
		m.keys = append(m.keys, Key{})
		copy(m.keys[idx+1:], m.keys[idx:])
		m.keys[idx] = removed
		return false
	}
	return true
}

// Valid reports whether the presented value matches a managed key.
func (m *Manager) Valid(value string) bool {
	if m == nil || value == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.keys {
		if subtle.ConstantTimeCompare([]byte(m.keys[i].Value), []byte(value)) == 1 {
			return true
		}
	}
	return false
}

func generateKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return "vd-" + hex.EncodeToString(b), nil
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("vk-%d", time.Now().UnixNano())
	}
	return "vk-" + hex.EncodeToString(b)
}

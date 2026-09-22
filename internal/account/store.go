package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type FileStore struct {
	Path string
}

type storedCredential struct {
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
	Token    string `json:"token"`
	DeviceID string `json:"device_id"`
	TeamID   string `json:"team_id,omitempty"`
	ProxyURL string `json:"proxy_url,omitempty"`
}

type storedAccounts struct {
	Accounts []storedCredential `json:"accounts"`
}

func (s FileStore) Load() ([]Credential, error) {
	if s.Path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read accounts file: %w", err)
	}

	var wrapped storedAccounts
	if err := json.Unmarshal(data, &wrapped); err != nil {
		var direct []storedCredential
		if err2 := json.Unmarshal(data, &direct); err2 != nil {
			return nil, fmt.Errorf("decode accounts file: %w", err)
		}
		wrapped.Accounts = direct
	}

	out := make([]Credential, 0, len(wrapped.Accounts))
	for _, item := range wrapped.Accounts {
		if item.ID == "" || item.Token == "" {
			continue
		}
		out = append(out, Credential{
			ID: item.ID, Label: item.Label, Token: item.Token,
			DeviceID: item.DeviceID, TeamID: item.TeamID, ProxyURL: item.ProxyURL,
		})
	}
	return out, nil
}

func (s FileStore) Save(credentials []Credential) error {
	if s.Path == "" {
		return nil
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create account directory: %w", err)
	}
	payload := storedAccounts{Accounts: make([]storedCredential, 0, len(credentials))}
	for _, credential := range credentials {
		if credential.ID == "" || credential.Token == "" {
			continue
		}
		payload.Accounts = append(payload.Accounts, storedCredential{
			ID: credential.ID, Label: credential.Label, Token: credential.Token,
			DeviceID: credential.DeviceID, TeamID: credential.TeamID, ProxyURL: credential.ProxyURL,
		})
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode accounts: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".accounts-*.tmp")
	if err != nil {
		return fmt.Errorf("create accounts temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure accounts temp file: %w", err)
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return fmt.Errorf("write accounts temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync accounts temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close accounts temp file: %w", err)
	}
	if err := os.Rename(tmpPath, s.Path); err != nil {
		return fmt.Errorf("replace accounts file: %w", err)
	}
	return os.Chmod(s.Path, 0o600)
}

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// handleKeysList returns the managed keys plus the legacy static key (if any).
// This endpoint is admin-gated: it is the only place the full key values are
// exposed so the console can offer copy.
func (s *Server) handleKeysList(w http.ResponseWriter, r *http.Request) {
	items := []map[string]any{}
	if s.APIKey != "" {
		items = append(items, map[string]any{
			"id": "env", "name": "内置 (VERDENT_API_KEY)", "key": s.APIKey,
			"created_at": 0, "builtin": true,
		})
	}
	for _, k := range s.Keys.List() {
		items = append(items, map[string]any{
			"id": k.ID, "name": k.Name, "key": k.Value,
			"created_at": k.CreatedAt, "builtin": false,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"keys": items, "store_file": s.Keys.Path(),
	})
}

func (s *Server) handleKeysCreate(w http.ResponseWriter, r *http.Request) {
	if s.Keys == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "keystore is not configured"})
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid key request"})
		return
	}
	key, err := s.Keys.Create(input.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": map[string]any{
		"id": key.ID, "name": key.Name, "key": key.Value, "created_at": key.CreatedAt,
	}})
}

func (s *Server) handleKeysDelete(w http.ResponseWriter, r *http.Request) {
	if s.Keys == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "keystore is not configured"})
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing key id"})
		return
	}
	if !s.Keys.Delete(id) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "key not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

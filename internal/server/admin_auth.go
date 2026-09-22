package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const adminCookieName = "verdent_admin"

type adminSessions struct {
	mu       sync.Mutex
	sessions map[string]time.Time
	ttl      time.Duration
}

func newAdminSessions() *adminSessions {
	return &adminSessions{sessions: make(map[string]time.Time), ttl: 12 * time.Hour}
}

func (s *adminSessions) create() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	token := hex.EncodeToString(buf)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[token] = time.Now().Add(s.ttl)
	return token
}

func (s *adminSessions) valid(token string) bool {
	if token == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.sessions, token)
		return false
	}
	s.sessions[token] = time.Now().Add(s.ttl)
	return true
}

func (s *adminSessions) remove(token string) {
	if token == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, token)
}

func (s *Server) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if s.AdminPassword == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error": "VERDENT_ADMIN_PASSWORD or VERDENT_API_KEY must be configured before using the management console",
		})
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid login request"})
		return
	}
	userOK := subtle.ConstantTimeCompare([]byte(input.Username), []byte(s.AdminUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(input.Password), []byte(s.AdminPassword)) == 1
	if !userOK || !passOK {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "invalid username or password"})
		return
	}
	token := s.adminSessions.create()
	http.SetCookie(w, &http.Cookie{
		Name: adminCookieName, Value: token, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteStrictMode,
		Secure: r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		MaxAge: int((12 * time.Hour).Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "username": s.AdminUser})
}

func (s *Server) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(adminCookieName); err == nil {
		s.adminSessions.remove(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name: adminCookieName, Value: "", Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminSession(w http.ResponseWriter, r *http.Request) {
	if s.adminAuthorized(r) {
		writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "username": s.AdminUser})
		return
	}
	writeJSON(w, http.StatusUnauthorized, map[string]any{"authenticated": false})
}

func (s *Server) adminAuthorized(r *http.Request) bool {
	if s.adminSessions == nil {
		return false
	}
	cookie, err := r.Cookie(adminCookieName)
	if err != nil {
		return false
	}
	return s.adminSessions.valid(cookie.Value)
}

func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.adminAuthorized(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "admin session required"})
			return
		}
		next(w, r)
	}
}

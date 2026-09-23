package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mrgolftech/Verdent/internal/account"
	"github.com/mrgolftech/Verdent/internal/verdentauth"
)

type accountView struct {
	ID            string    `json:"id"`
	Label         string    `json:"label"`
	State         string    `json:"state"`
	RuntimeState  string    `json:"runtime_state"`
	Enabled       bool      `json:"enabled"`
	DeviceID      string    `json:"device_id"`
	TeamID        string    `json:"team_id"`
	ProxyURL      string    `json:"proxy_url"`
	TokenUID      string    `json:"token_uid,omitempty"`
	TokenExpires  int64     `json:"token_expires,omitempty"`
	CooldownUntil time.Time `json:"cooldown_until,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
}

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	snapshot := s.Accounts.Snapshot()
	counts := map[string]int{"healthy": 0, "cooling_down": 0, "suspended": 0, "disabled": 0}
	for _, item := range snapshot {
		if item.Credential.Disabled {
			counts["disabled"]++
			continue
		}
		counts[string(item.State)]++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": s.Version,
		"accounts": map[string]any{
			"total": len(snapshot),
			"healthy": counts["healthy"],
			"cooling_down": counts["cooling_down"],
			"suspended": counts["suspended"],
			"disabled": counts["disabled"],
		},
		"protocol": map[string]any{
			"app_version": s.ProtocolConfig.AppVersion,
			"beta": s.ProtocolConfig.BetaHeader,
			"endpoint": s.ProtocolConfig.Endpoint,
		},
		"api": map[string]any{
			"chat_completions": true,
			"responses": true,
			"models": true,
		},
	})
}

func (s *Server) handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	snapshot := s.Accounts.Snapshot()
	out := make([]accountView, 0, len(snapshot))
	for _, item := range snapshot {
		out = append(out, toAccountView(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out, "store": s.AccountStore.Path})
}

func (s *Server) handleAdminAddToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token    string `json:"token"`
		Label    string `json:"label"`
		DeviceID string `json:"device_id"`
		TeamID   string `json:"team_id"`
		ProxyURL string `json:"proxy_url"`
	}
	if err := readAdminJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(input.Token) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "token is required"})
		return
	}
	if err := validateProxy(input.ProxyURL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	item, err := s.upsertToken(input.Token, input.Label, input.DeviceID, input.TeamID, input.ProxyURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": toAccountView(item)})
}

func (s *Server) handleAdminAccountState(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var input struct {
		Disabled bool `json:"disabled"`
	}
	if err := readAdminJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if _, ok := s.Accounts.Credential(id); !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "account not found"})
		return
	}
	s.Accounts.SetDisabled(id, input.Disabled)
	if err := s.persistAccounts(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminAccountProxy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var input struct {
		ProxyURL string `json:"proxy_url"`
	}
	if err := readAdminJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := validateProxy(input.ProxyURL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if !s.Accounts.UpdateCredential(id, func(c *account.Credential) { c.ProxyURL = strings.TrimSpace(input.ProxyURL) }) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "account not found"})
		return
	}
	if err := s.persistAccounts(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminAccountDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.Accounts.Remove(id) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "account not found"})
		return
	}
	if err := s.persistAccounts(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAdminAccountTest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	credential, ok := s.Accounts.Credential(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "account not found"})
		return
	}
	selected, err := s.ensureFreshAccount(r.Context(), &account.Account{Credential: credential, State: account.StateHealthy})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.diagnoseAccount(ctx, selected))
}

func (s *Server) handleAdminModels(w http.ResponseWriter, r *http.Request) {
	selected := s.Accounts.Select("admin-model-catalog", "")
	if selected == nil {
		writeJSON(w, http.StatusOK, map[string]any{"models": []any{}, "error": "no eligible account"})
		return
	}
	selected, err := s.ensureFreshAccount(r.Context(), selected)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	client, err := s.NewClient(*selected, s.ProtocolConfig, 30*time.Second)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	models, err := client.DiscoverModels(ctx, selected.Credential.Token)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models, "account_id": selected.Credential.ID})
}

func (s *Server) handleVerdentOAuthStart(w http.ResponseWriter, r *http.Request) {
	if s.OAuth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "Verdent OAuth is not configured"})
		return
	}
	result, err := s.OAuth.Start(s.publicBaseURL(r))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleVerdentOAuthStatus(w http.ResponseWriter, r *http.Request) {
	if s.OAuth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "Verdent OAuth is not configured"})
		return
	}
	flow, ok := s.OAuth.Status(r.URL.Query().Get("flow_id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "OAuth flow not found or expired"})
		return
	}
	writeJSON(w, http.StatusOK, flow)
}

func (s *Server) handleVerdentOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if s.OAuth == nil {
		http.Error(w, "Verdent OAuth is not configured", http.StatusServiceUnavailable)
		return
	}
	flowID, token, err := s.OAuth.ExchangeCallback(r.Context(), r.URL.Query())
	if err != nil {
		s.oauthCallbackPage(w, false, err.Error())
		return
	}
	item, err := s.upsertAuthToken(token, "", "", "0", "")
	if err != nil {
		s.OAuth.Fail(flowID, err)
		s.oauthCallbackPage(w, false, err.Error())
		return
	}
	s.OAuth.Complete(flowID, item.Credential.ID)
	s.oauthCallbackPage(w, true, item.Credential.Label)
}

func (s *Server) handleVerdentDesktopImport(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	token, err := verdentauth.ReadDesktopToken(ctx)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	item, err := s.upsertToken(token, "", "", "0", "")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account": toAccountView(item)})
}

func (s *Server) upsertToken(token, label, deviceID, teamID, proxyURL string) (account.Account, error) {
	return s.upsertAuthToken(verdentauth.TokenResponse{Token: token}, label, deviceID, teamID, proxyURL)
}

func (s *Server) upsertAuthToken(authToken verdentauth.TokenResponse, label, deviceID, teamID, proxyURL string) (account.Account, error) {
	token := strings.TrimSpace(authToken.Token)
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		teamID = "0"
	}
	meta := account.MetadataFromToken(token)
	identity := strings.TrimSpace(authToken.UserID)
	if identity == "" {
		identity = meta.UID
	}
	id := account.StableAccountIDForIdentity(identity, teamID)
	existing, exists := s.Accounts.Credential(id)
	if strings.TrimSpace(label) == "" {
		if exists && existing.Label != "" {
			label = existing.Label
		} else {
			label = meta.Label
		}
	}
	if strings.TrimSpace(deviceID) == "" {
		if exists && existing.DeviceID != "" {
			deviceID = existing.DeviceID
		} else {
			deviceID = account.NewDeviceID()
		}
	}
	if strings.TrimSpace(proxyURL) == "" && exists {
		proxyURL = existing.ProxyURL
	}
	refreshToken := strings.TrimSpace(authToken.RefreshToken)
	tokenExpiresAt := int64(0)
	if !authToken.ExpiresAt.IsZero() {
		tokenExpiresAt = authToken.ExpiresAt.Unix()
	}
	if exists {
		if refreshToken == "" {
			refreshToken = existing.RefreshToken
		}
		if tokenExpiresAt == 0 {
			tokenExpiresAt = existing.TokenExpiresAt
		}
	}
	credential := account.Credential{
		ID: id, Label: strings.TrimSpace(label), Token: token,
		RefreshToken: refreshToken, TokenExpiresAt: tokenExpiresAt,
		DeviceID: strings.TrimSpace(deviceID), TeamID: teamID, ProxyURL: strings.TrimSpace(proxyURL),
	}
	if exists {
		credential.Disabled = existing.Disabled
	}
	s.Accounts.Add(credential)
	if err := s.persistAccounts(); err != nil {
		return account.Account{}, err
	}
	for _, current := range s.Accounts.Snapshot() {
		if current.Credential.ID == id {
			return current, nil
		}
	}
	return account.Account{}, fmt.Errorf("account %s disappeared after upsert", id)
}

func (s *Server) ensureFreshAccount(ctx context.Context, selected *account.Account) (*account.Account, error) {
	if selected == nil {
		return nil, fmt.Errorf("no account selected")
	}
	credential := selected.Credential
	if credential.RefreshToken == "" || credential.TokenExpiresAt == 0 || time.Until(time.Unix(credential.TokenExpiresAt, 0)) > time.Minute {
		return selected, nil
	}
	if s.OAuth == nil {
		if time.Now().Unix() >= credential.TokenExpiresAt {
			return nil, fmt.Errorf("Verdent access token expired and OAuth refresh is unavailable")
		}
		return selected, nil
	}

	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()

	current, ok := s.Accounts.Credential(credential.ID)
	if !ok {
		return nil, fmt.Errorf("account %s disappeared before token refresh", credential.ID)
	}
	if current.RefreshToken == "" || current.TokenExpiresAt == 0 || time.Until(time.Unix(current.TokenExpiresAt, 0)) > time.Minute {
		clone := *selected
		clone.Credential = current
		return &clone, nil
	}

	refreshed, err := s.OAuth.Refresh(ctx, current.RefreshToken)
	if err != nil {
		if time.Now().Unix() < current.TokenExpiresAt {
			clone := *selected
			clone.Credential = current
			return &clone, nil
		}
		return nil, fmt.Errorf("refresh Verdent access token: %w", err)
	}
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = current.RefreshToken
	}
	if refreshed.UserID == "" {
		refreshed.UserID = account.MetadataFromToken(current.Token).UID
	}

	s.Accounts.UpdateCredential(current.ID, func(c *account.Credential) {
		c.Token = refreshed.Token
		c.RefreshToken = refreshed.RefreshToken
		if !refreshed.ExpiresAt.IsZero() {
			c.TokenExpiresAt = refreshed.ExpiresAt.Unix()
		}
	})
	if err := s.persistAccounts(); err != nil {
		return nil, fmt.Errorf("persist refreshed Verdent token: %w", err)
	}
	updated, _ := s.Accounts.Credential(current.ID)
	clone := *selected
	clone.Credential = updated
	return &clone, nil
}

func (s *Server) persistAccounts() error {
	return s.AccountStore.Save(s.Accounts.Credentials())
}

func toAccountView(item account.Account) accountView {
	meta := account.MetadataFromToken(item.Credential.Token)
	expires := meta.Expires
	if item.Credential.TokenExpiresAt > 0 {
		expires = item.Credential.TokenExpiresAt
	}
	effectiveState := string(item.State)
	if item.Credential.Disabled {
		effectiveState = string(account.StateDisabled)
	}
	return accountView{
		ID: item.Credential.ID,
		Label: item.Credential.Label,
		State: effectiveState,
		RuntimeState: string(item.State),
		Enabled: !item.Credential.Disabled,
		DeviceID: item.Credential.DeviceID,
		TeamID: item.Credential.TeamID,
		ProxyURL: maskProxyURL(item.Credential.ProxyURL),
		TokenUID: meta.UID,
		TokenExpires: expires,
		CooldownUntil: item.CooldownUntil,
		LastError: item.LastError,
	}
}

func maskProxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "***"
	}
	if parsed.User != nil {
		parsed.User = nil
	}
	return parsed.String()
}

func validateProxy(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("invalid proxy URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("proxy must use http:// or https://")
	}
	return nil
}

func readAdminJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func (s *Server) publicBaseURL(r *http.Request) string {
	if strings.TrimSpace(s.PublicBaseURL) != "" {
		return strings.TrimRight(strings.TrimSpace(s.PublicBaseURL), "/")
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0]); forwarded == "https" || forwarded == "http" {
		scheme = forwarded
	}
	return scheme + "://" + r.Host
}

func (s *Server) oauthCallbackPage(w http.ResponseWriter, success bool, detail string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	status := "failed"
	title := "Verdent 登录失败"
	if success {
		status = "success"
		title = "Verdent 登录成功"
	}
	_, _ = fmt.Fprintf(w, `<!doctype html><html lang="zh-CN"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title><body style="font-family:system-ui;padding:40px;background:#f7f7f8;color:#202124"><div style="max-width:520px;margin:auto;background:white;border:1px solid #ddd;border-radius:12px;padding:28px"><h2>%s</h2><p>%s</p><p style="color:#777">此窗口可以关闭。</p></div><script>try{window.opener&&window.opener.postMessage({type:"verdent-auth",status:%q},location.origin)}catch(e){};setTimeout(()=>window.close(),900)</script></body></html>`, html.EscapeString(title), html.EscapeString(title), html.EscapeString(detail), status)
}

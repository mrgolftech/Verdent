package verdentauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultAuthBase  = "https://www.verdent.ai"
	defaultLoginBase = "https://login.verdent.ai"
)

type Config struct {
	AuthBaseURL  string
	LoginBaseURL string
	UserAgent    string
	Timeout      time.Duration
}

type Manager struct {
	cfg     Config
	client  *http.Client
	mu      sync.Mutex
	pending map[string]*Flow
	byState map[string]string
}

type Flow struct {
	ID        string    `json:"id"`
	State     string    `json:"-"`
	Verifier  string    `json:"-"`
	AuthID    string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"`
	AccountID string    `json:"account_id,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type StartResult struct {
	FlowID  string `json:"flow_id"`
	AuthURL string `json:"auth_url"`
}

type TokenResponse struct {
	Token        string
	RefreshToken string
	UserID       string
	ExpiresAt    time.Time
	Raw          map[string]any
}

func New(cfg Config, client *http.Client) *Manager {
	if strings.TrimSpace(cfg.AuthBaseURL) == "" {
		cfg.AuthBaseURL = defaultAuthBase
	}
	if strings.TrimSpace(cfg.LoginBaseURL) == "" {
		cfg.LoginBaseURL = defaultLoginBase
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	return &Manager{
		cfg: cfg,
		client: client,
		pending: make(map[string]*Flow),
		byState: make(map[string]string),
	}
}

// Start builds the same PKCE authorization shape observed in Verdent Desktop
// and independently reproduced by current Verdent2API implementations:
//
//   /auth?challenge=<S256>&state=<random>&intent=signin
//     &callback=<gateway callback>&ots=deck&source=deck&id=<random>
//
// The callback itself is intentionally query-free. OAuth state is the single
// CSRF/callback binding so intermediaries cannot break the flow by stripping
// custom query parameters from the registered callback URL.
func (m *Manager) Start(callbackBase string) (StartResult, error) {
	callbackBase = strings.TrimRight(strings.TrimSpace(callbackBase), "/")
	if callbackBase == "" {
		return StartResult{}, errors.New("callback base URL is required")
	}
	base, err := url.Parse(callbackBase)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return StartResult{}, errors.New("callback base URL must be an absolute http(s) URL")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return StartResult{}, errors.New("callback base URL must use http or https")
	}

	flow := &Flow{
		ID: randomURLToken(18),
		State: randomURLToken(32),
		Verifier: randomURLToken(32),
		AuthID: randomHexToken(16),
		CreatedAt: time.Now(),
		Status: "pending",
	}
	callback := callbackBase + "/api/auth/verdent/callback"

	auth, err := url.Parse(strings.TrimRight(m.cfg.AuthBaseURL, "/") + "/auth")
	if err != nil {
		return StartResult{}, err
	}
	q := auth.Query()
	q.Set("challenge", codeChallenge(flow.Verifier))
	q.Set("state", flow.State)
	q.Set("intent", "signin")
	q.Set("callback", callback)
	q.Set("ots", "deck")
	q.Set("source", "deck")
	q.Set("id", flow.AuthID)
	auth.RawQuery = q.Encode()

	m.mu.Lock()
	m.cleanupLocked(time.Now())
	m.pending[flow.ID] = flow
	m.byState[flow.State] = flow.ID
	m.mu.Unlock()

	return StartResult{FlowID: flow.ID, AuthURL: auth.String()}, nil
}

func (m *Manager) Status(flowID string) (Flow, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanupLocked(time.Now())
	flow := m.pending[flowID]
	if flow == nil {
		return Flow{}, false
	}
	return *flow, true
}

func (m *Manager) ExchangeCallback(ctx context.Context, values url.Values) (string, TokenResponse, error) {
	state := strings.TrimSpace(values.Get("state"))
	if state == "" {
		return "", TokenResponse{}, errors.New("missing OAuth state")
	}

	m.mu.Lock()
	m.cleanupLocked(time.Now())
	flowID := m.byState[state]
	flow := m.pending[flowID]
	if flow == nil {
		m.mu.Unlock()
		return "", TokenResponse{}, errors.New("OAuth flow is missing, expired, or state does not match")
	}
	if message := callbackError(values); message != "" {
		flow.Status = "failed"
		flow.Error = message
		m.mu.Unlock()
		return flow.ID, TokenResponse{}, errors.New(message)
	}
	code := strings.TrimSpace(values.Get("code"))
	if code == "" {
		m.mu.Unlock()
		return flow.ID, TokenResponse{}, errors.New("missing OAuth authorization code")
	}
	flow.Status = "exchanging"
	verifier := flow.Verifier
	m.mu.Unlock()

	token, err := m.exchangeCode(ctx, code, verifier)
	if err != nil {
		m.Fail(flow.ID, err)
		return flow.ID, TokenResponse{}, err
	}
	return flow.ID, token, nil
}

func (m *Manager) Refresh(ctx context.Context, refreshToken string) (TokenResponse, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return TokenResponse{}, errors.New("refresh token is required")
	}
	payload, _ := json.Marshal(map[string]string{"refreshToken": refreshToken})
	tokenURL := strings.TrimRight(m.cfg.LoginBaseURL, "/") + "/passport/token/refresh"
	return m.postTokenRequest(ctx, tokenURL, payload)
}

func (m *Manager) Complete(flowID, accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if flow := m.pending[flowID]; flow != nil {
		flow.Status = "complete"
		flow.AccountID = accountID
		flow.Error = ""
		delete(m.byState, flow.State)
	}
}

func (m *Manager) Fail(flowID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if flow := m.pending[flowID]; flow != nil {
		flow.Status = "failed"
		if err != nil {
			flow.Error = err.Error()
		}
		delete(m.byState, flow.State)
	}
}

func (m *Manager) exchangeCode(ctx context.Context, code, verifier string) (TokenResponse, error) {
	payload, _ := json.Marshal(map[string]string{"code": code, "codeVerifier": verifier})
	tokenURL := strings.TrimRight(m.cfg.LoginBaseURL, "/") + "/passport/pkce/callback"
	return m.postTokenRequest(ctx, tokenURL, payload)
}

func (m *Manager) postTokenRequest(ctx context.Context, endpoint string, payload []byte) (TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if m.cfg.UserAgent != "" {
		req.Header.Set("User-Agent", m.cfg.UserAgent)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("Verdent token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return TokenResponse{}, fmt.Errorf("read Verdent token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail := strings.TrimSpace(string(body))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		if detail == "" {
			return TokenResponse{}, fmt.Errorf("Verdent token request failed (%d)", resp.StatusCode)
		}
		return TokenResponse{}, fmt.Errorf("Verdent token request failed (%d): %s", resp.StatusCode, detail)
	}

	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return TokenResponse{}, fmt.Errorf("decode Verdent token response: %w", err)
	}
	token, err := parseTokenResponse(root)
	if err != nil {
		return TokenResponse{}, err
	}
	return token, nil
}

func parseTokenResponse(root map[string]any) (TokenResponse, error) {
	data, _ := root["data"].(map[string]any)
	token := firstString(root, "token", "access_token", "accessToken")
	if token == "" && data != nil {
		token = firstString(data, "token", "access_token", "accessToken")
	}
	if token == "" {
		return TokenResponse{}, errors.New("Verdent token response is missing access token")
	}

	refresh := firstString(root, "refreshToken", "refresh_token")
	userID := firstString(root, "userId", "user_id", "uid")
	if data != nil {
		if refresh == "" {
			refresh = firstString(data, "refreshToken", "refresh_token")
		}
		if userID == "" {
			userID = firstString(data, "userId", "user_id", "uid")
		}
	}

	var expiresAt time.Time
	for _, source := range []map[string]any{data, root} {
		if source == nil {
			continue
		}
		for _, key := range []string{"accessTokenExpiresAt", "expireTime", "expires_at"} {
			if value := numeric(source[key]); value > 0 {
				expiresAt = unixTimeFlexible(value)
				break
			}
		}
		if !expiresAt.IsZero() {
			break
		}
		if value := numeric(source["expires_in"]); value > 0 {
			expiresAt = time.Now().Add(time.Duration(value) * time.Second)
			break
		}
	}

	return TokenResponse{
		Token: token,
		RefreshToken: refresh,
		UserID: userID,
		ExpiresAt: expiresAt,
		Raw: root,
	}, nil
}

func (m *Manager) cleanupLocked(now time.Time) {
	for id, flow := range m.pending {
		if now.Sub(flow.CreatedAt) > 10*time.Minute {
			delete(m.byState, flow.State)
			delete(m.pending, id)
		}
	}
}

func callbackError(values url.Values) string {
	if values.Get("error") == "" {
		return ""
	}
	for _, key := range []string{"error_description", "uh", "error"} {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return "Verdent authorization failed"
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomURLToken(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%d", time.Now().UnixNano())))
		buf = sum[:size]
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func randomHexToken(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("hex:%d", time.Now().UnixNano())))
		buf = sum[:size]
	}
	const alphabet = "0123456789abcdef"
	out := make([]byte, len(buf)*2)
	for i, b := range buf {
		out[i*2] = alphabet[b>>4]
		out[i*2+1] = alphabet[b&0x0f]
	}
	return string(out)
}

func firstString(root map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := root[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case json.Number:
			return value.String()
		case float64:
			if value == float64(int64(value)) {
				return fmt.Sprintf("%d", int64(value))
			}
		}
	}
	return ""
}

func numeric(value any) int64 {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case json.Number:
		n, _ := v.Int64()
		return n
	case string:
		var n int64
		_, _ = fmt.Sscan(v, &n)
		return n
	default:
		return 0
	}
}

func unixTimeFlexible(value int64) time.Time {
	if value > 10_000_000_000 {
		value /= 1000
	}
	return time.Unix(value, 0)
}

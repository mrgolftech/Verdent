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
	ID            string    `json:"id"`
	State         string    `json:"-"`
	Verifier      string    `json:"-"`
	RequestID     string    `json:"-"`
	CallbackNonce string    `json:"-"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"`
	AccountID     string    `json:"account_id,omitempty"`
	Error         string    `json:"error,omitempty"`
}

type StartResult struct {
	FlowID  string `json:"flow_id"`
	AuthURL string `json:"auth_url"`
}

type TokenResponse struct {
	Token       string
	ExpiresAt   time.Time
	Raw         map[string]any
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

func (m *Manager) Start(callbackBase string) (StartResult, error) {
	callbackBase = strings.TrimRight(strings.TrimSpace(callbackBase), "/")
	if callbackBase == "" {
		return StartResult{}, errors.New("callback base URL is required")
	}
	if _, err := url.ParseRequestURI(callbackBase); err != nil {
		return StartResult{}, fmt.Errorf("invalid callback base URL: %w", err)
	}

	flow := &Flow{
		ID: randomURLToken(18),
		State: randomURLToken(32),
		Verifier: randomURLToken(32),
		RequestID: randomURLToken(16),
		CallbackNonce: randomURLToken(16),
		CreatedAt: time.Now(),
		Status: "pending",
	}

	callback, err := url.Parse(callbackBase + "/api/auth/verdent/callback")
	if err != nil {
		return StartResult{}, err
	}
	q := callback.Query()
	q.Set("rid", flow.RequestID)
	q.Set("nonce", flow.CallbackNonce)
	callback.RawQuery = q.Encode()

	auth, err := url.Parse(strings.TrimRight(m.cfg.AuthBaseURL, "/") + "/auth")
	if err != nil {
		return StartResult{}, err
	}
	aq := auth.Query()
	aq.Set("challenge", codeChallenge(flow.Verifier))
	aq.Set("state", flow.State)
	aq.Set("callback", callback.String())
	aq.Set("intent", "signin")
	aq.Set("ots", "deck")
	aq.Set("source", "deck")
	aq.Set("id", flow.RequestID)
	auth.RawQuery = aq.Encode()

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
	state := values.Get("state")
	if state == "" {
		return "", TokenResponse{}, errors.New("missing OAuth state")
	}

	m.mu.Lock()
	m.cleanupLocked(time.Now())
	flowID := m.byState[state]
	flow := m.pending[flowID]
	if flow == nil {
		m.mu.Unlock()
		return "", TokenResponse{}, errors.New("OAuth flow is missing or expired")
	}
	if values.Get("rid") != flow.RequestID || values.Get("nonce") != flow.CallbackNonce {
		m.mu.Unlock()
		return flow.ID, TokenResponse{}, errors.New("invalid OAuth callback binding")
	}
	if message := callbackError(values); message != "" {
		flow.Status = "failed"
		flow.Error = message
		m.mu.Unlock()
		return flow.ID, TokenResponse{}, errors.New(message)
	}
	code := values.Get("code")
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

func (m *Manager) Complete(flowID, accountID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if flow := m.pending[flowID]; flow != nil {
		flow.Status = "complete"
		flow.AccountID = accountID
		flow.Error = ""
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
	}
}

func (m *Manager) exchangeCode(ctx context.Context, code, verifier string) (TokenResponse, error) {
	payload, _ := json.Marshal(map[string]string{"code": code, "codeVerifier": verifier})
	tokenURL := strings.TrimRight(m.cfg.LoginBaseURL, "/") + "/passport/pkce/callback"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewReader(payload))
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
		return TokenResponse{}, fmt.Errorf("Verdent OAuth token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return TokenResponse{}, fmt.Errorf("read Verdent OAuth response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenResponse{}, fmt.Errorf("Verdent OAuth token exchange failed (%d)", resp.StatusCode)
	}

	var root map[string]any
	if err := json.Unmarshal(body, &root); err != nil {
		return TokenResponse{}, fmt.Errorf("decode Verdent OAuth response: %w", err)
	}
	token := firstString(root, "token", "access_token", "accessToken")
	data, _ := root["data"].(map[string]any)
	if token == "" && data != nil {
		token = firstString(data, "token", "access_token", "accessToken")
	}
	if token == "" {
		return TokenResponse{}, errors.New("Verdent OAuth response is missing access token")
	}

	var expiresAt time.Time
	if data != nil {
		if value := numeric(data["expireTime"]); value > 0 {
			expiresAt = time.Unix(value, 0)
		}
	}
	if expiresAt.IsZero() {
		if value := numeric(root["expireTime"]); value > 0 {
			expiresAt = time.Unix(value, 0)
		} else if value := numeric(root["expires_in"]); value > 0 {
			expiresAt = time.Now().Add(time.Duration(value) * time.Second)
		}
	}
	return TokenResponse{Token: token, ExpiresAt: expiresAt, Raw: root}, nil
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

func firstString(root map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := root[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
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

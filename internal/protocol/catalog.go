package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ContextWindow struct {
	Display string `json:"display"`
	Tokens  int    `json:"tokens"`
}

type ModelInfo struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Family           string          `json:"family,omitempty"`
	Description      string          `json:"description,omitempty"`
	ContextWindows   []ContextWindow `json:"context_windows,omitempty"`
	MaxOutputTokens  int             `json:"max_output_tokens,omitempty"`
	SupportsImages   bool            `json:"supports_images,omitempty"`
	SupportsThinking bool            `json:"supports_thinking,omitempty"`
	EffortLevels     []string        `json:"effort_levels,omitempty"`
	IsLimitFree      bool            `json:"is_limit_free,omitempty"`
	CostMultiplier   float64         `json:"cost_multiplier,omitempty"`
}

type rawCatalogResponse struct {
	Data struct {
		Models []rawCatalogModel `json:"model_config"`
	} `json:"data"`
	Models []rawCatalogModel `json:"model_config"`
}

type rawCatalogModel struct {
	Key                   string  `json:"key"`
	Label                 string  `json:"label"`
	Description           string  `json:"description"`
	ContextWindow         string  `json:"contextWindow"`
	CostMultiplier        float64 `json:"costMultiplier"`
	IsHidden              bool    `json:"is_hidden"`
	IsLimitFree           bool    `json:"is_limit_free"`
	SupportsImages        bool    `json:"supportsImages"`
	SupportsThinking      bool    `json:"supports_thinking"`
	SupportsReasoning     bool    `json:"supportsReasoningEffort"`
	DefaultMaxOutputTokens int    `json:"default_max_output_tokens"`
	Provider              []string `json:"provider"`
	ContextWindows []struct {
		Display string `json:"contextWindowDisplay"`
		Tokens  int    `json:"contextWindowTokens"`
	} `json:"contextWindows"`
	EffortLevels []struct {
		Label string `json:"label"`
	} `json:"effortLevels"`
}

func ParseCatalog(body []byte) ([]ModelInfo, error) {
	var raw rawCatalogResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode model catalog: %w", err)
	}
	models := raw.Data.Models
	if len(models) == 0 { models = raw.Models }
	out := make([]ModelInfo, 0, len(models))
	for _, m := range models {
		if m.Key == "" || m.IsHidden { continue }
		name := m.Label
		if name == "" { name = m.Key }
		info := ModelInfo{
			ID: m.Key, Name: name, Description: m.Description, MaxOutputTokens: m.DefaultMaxOutputTokens,
			SupportsImages: m.SupportsImages, SupportsThinking: m.SupportsThinking || m.SupportsReasoning,
			IsLimitFree: m.IsLimitFree, CostMultiplier: m.CostMultiplier,
		}
		if len(m.Provider) > 0 { info.Family = strings.ToLower(m.Provider[0]) }
		for _, cw := range m.ContextWindows {
			if cw.Display == "" || cw.Tokens <= 0 { continue }
			info.ContextWindows = append(info.ContextWindows, ContextWindow{Display:cw.Display, Tokens:cw.Tokens})
		}
		if len(info.ContextWindows) == 0 && m.ContextWindow != "" {
			if n, ok := parseCompactTokenCount(m.ContextWindow); ok {
				info.ContextWindows = []ContextWindow{{Display:m.ContextWindow, Tokens:n}}
			}
		}
		seen := map[string]bool{}
		for _, e := range m.EffortLevels {
			v := strings.ToLower(strings.TrimSpace(e.Label))
			if v == "" || seen[v] { continue }
			seen[v] = true
			info.EffortLevels = append(info.EffortLevels, v)
		}
		out = append(out, info)
	}
	return out, nil
}

func parseCompactTokenCount(s string) (int, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" { return 0, false }
	mult := 1
	last := s[len(s)-1]
	if last == 'k' { mult = 1000; s = s[:len(s)-1] }
	if last == 'm' { mult = 1000000; s = s[:len(s)-1] }
	var whole, frac int
	var hasFrac bool
	for _, r := range s {
		if r == '.' {
			if hasFrac { return 0, false }
			hasFrac = true
			continue
		}
		if r < '0' || r > '9' { return 0, false }
		if hasFrac { frac = frac*10 + int(r-'0') } else { whole = whole*10 + int(r-'0') }
	}
	if hasFrac {
		// Compact display values are normally integers; support one decimal place.
		return whole*mult + frac*(mult/10), true
	}
	return whole*mult, true
}

func (c *Client) DiscoverModels(ctx context.Context, token string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.CatalogEndpoint, nil)
	if err != nil { return nil, fmt.Errorf("create catalog request: %w", err) }
	headers, err := BuildHeaders(token, c.cfg)
	if err != nil { return nil, err }
	req.Header = headers
	resp, err := c.http.Do(req)
	if err != nil { return nil, fmt.Errorf("fetch catalog: %w", err) }
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil { return nil, fmt.Errorf("read catalog response: %w", err) }
	if resp.StatusCode != http.StatusOK { return nil, fmt.Errorf("catalog HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))) }
	return ParseCatalog(body)
}

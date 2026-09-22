package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mrgolftech/Verdent/internal/canonical"
)

type Client struct {
	cfg   Config
	codec *Codec
	http  *http.Client
	gate  *RequestGate
}

func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	return NewClientWithGate(cfg, httpClient, NewRequestGate(cfg.MinRequestInterval))
}

func NewClientWithGate(cfg Config, httpClient *http.Client, gate *RequestGate) (*Client, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil { return nil, err }
	codec, err := NewCodec(cfg.Sign)
	if err != nil { return nil, err }
	if httpClient == nil { httpClient = http.DefaultClient }
	if gate == nil { gate = NewRequestGate(cfg.MinRequestInterval) }
	return &Client{cfg: cfg, codec: codec, http: httpClient, gate: gate}, nil
}

func (c *Client) Do(ctx context.Context, token string, req canonical.Request, opt EnvelopeOptions) (*http.Response, error) {
	envelope, err := BuildEnvelope(req, c.cfg, c.codec, opt)
	if err != nil { return nil, err }
	body, err := json.Marshal(envelope)
	if err != nil { return nil, fmt.Errorf("marshal Verdent envelope: %w", err) }

	delays := c.cfg.RetryDelays
	for attempt := 0; ; attempt++ {
		if err := c.gate.Wait(ctx); err != nil {
			return nil, err
		}
		resp, err := c.doOnce(ctx, token, body)
		if err != nil {
			return nil, err
		}
		if attempt >= len(delays) {
			return resp, nil
		}

		retry, retryDelay, err := retryDecision(resp, delays[attempt])
		if err != nil {
			return nil, err
		}
		if !retry {
			return resp, nil
		}
		if err := sleepContext(ctx, retryDelay); err != nil {
			return nil, err
		}
	}
}

func (c *Client) doOnce(ctx context.Context, token string, body []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil { return nil, fmt.Errorf("create Verdent request: %w", err) }
	headers, err := BuildHeaders(token, c.cfg)
	if err != nil { return nil, err }
	httpReq.Header = headers
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Verdent upstream request: %w", err)
	}
	return resp, nil
}

func retryDecision(resp *http.Response, fallback time.Duration) (bool, time.Duration, error) {
	if resp == nil {
		return false, 0, nil
	}
	status := resp.StatusCode
	if status != http.StatusTooManyRequests &&
		status != http.StatusInternalServerError &&
		status != http.StatusBadGateway &&
		status != http.StatusServiceUnavailable &&
		status != http.StatusGatewayTimeout {
		return false, 0, nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		resp.Body.Close()
		return false, 0, fmt.Errorf("read Verdent retry response: %w", err)
	}
	resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))

	raw := strings.TrimSpace(string(body))
	retry := status == http.StatusTooManyRequests
	if status >= 500 {
		retry = strings.Contains(raw, "20004") || strings.Contains(raw, "need_retry")
		if retry {
			var payload map[string]any
			if json.Unmarshal(body, &payload) == nil {
				if value, ok := payload["need_retry"].(bool); ok && !value && !strings.Contains(raw, "20004") {
					retry = false
				}
			}
		}
	}
	if !retry {
		return false, 0, nil
	}

	delay := parseRetryDelay(resp.Header.Get("Retry-After"), body, fallback)
	if delay > 45*time.Second {
		delay = 45*time.Second
	}
	if delay < 0 {
		delay = 0
	}
	return true, delay, nil
}

func parseRetryDelay(retryAfter string, body []byte, fallback time.Duration) time.Duration {
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		for _, key := range []string{"retryAfterMs", "retry_after_ms"} {
			switch value := payload[key].(type) {
			case float64:
				if value > 0 { return time.Duration(value) * time.Millisecond }
			case json.Number:
				if ms, err := value.Int64(); err == nil && ms > 0 { return time.Duration(ms) * time.Millisecond }
			case string:
				if ms, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && ms > 0 {
					return time.Duration(ms) * time.Millisecond
				}
			}
		}
	}
	if seconds, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

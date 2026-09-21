package protocol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mrgolftech/Verdent/internal/canonical"
)

type Client struct {
	cfg   Config
	codec *Codec
	http  *http.Client
}

func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil { return nil, err }
	codec, err := NewCodec(cfg.Sign)
	if err != nil { return nil, err }
	if httpClient == nil { httpClient = http.DefaultClient }
	return &Client{cfg: cfg, codec: codec, http: httpClient}, nil
}

func (c *Client) Do(ctx context.Context, token string, req canonical.Request, opt EnvelopeOptions) (*http.Response, error) {
	envelope, err := BuildEnvelope(req, c.cfg, c.codec, opt)
	if err != nil { return nil, err }
	body, err := json.Marshal(envelope)
	if err != nil { return nil, fmt.Errorf("marshal Verdent envelope: %w", err) }
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(body))
	if err != nil { return nil, fmt.Errorf("create Verdent request: %w", err) }
	headers, err := BuildHeaders(token, c.cfg)
	if err != nil { return nil, err }
	httpReq.Header = headers
	return c.http.Do(httpReq)
}

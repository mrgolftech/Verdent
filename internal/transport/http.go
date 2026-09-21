package transport

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// NewHTTPClient creates an isolated transport. Callers should create one client
// per account so proxy affinity cannot change accidentally between requests.
func NewHTTPClient(proxyRaw string, timeout time.Duration) (*http.Client, error) {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok { return nil, fmt.Errorf("default HTTP transport has unexpected type") }
	tr := base.Clone()
	tr.Proxy = nil
	if proxyRaw != "" {
		u, err := url.Parse(proxyRaw)
		if err != nil { return nil, fmt.Errorf("parse proxy URL: %w", err) }
		if u.Scheme != "http" && u.Scheme != "https" {
			return nil, fmt.Errorf("unsupported proxy scheme %q", u.Scheme)
		}
		if u.Host == "" { return nil, fmt.Errorf("proxy host is required") }
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Transport: tr, Timeout: timeout}, nil
}

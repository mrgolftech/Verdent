package transport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPerClientProxyAffinity(t *testing.T) {
	targetHits := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetHits++
		_, _ = io.WriteString(w, "direct")
	}))
	defer target.Close()

	proxyHits := 0
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits++
		if !r.URL.IsAbs() { t.Errorf("proxy request URL should be absolute: %s", r.URL.String()) }
		_, _ = io.WriteString(w, "proxied")
	}))
	defer proxy.Close()

	directClient, err := NewHTTPClient("", 5*time.Second)
	if err != nil { t.Fatal(err) }
	resp, err := directClient.Get(target.URL)
	if err != nil { t.Fatal(err) }
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "direct" || targetHits != 1 || proxyHits != 0 {
		t.Fatalf("direct path mismatch body=%q target=%d proxy=%d", body, targetHits, proxyHits)
	}

	proxiedClient, err := NewHTTPClient(proxy.URL, 5*time.Second)
	if err != nil { t.Fatal(err) }
	resp, err = proxiedClient.Get(target.URL)
	if err != nil { t.Fatal(err) }
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(body) != "proxied" || targetHits != 1 || proxyHits != 1 {
		t.Fatalf("proxy path mismatch body=%q target=%d proxy=%d", body, targetHits, proxyHits)
	}
}

func TestRejectsUnsupportedProxyScheme(t *testing.T) {
	if _, err := NewHTTPClient("socks5://127.0.0.1:1080", time.Second); err == nil {
		t.Fatal("expected unsupported SOCKS proxy to be rejected until SOCKS transport is implemented")
	}
}

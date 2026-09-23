package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mrgolftech/Verdent/internal/account"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

func adminSessionCookie(t *testing.T, ts *httptest.Server) *http.Cookie {
	t.Helper()
	body := []byte(`{"username":"admin","password":"test-password"}`)
	resp, err := http.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("login failed: %d %s", resp.StatusCode, raw)
	}
	for _, cookie := range resp.Cookies() {
		if cookie.Name == adminCookieName {
			return cookie
		}
	}
	t.Fatal("admin session cookie missing")
	return nil
}

func TestNewUsesBuildVersion(t *testing.T) {
	previous := Version
	Version = "v0.1.0-alpha.4"
	defer func() { Version = previous }()

	s := New(account.NewRouter(nil), protocol.Config{})
	if s.Version != "v0.1.0-alpha.4" {
		t.Fatalf("New() should report the build version, got %q", s.Version)
	}
}

func TestOverviewReportsVersion(t *testing.T) {
	previous := Version
	Version = "v0.1.0-alpha.4+test"
	defer func() { Version = previous }()

	s := New(account.NewRouter(nil), protocol.Config{})
	s.AdminUser = "admin"
	s.AdminPassword = "test-password"
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/overview", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(adminSessionCookie(t, ts))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overview failed: %d %s", resp.StatusCode, raw)
	}
	var overview struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &overview); err != nil {
		t.Fatalf("decode overview: %v (%s)", err, raw)
	}
	if overview.Version != "v0.1.0-alpha.4+test" {
		t.Fatalf("overview version=%q want %q", overview.Version, "v0.1.0-alpha.4+test")
	}
}

func TestVersionDefaultsToDev(t *testing.T) {
	previous := Version
	Version = "dev"
	defer func() { Version = previous }()

	if got := New(account.NewRouter(nil), protocol.Config{}).Version; got != "dev" {
		t.Fatalf("default version=%q want %q", got, "dev")
	}
}

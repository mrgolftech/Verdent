package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrgolftech/Verdent/internal/account"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

func TestAdminConsoleLoginAndManualAccountPersistence(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "data", "accounts.json")
	s := New(account.NewRouter(nil), protocol.Config{})
	s.AdminUser = "admin"
	s.AdminPassword = "test-password"
	s.AccountStore = account.FileStore{Path: storePath}

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	noRedirect := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := noRedirect.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login" {
		t.Fatalf("expected redirect to /login, got %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	loginBody := []byte(`{"username":"admin","password":"test-password"}`)
	resp, err = http.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("login failed: %d %s", resp.StatusCode, raw)
	}
	var session *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == adminCookieName {
			session = cookie
			break
		}
	}
	resp.Body.Close()
	if session == nil || session.Value == "" {
		t.Fatal("admin session cookie missing")
	}

	token := "header.eyJ1c2VyX2lkIjoiMTIzNDU2Nzg5MCIsImVtYWlsIjoidGVzdEBleGFtcGxlLmNvbSIsImV4cCI6MjAwMDAwMDAwMH0.signature"
	addBody := []byte(`{"token":"` + token + `","label":"test-account","proxy_url":"http://user:pass@127.0.0.1:8080"}`)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/accounts/token", bytes.NewReader(addBody))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(session)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add account failed: %d %s", resp.StatusCode, raw)
	}
	if bytes.Contains(raw, []byte(token)) || bytes.Contains(raw, []byte("user:pass")) {
		t.Fatalf("management response leaked secret material: %s", raw)
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/accounts", nil)
	req.AddCookie(session)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	listBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list accounts failed: %d %s", resp.StatusCode, listBody)
	}
	if bytes.Contains(listBody, []byte(token)) || bytes.Contains(listBody, []byte("user:pass")) {
		t.Fatalf("account listing leaked secret material: %s", listBody)
	}
	if !bytes.Contains(listBody, []byte("test-account")) || !bytes.Contains(listBody, []byte("***")) {
		t.Fatalf("account listing missing expected safe fields: %s", listBody)
	}

	persisted, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(persisted, []byte(token)) {
		t.Fatal("managed account token was not persisted server-side")
	}

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	req.AddCookie(session)
	resp, err = noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(page), "Verdent Gateway") {
		t.Fatalf("admin console page not served: status=%d", resp.StatusCode)
	}
}

func TestManagementAPIRequiresAdminSession(t *testing.T) {
	s := New(account.NewRouter(nil), protocol.Config{})
	s.AdminPassword = "secret"
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/accounts")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

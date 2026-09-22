package verdentauth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPKCEFlowMatchesObservedVerdentShape(t *testing.T) {
	var exchange map[string]string
	login := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/passport/pkce/callback":
			body, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(body, &exchange); err != nil { t.Fatal(err) }
			_ = json.NewEncoder(w).Encode(map[string]any{"data":map[string]any{
				"token":"access-1",
				"refreshToken":"refresh-1",
				"userId":1234567890,
				"expireTime":1900000000,
			}})
		case "/passport/token/refresh":
			body, _ := io.ReadAll(r.Body)
			var refresh map[string]string
			if err := json.Unmarshal(body, &refresh); err != nil { t.Fatal(err) }
			if refresh["refreshToken"] != "refresh-1" { t.Fatalf("bad refresh body: %#v", refresh) }
			_ = json.NewEncoder(w).Encode(map[string]any{"data":map[string]any{
				"accessToken":"access-2",
				"refreshToken":"refresh-2",
				"userId":"1234567890",
				"accessTokenExpiresAt":1900001000000,
			}})
		default:
			t.Fatalf("path=%s", r.URL.Path)
		}
	}))
	defer login.Close()

	manager := New(Config{AuthBaseURL:"https://www.verdent.ai",LoginBaseURL:login.URL,UserAgent:"Verdent/2.15.1"}, login.Client())
	start, err := manager.Start("http://127.0.0.1:5084")
	if err != nil { t.Fatal(err) }
	authURL, _ := url.Parse(start.AuthURL)
	if authURL.Path != "/auth" || authURL.Query().Get("intent") != "signin" || authURL.Query().Get("ots") != "deck" || authURL.Query().Get("source") != "deck" {
		t.Fatalf("unexpected auth URL: %s", authURL)
	}
	if len(authURL.Query().Get("challenge")) != 43 || authURL.Query().Get("id") != strings.Repeat("", 0)+authURL.Query().Get("id") || len(authURL.Query().Get("id")) != 32 {
		t.Fatalf("missing PKCE/request identity: %s", authURL)
	}
	callbackURL, err := url.Parse(authURL.Query().Get("callback"))
	if err != nil { t.Fatal(err) }
	if callbackURL.RawQuery != "" || callbackURL.Path != "/api/auth/verdent/callback" {
		t.Fatalf("callback should be clean and stable: %s", callbackURL)
	}

	q := url.Values{}
	q.Set("code","authorization-code")
	q.Set("state",authURL.Query().Get("state"))
	flowID, token, err := manager.ExchangeCallback(context.Background(), q)
	if err != nil { t.Fatal(err) }
	if flowID != start.FlowID || token.Token != "access-1" || token.RefreshToken != "refresh-1" || token.UserID != "1234567890" {
		t.Fatalf("bad callback result: %s %#v", flowID, token)
	}
	if token.ExpiresAt.Unix() != 1900000000 { t.Fatalf("bad expiry: %v", token.ExpiresAt) }
	if exchange["code"] != "authorization-code" || exchange["codeVerifier"] == "" { t.Fatalf("bad exchange body: %#v", exchange) }
	if codeChallenge(exchange["codeVerifier"]) != authURL.Query().Get("challenge") { t.Fatal("challenge does not match verifier") }

	refreshed, err := manager.Refresh(context.Background(), token.RefreshToken)
	if err != nil { t.Fatal(err) }
	if refreshed.Token != "access-2" || refreshed.RefreshToken != "refresh-2" || refreshed.UserID != "1234567890" {
		t.Fatalf("bad refresh result: %#v", refreshed)
	}
	if refreshed.ExpiresAt.Unix() != 1900001000 {
		t.Fatalf("millisecond expiry was not normalized: %v", refreshed.ExpiresAt)
	}

	manager.Complete(flowID,"vd-account")
	status, ok := manager.Status(flowID)
	if !ok || status.Status != "complete" || status.AccountID != "vd-account" { t.Fatalf("bad status: %#v",status) }
}

func TestCallbackRejectsUnknownState(t *testing.T) {
	manager := New(Config{}, &http.Client{Timeout:time.Second})
	_, err := manager.Start("http://127.0.0.1:5084")
	if err != nil { t.Fatal(err) }
	q := url.Values{"state":{"wrong-state"},"code":{"x"}}
	_,_,err = manager.ExchangeCallback(context.Background(),q)
	if err == nil || !strings.Contains(err.Error(),"state") { t.Fatalf("expected state error, got %v",err) }
}

func TestStartRejectsNonHTTPCallback(t *testing.T) {
	manager := New(Config{}, nil)
	if _, err := manager.Start("file:///tmp/callback"); err == nil {
		t.Fatal("expected callback URL validation error")
	}
}

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
)

func TestPKCEFlowMatchesObservedVerdentShape(t *testing.T) {
	var exchange map[string]string
	login := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/passport/pkce/callback" { t.Fatalf("path=%s", r.URL.Path) }
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &exchange); err != nil { t.Fatal(err) }
		_ = json.NewEncoder(w).Encode(map[string]any{"data":map[string]any{"token":"access-1","expireTime":1900000000}})
	}))
	defer login.Close()

	manager := New(Config{AuthBaseURL:"https://www.verdent.ai",LoginBaseURL:login.URL}, login.Client())
	start, err := manager.Start("http://127.0.0.1:5084")
	if err != nil { t.Fatal(err) }
	authURL, _ := url.Parse(start.AuthURL)
	if authURL.Path != "/auth" || authURL.Query().Get("intent") != "signin" || authURL.Query().Get("ots") != "deck" || authURL.Query().Get("source") != "deck" {
		t.Fatalf("unexpected auth URL: %s", authURL)
	}
	if len(authURL.Query().Get("challenge")) != 43 || authURL.Query().Get("id") == "" {
		t.Fatalf("missing PKCE/request identity: %s", authURL)
	}
	callbackURL, err := url.Parse(authURL.Query().Get("callback"))
	if err != nil { t.Fatal(err) }
	callbackQuery := callbackURL.Query()
	callbackQuery.Set("code","authorization-code")
	callbackQuery.Set("state",authURL.Query().Get("state"))
	flowID, token, err := manager.ExchangeCallback(context.Background(), callbackQuery)
	if err != nil { t.Fatal(err) }
	if flowID != start.FlowID || token.Token != "access-1" { t.Fatalf("bad callback result: %s %#v", flowID, token) }
	if exchange["code"] != "authorization-code" || exchange["codeVerifier"] == "" { t.Fatalf("bad exchange body: %#v", exchange) }
	if codeChallenge(exchange["codeVerifier"]) != authURL.Query().Get("challenge") { t.Fatal("challenge does not match verifier") }
	manager.Complete(flowID,"vd-account")
	status, ok := manager.Status(flowID)
	if !ok || status.Status != "complete" || status.AccountID != "vd-account" { t.Fatalf("bad status: %#v",status) }
}

func TestCallbackRejectsBindingMismatch(t *testing.T) {
	manager := New(Config{}, nil)
	start, err := manager.Start("http://127.0.0.1:5084")
	if err != nil { t.Fatal(err) }
	authURL, _ := url.Parse(start.AuthURL)
	callbackURL, _ := url.Parse(authURL.Query().Get("callback"))
	q := callbackURL.Query()
	q.Set("state", authURL.Query().Get("state"))
	q.Set("code","x")
	q.Set("nonce","wrong")
	_,_,err = manager.ExchangeCallback(context.Background(),q)
	if err == nil || !strings.Contains(err.Error(),"binding") { t.Fatalf("expected binding error, got %v",err) }
}

package protocol

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseCatalog(t *testing.T) {
	body := []byte(`{"data":{"model_config":[{"key":"deepseek-v4.1-flash-free","label":"DeepSeek V4.1 Flash","provider":["deepseek"],"contextWindows":[{"contextWindowDisplay":"300K","contextWindowTokens":300000},{"contextWindowDisplay":"1M","contextWindowTokens":1048576}],"default_max_output_tokens":64000,"supportsImages":true,"supports_thinking":true,"effortLevels":[{"label":"Low"},{"label":"High"}],"is_limit_free":true,"costMultiplier":0},{"key":"hidden","is_hidden":true}]}}`)
	models, err := ParseCatalog(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 {
		t.Fatalf("expected one visible model, got %d", len(models))
	}
	m := models[0]
	if m.ID != "deepseek-v4.1-flash-free" || m.Family != "deepseek" {
		t.Fatalf("bad model: %#v", m)
	}
	if len(m.ContextWindows) != 2 || m.ContextWindows[1].Tokens != 1048576 {
		t.Fatalf("bad context windows: %#v", m.ContextWindows)
	}
	if !m.SupportsThinking || !m.SupportsImages || len(m.EffortLevels) != 2 {
		t.Fatalf("missing capabilities: %#v", m)
	}
}

func TestDiscoverModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("missing auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"model_config":[{"key":"m-free","label":"M","contextWindow":"300K","is_limit_free":true}]}}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{
		Endpoint: server.URL, CatalogEndpoint: server.URL,
		AppVersion: "2.test", BetaHeader: "beta",
		Sign: "protocol-sign-for-test-only", DeviceID: "dev",
	}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	models, err := client.DiscoverModels(context.Background(), "token")
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || len(models[0].ContextWindows) != 1 || models[0].ContextWindows[0].Tokens != 300000 {
		t.Fatalf("unexpected models: %#v", models)
	}
}

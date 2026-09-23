package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mrgolftech/Verdent/internal/account"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

func TestValidateBenchmarkRequestDefaults(t *testing.T) {
	input := benchmarkRequest{Model: "gpt-test", Prompt: "hello"}
	if err := validateBenchmarkRequest(&input); err != nil {
		t.Fatal(err)
	}
	if input.Concurrency != 1 {
		t.Fatalf("expected default concurrency 1, got %d", input.Concurrency)
	}
	if input.MaxOutputTokens != 512 {
		t.Fatalf("expected default max output 512, got %d", input.MaxOutputTokens)
	}

	input = benchmarkRequest{Model: "gpt-test", Prompt: "hello", Concurrency: 65}
	if err := validateBenchmarkRequest(&input); err == nil {
		t.Fatal("expected concurrency limit error")
	}
	input = benchmarkRequest{Model: "gpt-test", Prompt: "hello", ContextWindowTokens: 2_000_001}
	if err := validateBenchmarkRequest(&input); err == nil {
		t.Fatal("expected context window limit error")
	}
}

func TestEstimateGeneratedTokens(t *testing.T) {
	if got := estimateGeneratedTokens("测试中文"); got != 4 {
		t.Fatalf("expected four CJK token estimates, got %d", got)
	}
	if got := estimateGeneratedTokens("abcdefgh"); got != 2 {
		t.Fatalf("expected two ASCII token estimates, got %d", got)
	}
}

func TestAdminBenchmarkRequiresSession(t *testing.T) {
	s := New(account.NewRouter(nil), protocol.Config{})
	s.AdminPassword = "secret"

	req := httptest.NewRequest(http.MethodPost, "/api/benchmark/chat", strings.NewReader(`{"model":"m","prompt":"hi"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

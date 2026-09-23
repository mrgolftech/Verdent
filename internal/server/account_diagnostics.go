package server

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mrgolftech/Verdent/internal/account"
	"github.com/mrgolftech/Verdent/internal/canonical"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

type accountDiagnosticStep struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Model  string `json:"model,omitempty"`
}

type accountDiagnostics struct {
	Credential    accountDiagnosticStep `json:"credential"`
	Catalog       accountDiagnosticStep `json:"catalog"`
	FreeInference accountDiagnosticStep `json:"free_inference"`
}

type accountDiagnosticResult struct {
	OK          bool               `json:"ok"`
	AccountID   string             `json:"account_id"`
	State       string             `json:"state"`
	Models      int                `json:"models"`
	Diagnostics accountDiagnostics `json:"diagnostics"`
}

func (s *Server) diagnoseAccount(ctx context.Context, selected *account.Account) accountDiagnosticResult {
	result := accountDiagnosticResult{AccountID: selected.Credential.ID}
	result.Diagnostics.Credential = accountDiagnosticStep{Status: "ok", Detail: "credential loaded"}

	if selected.Credential.TokenExpiresAt > 0 && time.Now().Unix() >= selected.Credential.TokenExpiresAt {
		result.Diagnostics.Credential = accountDiagnosticStep{Status: "error", Detail: "access token expired"}
		result.State = currentAccountState(s.Accounts, selected.Credential.ID)
		return result
	}

	// Diagnostics deliberately use no automatic retry: one operator-initiated
	// probe should produce one upstream attempt and surface the real account state.
	cfg := s.ProtocolConfig
	cfg.RetryDelays = nil
	client, err := s.NewClient(*selected, cfg, 30*time.Second)
	if err != nil {
		result.Diagnostics.Catalog = accountDiagnosticStep{Status: "error", Detail: err.Error()}
		result.State = currentAccountState(s.Accounts, selected.Credential.ID)
		return result
	}

	models, err := client.DiscoverModels(ctx, selected.Credential.Token)
	if err != nil {
		raw := err.Error()
		status := "error"
		if protocol.IsAccountSuspended(raw) {
			s.Accounts.MarkSuspended(selected.Credential.ID, raw)
			status = "suspended"
		}
		result.Diagnostics.Catalog = accountDiagnosticStep{Status: status, Detail: raw}
		result.State = currentAccountState(s.Accounts, selected.Credential.ID)
		return result
	}
	result.Models = len(models)
	result.Diagnostics.Catalog = accountDiagnosticStep{Status: "ok", Detail: fmt.Sprintf("%d models", len(models))}

	model := diagnosticFreeModel(models)
	if model == "" {
		result.Diagnostics.FreeInference = accountDiagnosticStep{Status: "skipped", Detail: "no free model discovered"}
		result.OK = true
		result.State = currentAccountState(s.Accounts, selected.Credential.ID)
		return result
	}

	probe := canonical.Request{
		Model: model,
		Messages: []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.ContentBlock{{Type: canonical.BlockText, Text: "Reply with OK."}},
		}},
		MaxTokens: 8,
		UserQuery: "Reply with OK.",
	}
	resp, err := client.Do(ctx, selected.Credential.Token, probe, protocol.EnvelopeOptions{IDs: requestIDs()})
	if err != nil {
		result.Diagnostics.FreeInference = accountDiagnosticStep{Status: "error", Detail: err.Error(), Model: model}
		result.State = currentAccountState(s.Accounts, selected.Credential.ID)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		raw := strings.TrimSpace(string(body))
		if raw == "" {
			raw = http.StatusText(resp.StatusCode)
		}
		code := s.recordUpstreamFailure(selected.Credential.ID, resp.StatusCode, raw, resp.Header.Get("Retry-After"))
		status := "error"
		if code == "verdent_account_suspended" {
			status = "suspended"
		} else if code == "verdent_rate_limit" {
			status = "cooling_down"
		}
		result.Diagnostics.FreeInference = accountDiagnosticStep{Status: status, Detail: raw, Model: model}
		result.State = currentAccountState(s.Accounts, selected.Credential.ID)
		return result
	}

	scanner := protocol.NewSSEScanner(resp.Body)
	decoder := protocol.NewStreamDecoder()
	for {
		frame, scanErr := scanner.NextFrame()
		if scanErr == io.EOF {
			break
		}
		if scanErr != nil {
			result.Diagnostics.FreeInference = accountDiagnosticStep{Status: "error", Detail: scanErr.Error(), Model: model}
			result.State = currentAccountState(s.Accounts, selected.Credential.ID)
			return result
		}
		events, decodeErr := decoder.Decode(frame)
		if decodeErr == protocol.ErrUnsupportedEvent {
			continue
		}
		if decodeErr != nil {
			result.Diagnostics.FreeInference = accountDiagnosticStep{Status: "error", Detail: decodeErr.Error(), Model: model}
			result.State = currentAccountState(s.Accounts, selected.Credential.ID)
			return result
		}
		for _, event := range events {
			if event.Type != protocol.EventError {
				continue
			}
			code := s.recordUpstreamFailure(selected.Credential.ID, 0, event.Err, "")
			status := "error"
			if code == "verdent_account_suspended" {
				status = "suspended"
			} else if code == "verdent_rate_limit" {
				status = "cooling_down"
			}
			result.Diagnostics.FreeInference = accountDiagnosticStep{Status: status, Detail: event.Err, Model: model}
			result.State = currentAccountState(s.Accounts, selected.Credential.ID)
			return result
		}
	}

	result.Diagnostics.FreeInference = accountDiagnosticStep{Status: "ok", Detail: "free inference accepted", Model: model}
	result.OK = true
	result.State = currentAccountState(s.Accounts, selected.Credential.ID)
	return result
}

func diagnosticFreeModel(models []protocol.ModelInfo) string {
	for _, model := range models {
		if model.IsLimitFree {
			return model.ID
		}
	}
	for _, model := range models {
		if strings.Contains(strings.ToLower(model.ID), "free") {
			return model.ID
		}
	}
	return ""
}

func currentAccountState(router *account.Router, id string) string {
	if router == nil {
		return ""
	}
	for _, item := range router.Snapshot() {
		if item.Credential.ID == id {
			return string(item.State)
		}
	}
	return ""
}

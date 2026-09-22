package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResponsesNonStreamFunctionCall(t *testing.T) {
	s, upstream := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":6}}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_start\",\"index\":1,\"content_block\":{\"type\":\"tool_use\",\"id\":\"call_1\",\"name\":\"read_file\",\"input\":{}}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"index\":1,\"delta\":{\"partial_json\":\"{\\\"path\\\":\\\"/tmp/a\\\"}\"}}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_stop\",\"index\":1}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\",\"usage\":{\"output_tokens\":2}}\n\n")
	}))
	defer upstream.Close()

	api := httptest.NewServer(s.Handler())
	defer api.Close()

	body := `{
		"model":"gpt-5.6-luna",
		"input":"read the file",
		"tools":[
			{"type":"function","name":"read_file","description":"Read a file","parameters":{"type":"object","properties":{"path":{"type":"string"}}}}
		],
		"tool_choice":"auto"
	}`
	resp, err := http.Post(api.URL+"/v1/responses", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
	}

	var got struct {
		Object string `json:"object"`
		Status string `json:"status"`
		Output []map[string]any `json:"output"`
		Usage struct {
			InputTokens int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Object != "response" || got.Status != "completed" {
		t.Fatalf("bad response shell: %#v", got)
	}
	if len(got.Output) != 1 || got.Output[0]["type"] != "function_call" {
		t.Fatalf("bad output: %#v", got.Output)
	}
	if got.Output[0]["name"] != "read_file" || got.Output[0]["arguments"] != `{"path":"/tmp/a"}` {
		t.Fatalf("bad function call: %#v", got.Output[0])
	}
	if got.Usage.TotalTokens != 8 {
		t.Fatalf("bad usage: %#v", got.Usage)
	}
}

func TestResponsesStreamingCustomApplyPatch(t *testing.T) {
	s, upstream := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: {\"type\":\"message_start\"}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_start\",\"index\":2,\"content_block\":{\"type\":\"tool_use\",\"id\":\"patch_1\",\"name\":\"apply_patch\",\"input\":{}}}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_delta\",\"index\":2,\"delta\":{\"partial_json\":\"{\\\"input\\\":\\\"*** Begin Patch\\n*** End Patch\\\"}\"}}\n\n")
		flusher.Flush()
		_, _ = io.WriteString(w, "data: {\"type\":\"content_block_stop\",\"index\":2}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"message_stop\",\"usage\":{\"output_tokens\":1}}\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	api := httptest.NewServer(s.Handler())
	defer api.Close()

	body := `{
		"model":"gpt-5.6-luna",
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"patch it"}]}],
		"tools":[
			{"type":"custom","name":"apply_patch","description":"Apply a patch","format":{"type":"grammar","syntax":"lark","definition":"start: /.+/"}}
		],
		"tool_choice":"auto",
		"stream":true
	}`
	resp, err := http.Post(api.URL+"/v1/responses", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status=%d body=%s", resp.StatusCode, raw)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("bad content type %q", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	var eventNames []string
	var dataLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			eventNames = append(eventNames, strings.TrimPrefix(line, "event: "))
		}
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimPrefix(line, "data: "))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	names := strings.Join(eventNames, "\n")
	for _, want := range []string{
		"response.created",
		"response.output_item.added",
		"response.custom_tool_call_input.delta",
		"response.custom_tool_call_input.done",
		"response.output_item.done",
		"response.completed",
	} {
		if !strings.Contains(names, want) {
			t.Fatalf("missing event %q in %s", want, names)
		}
	}
	data := strings.Join(dataLines, "\n")
	if !strings.Contains(data, `"input":"*** Begin Patch\n*** End Patch"`) {
		t.Fatalf("custom tool input not preserved: %s", data)
	}
}

func TestResponsesRejectsPreviousResponseID(t *testing.T) {
	s, upstream := testServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("upstream must not be called for unsupported previous_response_id")
	}))
	defer upstream.Close()

	api := httptest.NewServer(s.Handler())
	defer api.Close()

	body := `{"model":"m","input":"hello","previous_response_id":"resp_old"}`
	resp, err := http.Post(api.URL+"/v1/responses", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 400, got %d: %s", resp.StatusCode, raw)
	}
}

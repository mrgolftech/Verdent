package responses

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mrgolftech/Verdent/internal/canonical"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

func TestToCanonicalCodexFunctionAndApplyPatch(t *testing.T) {
	input := json.RawMessage(`[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"edit it"}]},
		{"type":"function_call","call_id":"call_1","name":"read_file","arguments":"{\"path\":\"/a\"}"},
		{"type":"function_call_output","call_id":"call_1","output":"old"},
		{"type":"custom_tool_call","call_id":"call_2","name":"apply_patch","input":"*** Begin Patch\n*** End Patch"},
		{"type":"custom_tool_call_output","call_id":"call_2","output":"Done!"}
	]`)
	req := Request{
		Model:        "gpt-5.6-luna",
		Input:        input,
		Instructions: "be precise",
		Tools: []Tool{
			{Type: "function", Name: "read_file", Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
			{Type: "custom", Name: "apply_patch", Format: json.RawMessage(`{"type":"grammar","syntax":"lark","definition":"start: /.+/"}`)},
		},
		ToolChoice: json.RawMessage(`"auto"`),
		Reasoning:  &Reasoning{Effort: "high"},
	}
	got, err := ToCanonical(req)
	if err != nil {
		t.Fatal(err)
	}
	if got.Request.Effort != "high" || len(got.Request.Tools) != 2 || !got.CustomTools["apply_patch"] {
		t.Fatalf("bad conversion: %#v", got)
	}
	if got.Request.Tools[1].Kind != canonical.ToolKindCustom {
		t.Fatalf("custom tool kind lost: %#v", got.Request.Tools[1])
	}
	if len(got.Request.Messages) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(got.Request.Messages))
	}
	if got.Request.Messages[3].Content[0].ToolName != "apply_patch" {
		t.Fatalf("custom history lost: %#v", got.Request.Messages[3])
	}
}

func TestRejectsUnsupportedCustomTool(t *testing.T) {
	_, err := ToCanonical(Request{
		Model: "m",
		Input: json.RawMessage(`"hi"`),
		Tools: []Tool{{Type: "custom", Name: "exec"}},
	})
	if err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("expected explicit unsupported error, got %v", err)
	}
}

func TestAssemblerOutputsCustomAndFunctionCalls(t *testing.T) {
	a := NewAssembler(map[string]bool{"apply_patch": true})
	for _, ev := range []protocol.Event{
		{Type: protocol.EventThinking, Text: "plan"},
		{Type: protocol.EventToolStart, Index: 1, ToolID: "c1", ToolName: "read_file"},
		{Type: protocol.EventToolDelta, Index: 1, ArgumentDelta: `{"path":"/a"}`},
		{Type: protocol.EventToolStart, Index: 2, ToolID: "c2", ToolName: "apply_patch"},
		{Type: protocol.EventToolDelta, Index: 2, ArgumentDelta: `{"input":"*** Begin Patch"}`},
		{Type: protocol.EventTextDelta, Text: "done"},
		{Type: protocol.EventUsage, Usage: &protocol.Usage{InputTokens: 4, OutputTokens: 2}},
	} {
		a.Consume(ev)
	}
	resp := a.Response("resp_1", "m", 1)
	if len(resp.Output) != 4 || resp.OutputText != "done" || resp.Usage.TotalTokens != 6 {
		t.Fatalf("bad response: %#v", resp)
	}
	custom := resp.Output[2].(map[string]any)
	if custom["type"] != "custom_tool_call" || custom["input"] != "*** Begin Patch" {
		t.Fatalf("bad custom output: %#v", custom)
	}
}

func TestStreamEncoderFunctionLifecycle(t *testing.T) {
	s := NewStreamEncoder("resp_1", "m", 1, nil)
	var all []Event
	for _, ev := range []protocol.Event{
		{Type: protocol.EventMessageStart},
		{Type: protocol.EventToolStart, Index: 4, ToolID: "c1", ToolName: "read_file"},
		{Type: protocol.EventToolDelta, Index: 4, ArgumentDelta: `{"path":"/a"}`},
		{Type: protocol.EventToolEnd, Index: 4},
		{Type: protocol.EventUsage, Usage: &protocol.Usage{InputTokens: 3, OutputTokens: 1}},
		{Type: protocol.EventMessageEnd},
	} {
		all = append(all, s.Encode(ev)...)
	}

	var names []string
	for _, event := range all {
		names = append(names, event.Type)
	}
	joined := strings.Join(names, "\n")
	for _, want := range []string{
		"response.created",
		"response.output_item.added",
		"response.function_call_arguments.delta",
		"response.function_call_arguments.done",
		"response.output_item.done",
		"response.completed",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
	last := all[len(all)-1]
	if last.Response == nil || last.Response.Usage.TotalTokens != 4 {
		t.Fatalf("bad completed response: %#v", last)
	}
}

func TestStreamEncoderCustomApplyPatchLifecycle(t *testing.T) {
	s := NewStreamEncoder("resp_1", "gpt-5.6-luna", 1, map[string]bool{"apply_patch": true})
	var all []Event
	for _, ev := range []protocol.Event{
		{Type: protocol.EventToolStart, Index: 2, ToolID: "patch_1", ToolName: "apply_patch"},
		{Type: protocol.EventToolDelta, Index: 2, ArgumentDelta: `{"input":"*** Begin Patch"}`},
		{Type: protocol.EventToolEnd, Index: 2},
		{Type: protocol.EventMessageEnd},
	} {
		all = append(all, s.Encode(ev)...)
	}
	var sawDelta, sawDone bool
	for _, event := range all {
		if event.Type == "response.custom_tool_call_input.delta" {
			sawDelta = true
		}
		if event.Type == "response.custom_tool_call_input.done" && event.Input == "*** Begin Patch" {
			sawDone = true
		}
	}
	if !sawDelta || !sawDone {
		t.Fatalf("missing custom lifecycle: %#v", all)
	}
}

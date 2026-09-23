package openai

import (
	"encoding/json"
	"testing"

	"github.com/mrgolftech/Verdent/internal/canonical"
)

func TestToCanonicalToolsAndHistory(t *testing.T) {
	req:=ChatRequest{
		Model:"gpt-5.6-luna-free",MaxCompletionTokens:4096,ReasoningEffort:"high",ContextWindowTokens:114688,
		Messages:[]Message{
			{Role:"system",Content:json.RawMessage(`"be precise"`)},
			{Role:"user",Content:json.RawMessage(`[{"type":"text","text":"read file"},{"type":"image_url","image_url":{"url":"https://example.test/a.png"}}]`)},
			{Role:"assistant",Content:json.RawMessage(`null`),ToolCalls:[]ToolCall{{ID:"call_1",Type:"function",Function:FunctionCall{Name:"read_file",Arguments:`{"path":"/tmp/a"}`}}}},
			{Role:"tool",ToolCallID:"call_1",Content:json.RawMessage(`"hello"`)},
		},
		Tools:[]Tool{{Type:"function",Function:FunctionDef{Name:"read_file",Description:"Read a file",Parameters:json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)}}},
		ToolChoice:json.RawMessage(`{"type":"function","function":{"name":"read_file"}}`),
	}
	got,err:=ToCanonical(req); if err!=nil { t.Fatal(err) }
	if got.MaxTokens!=4096 || got.Effort!="high" || got.ContextWindowTokens!=114688 || len(got.System)!=1 || len(got.Tools)!=1 { t.Fatalf("bad canonical request: %#v",got) }
	if got.ToolChoice==nil || got.ToolChoice.Type!=canonical.ToolChoiceTool || got.ToolChoice.Name!="read_file" { t.Fatalf("bad tool choice: %#v",got.ToolChoice) }
	if len(got.Messages)!=3 { t.Fatalf("expected 3 conversation messages, got %d",len(got.Messages)) }
	if got.Messages[1].Content[0].Type!=canonical.BlockToolUse { t.Fatalf("assistant tool call not preserved: %#v",got.Messages[1]) }
	if got.Messages[2].Content[0].Type!=canonical.BlockToolResult || got.Messages[2].Content[0].Text!="hello" { t.Fatalf("tool result not preserved: %#v",got.Messages[2]) }
}

func TestRequiredToolChoiceMapsToAny(t *testing.T) {
	req:=ChatRequest{Model:"m",Messages:[]Message{{Role:"user",Content:json.RawMessage(`"hi"`)}},ToolChoice:json.RawMessage(`"required"`)}
	got,err:=ToCanonical(req); if err!=nil {t.Fatal(err)}
	if got.ToolChoice==nil || got.ToolChoice.Type!=canonical.ToolChoiceAny { t.Fatalf("bad required mapping: %#v",got.ToolChoice) }
}

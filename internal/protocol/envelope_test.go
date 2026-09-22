package protocol

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
	"strings"

	"github.com/mrgolftech/Verdent/internal/canonical"
)

func TestBuildEnvelopeNativeTools(t *testing.T) {
	codec, err := newCodec("protocol-sign-for-test-only", bytes.NewReader(bytes.Repeat([]byte{0x33}, nonceSize*3)))
	if err != nil { t.Fatal(err) }
	temp := 0.5
	req := canonical.Request{
		Model:"gpt-5.6-luna-free",
		System:[]canonical.ContentBlock{{Type:canonical.BlockText, Text:"system"}},
		Messages:[]canonical.Message{
			{Role:canonical.RoleUser, Content:[]canonical.ContentBlock{{Type:canonical.BlockText, Text:"read it"}}},
			{Role:canonical.RoleAssistant, Content:[]canonical.ContentBlock{{Type:canonical.BlockToolUse, ToolID:"t1", ToolName:"read_file", Input:json.RawMessage(`{"path":"/a"}`)}}},
			{Role:canonical.RoleUser, Content:[]canonical.ContentBlock{{Type:canonical.BlockToolResult, ToolID:"t1", Text:"ok"}}},
		},
		Tools:[]canonical.Tool{
			{Name:"read_file", InputSchema:json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)},
			{Name:"apply_patch", InputSchema:json.RawMessage(`{"type":"object","properties":{}}`)},
		},
		ToolChoice:&canonical.ToolChoice{Type:canonical.ToolChoiceAuto},
		Temperature:&temp, MaxTokens:2048, Effort:"high", ContextWindowTokens:300000, UserQuery:"read it",
	}
	cfg:=Config{AppVersion:"2.test",BetaHeader:"beta",Sign:"protocol-sign-for-test-only",DeviceID:"dev"}
	env, err := BuildEnvelope(req,cfg,codec,EnvelopeOptions{IDs:RequestIDs{SessionID:"session_1",ConvID:"conv_1",ReactID:"model_agent_1"}})
	if err != nil { t.Fatal(err) }
	if !env.Stream || env.MaxTokens != 2048 || env.Temperature != 0.5 { t.Fatalf("bad envelope basics: %#v", env) }
	if env.ToolChoice["type"] != "auto" { t.Fatalf("bad tool choice: %#v", env.ToolChoice) }
	if env.Effort != "high" || env.ContextWindowTokens != 300000 { t.Fatalf("missing reasoning/context settings: %#v", env) }

	toolBytes, err := codec.Decode(env.Tools)
	if err != nil { t.Fatal(err) }
	var tools []map[string]any
	if err := json.Unmarshal(toolBytes,&tools); err != nil { t.Fatal(err) }
	if len(tools) != 2 { t.Fatalf("expected 2 tools, got %d", len(tools)) }
	if tools[1]["type"] != "apply_patch" { t.Fatalf("expected native apply_patch, got %#v", tools[1]) }

	msgBytes, err := codec.Decode(env.Messages)
	if err != nil { t.Fatal(err) }
	var messages []map[string]any
	if err := json.Unmarshal(msgBytes,&messages); err != nil { t.Fatal(err) }
	if len(messages) != 3 { t.Fatalf("expected 3 normalized messages, got %d", len(messages)) }
}

func TestNormalizeMessagesAddsContinuationAfterAssistantText(t *testing.T) {
	in := []canonical.Message{{Role:canonical.RoleAssistant,Content:[]canonical.ContentBlock{{Type:canonical.BlockText,Text:"partial"}}}}
	out := normalizeMessages(in)
	if len(out) != 2 || out[1].Role != canonical.RoleUser || out[1].Content[0].Text != trailingContinuationText {
		t.Fatalf("unexpected continuation normalization: %#v", out)
	}
}


func TestCapturedDesktopSystemFoldsClientSystemIntoUserMessage(t *testing.T) {
	codec, err := newCodec("protocol-sign-for-test-only", bytes.NewReader(bytes.Repeat([]byte{0x44}, nonceSize*2)))
	if err != nil { t.Fatal(err) }
	now := time.Date(2026,9,22,21,30,0,0,time.FixedZone("CST",8*3600))
	req := canonical.Request{
		Model:"deepseek-v4.1-flash-free",
		System:[]canonical.ContentBlock{{Type:canonical.BlockText,Text:"client system rule"}},
		Messages:[]canonical.Message{
			{Role:canonical.RoleUser,Content:[]canonical.ContentBlock{{Type:canonical.BlockText,Text:"hello"}}},
			{Role:canonical.RoleAssistant,Content:[]canonical.ContentBlock{{Type:canonical.BlockText,Text:"hi"}}},
		},
	}
	cfg := Config{
		AppVersion:"2.15.1",BetaHeader:"hybrid-stream@20250919",Sign:"protocol-sign-for-test-only",DeviceID:"dev",
		SystemCiphertext:"captured-system-ciphertext",ModelCatalogVersion:"model-catalog-test",NativeAPI:false,
	}
	env, err := BuildEnvelope(req,cfg,codec,EnvelopeOptions{
		IDs:RequestIDs{SessionID:"s",ConvID:"c",ReactID:"r"},Now:now,
	})
	if err != nil { t.Fatal(err) }
	if env.System != "captured-system-ciphertext" { t.Fatalf("captured system changed: %q",env.System) }
	if env.NativeAPI { t.Fatal("native_api should remain false for current Desktop shape") }
	if env.ModelCatalogVersion != "model-catalog-test" { t.Fatalf("catalog version=%q",env.ModelCatalogVersion) }
	if env.IsFree || env.IsLimitFree { t.Fatalf("desktop capture flags should default false: %#v",env) }

	raw, err := codec.Decode(env.Messages)
	if err != nil { t.Fatal(err) }
	var messages []map[string]any
	if err := json.Unmarshal(raw,&messages); err != nil { t.Fatal(err) }
	if len(messages)!=2 { t.Fatalf("messages=%#v",messages) }

	firstContent, _ := messages[0]["content"].([]any)
	if len(firstContent)<3 { t.Fatalf("first content=%#v",firstContent) }
	ts, _ := firstContent[0].(map[string]any)
	if got, _ := ts["text"].(string); got != "<timestamp>Tue Sep 22 2026 21:30:00 GMT+0800</timestamp>\n" {
		t.Fatalf("timestamp=%q",got)
	}
	systemBlock, _ := firstContent[1].(map[string]any)
	if got, _ := systemBlock["text"].(string); !strings.Contains(got,"<system>") || !strings.Contains(got,"client system rule") {
		t.Fatalf("system was not folded into user turn: %#v",systemBlock)
	}
	if _, ok := messages[0]["model"]; ok { t.Fatal("user message must not carry model") }
	if messages[1]["model"] != req.Model { t.Fatalf("assistant model missing: %#v",messages[1]) }
	lastContent, _ := messages[1]["content"].([]any)
	lastBlock, _ := lastContent[len(lastContent)-1].(map[string]any)
	cc, _ := lastBlock["cache_control"].(map[string]any)
	if cc["type"] != "ephemeral" { t.Fatalf("missing final cache_control: %#v",lastBlock) }
}

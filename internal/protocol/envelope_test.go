package protocol

import (
	"bytes"
	"encoding/json"
	"testing"

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

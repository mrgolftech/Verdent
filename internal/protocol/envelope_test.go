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
	if len(messages)!=3 { t.Fatalf("messages=%#v",messages) }

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
	if _, ok := messages[2]["model"]; ok { t.Fatal("continuation user message must not carry model") }
	lastContent, _ := messages[2]["content"].([]any)
	lastBlock, _ := lastContent[len(lastContent)-1].(map[string]any)
	cc, _ := lastBlock["cache_control"].(map[string]any)
	if cc["type"] != "ephemeral" { t.Fatalf("missing final cache_control: %#v",lastBlock) }
}


func Test2151TemplateFieldParity(t *testing.T) {
	codec, err := newCodec("protocol-sign-for-test-only", bytes.NewReader(bytes.Repeat([]byte{0x55}, nonceSize*2)))
	if err != nil { t.Fatal(err) }
	temp:=1.0
	cfg:=Config{
		AppVersion:"2.15.1",BetaHeader:"hybrid-stream@20250919",Sign:"protocol-sign-for-test-only",DeviceID:"dev",
		Channel:"deck",AgentName:"VerdentDeck",SystemCiphertext:"captured-system",Thinking:&Thinking{Type:"enabled",BudgetTokens:4000},
		Effort:"high",MaxTokens:64000,Temperature:&temp,ModelCatalogVersion:"model-catalog-1790068751126",
		Environment:Environment{Platform:"win32",OSVersion:"Windows_NT 10.0.26200",Shell:"gitbash"},
		TraceTags:[]string{},NativeAPI:false,IsEco:false,IsAuto:false,IsFree:false,IsLimitFree:false,
	}
	req:=canonical.Request{
		Model:"deepseek-v4.1-flash-free",UserQuery:"hello",
		Messages:[]canonical.Message{{Role:canonical.RoleUser,Content:[]canonical.ContentBlock{{Type:canonical.BlockText,Text:"hello"}}}},
	}
	now:=time.Date(2026,9,23,8,0,0,0,time.FixedZone("CST",8*3600))
	env,err:=BuildEnvelope(req,cfg,codec,EnvelopeOptions{IDs:RequestIDs{SessionID:"session_a",ConvID:"conv_b",ReactID:"model_agent_c"},Now:now})
	if err!=nil{t.Fatal(err)}
	raw,err:=json.Marshal(env);if err!=nil{t.Fatal(err)}
	var body map[string]any
	if err:=json.Unmarshal(raw,&body);err!=nil{t.Fatal(err)}
	required:=[]string{
		"channel","model","session_id","conv_id","react_id","react_type","stream",
		"max_tokens","temperature","system","thinking","messages","agent_name","env","encrypt",
		"custom_trace_tags_tmp","custom_trace_metadata_tmp","model_catalog_version","effort",
		"is_eco","is_auto","is_free","is_limit_free","native_api",
	}
	for _,key:=range required {
		if _,ok:=body[key];!ok { t.Fatalf("missing 2.15.1/OpenFork request field %q: %s",key,string(raw)) }
	}
	if body["channel"]!="deck" || body["agent_name"]!="VerdentDeck" || body["max_tokens"]!=float64(64000) || body["temperature"]!=float64(1) {
		t.Fatalf("captured defaults not preserved: %s",raw)
	}
	if body["system"]!="captured-system" { t.Fatalf("system fingerprint changed: %q",body["system"]) }
	if _,ok:=body["tools"];ok { t.Fatal("tools should be omitted when client supplied none") }
	if _,ok:=body["tool_choice"];ok { t.Fatal("tool_choice should be omitted when client supplied no tools") }
	if _,ok:=body["context_window_tokens"];ok { t.Fatal("context_window_tokens should be omitted unless supplied") }
	envObj:=body["env"].(map[string]any)
	if envObj["platform"]!="win32" || envObj["today_date"]!="2026-09-23" { t.Fatalf("env=%#v",envObj) }
	thinking:=body["thinking"].(map[string]any)
	if thinking["type"]!="enabled" || thinking["budget_tokens"]!=float64(4000) { t.Fatalf("thinking=%#v",thinking) }
}

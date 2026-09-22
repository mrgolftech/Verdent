package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mrgolftech/Verdent/internal/canonical"
)

type RequestIDs struct {
	SessionID string
	ConvID    string
	ReactID   string
}

type Environment struct {
	Platform  string `json:"platform,omitempty"`
	OSVersion string `json:"os_version,omitempty"`
	Shell     string `json:"shell,omitempty"`
	TodayDate string `json:"today_date,omitempty"`
}

type TraceMetadata struct {
	SelectedModelID          string `json:"selected_model_id"`
	EffectiveModelID         string `json:"effective_model_id"`
	ImageRouteSelectedIsBYOK bool   `json:"image_route_selected_model_is_byok"`
	CWD                      string `json:"cwd,omitempty"`
	ConversationScene        string `json:"conversation_scene"`
	ConversationPromptSource string `json:"conversation_prompt_source"`
	ActionType               string `json:"action_type"`
	DeviceType               string `json:"device_type"`
	OSType                   string `json:"os_type"`
	UserQuery                string `json:"user_query,omitempty"`
}

type EnvelopeOptions struct {
	IDs                 RequestIDs
	Environment         Environment
	Trace               TraceMetadata
	ModelCatalogVersion string
	Now                  time.Time
}

type Envelope struct {
	Channel              string         `json:"channel"`
	Model                string         `json:"model"`
	SessionID            string         `json:"session_id"`
	ConvID               string         `json:"conv_id"`
	ReactID              string         `json:"react_id"`
	ReactType            string         `json:"react_type"`
	Stream               bool           `json:"stream"`
	MaxTokens            int            `json:"max_tokens"`
	Temperature          float64        `json:"temperature"`
	System               string         `json:"system"`
	Thinking             *Thinking      `json:"thinking,omitempty"`
	Tools                string         `json:"tools,omitempty"`
	ToolChoice           map[string]any `json:"tool_choice,omitempty"`
	Messages             string         `json:"messages"`
	AgentName            string         `json:"agent_name"`
	Env                  Environment    `json:"env"`
	Encrypt              bool           `json:"encrypt"`
	TraceTags            []string       `json:"custom_trace_tags_tmp"`
	TraceMetadata        TraceMetadata  `json:"custom_trace_metadata_tmp"`
	ModelCatalogVersion  string         `json:"model_catalog_version,omitempty"`
	Effort               string         `json:"effort,omitempty"`
	ContextWindowTokens  int            `json:"context_window_tokens,omitempty"`
	IsEco                bool           `json:"is_eco"`
	IsAuto               bool           `json:"is_auto"`
	IsFree               bool           `json:"is_free"`
	IsLimitFree          bool           `json:"is_limit_free"`
	NativeAPI            bool           `json:"native_api"`
}

func BuildEnvelope(req canonical.Request, cfg Config, codec *Codec, opt EnvelopeOptions) (Envelope, error) {
	cfg = cfg.WithDefaults()
	if err := cfg.Validate(); err != nil { return Envelope{}, err }
	if codec == nil { return Envelope{}, errors.New("codec is required") }
	if req.Model == "" { return Envelope{}, errors.New("model is required") }
	if opt.IDs.SessionID == "" || opt.IDs.ConvID == "" || opt.IDs.ReactID == "" {
		return Envelope{}, errors.New("session, conversation and reaction IDs are required")
	}

	systemBlocks := append([]canonical.ContentBlock(nil), req.System...)
	systemBlocks = append(systemBlocks, cfg.SystemTrailer...)
	messages := normalizeMessages(req.Messages)

	var encSystem string
	var err error
	if strings.TrimSpace(cfg.SystemCiphertext) != "" {
		// Current Verdent production behavior fingerprints the encrypted Desktop
		// system field. Keep that captured ciphertext byte-for-byte and move the
		// downstream client's system instructions into the first user turn.
		encSystem = cfg.SystemCiphertext
		messages = foldSystemIntoMessages(systemBlocks, messages)
	} else {
		encSystem, err = codec.EncodeJSON(systemBlocks)
		if err != nil { return Envelope{}, fmt.Errorf("encode system: %w", err) }
	}

	now := opt.Now
	if now.IsZero() { now = time.Now() }
	encMessages, err := codec.EncodeJSON(toUpstreamMessages(messages, req.Model, now))
	if err != nil { return Envelope{}, fmt.Errorf("encode messages: %w", err) }

	maxTokens := req.MaxTokens
	if maxTokens <= 0 { maxTokens = 64000 }
	temperature := 1.0
	if req.Temperature != nil { temperature = *req.Temperature }

	trace := opt.Trace
	trace.SelectedModelID = req.Model
	trace.EffectiveModelID = req.Model
	if trace.ConversationScene == "" { trace.ConversationScene = "worker" }
	if trace.ConversationPromptSource == "" { trace.ConversationPromptSource = "user" }
	if trace.ActionType == "" { trace.ActionType = "text" }
	if trace.DeviceType == "" { trace.DeviceType = cfg.DeviceType }
	if trace.OSType == "" { trace.OSType = cfg.OSType }
	if trace.UserQuery == "" { trace.UserQuery = req.UserQuery }

	modelCatalogVersion := strings.TrimSpace(opt.ModelCatalogVersion)
	if modelCatalogVersion == "" {
		modelCatalogVersion = strings.TrimSpace(cfg.ModelCatalogVersion)
	}

	effort := strings.TrimSpace(req.Effort)
	if effort == "" { effort = strings.TrimSpace(cfg.Effort) }
	env := Envelope{
		Channel: cfg.Channel, Model: req.Model, SessionID: opt.IDs.SessionID, ConvID: opt.IDs.ConvID, ReactID: opt.IDs.ReactID,
		ReactType: cfg.ReactType, Stream: true, MaxTokens: maxTokens, Temperature: temperature,
		System: encSystem, Thinking: cfg.Thinking, Messages: encMessages, AgentName: cfg.AgentName, Env: opt.Environment, Encrypt: true,
		TraceTags: []string{}, TraceMetadata: trace, ModelCatalogVersion: modelCatalogVersion, Effort: effort,
		ContextWindowTokens: req.ContextWindowTokens, IsEco: false, IsAuto: false, IsFree: false, IsLimitFree: false,
		NativeAPI: cfg.NativeAPI,
	}

	allowTools := len(req.Tools) > 0
	if req.ToolChoice != nil && req.ToolChoice.Type == canonical.ToolChoiceNone {
		// Current Desktop captures do not expose a native "none" choice. The
		// reliable representation is to omit tools entirely.
		allowTools = false
	}
	if allowTools {
		encTools, err := codec.EncodeJSON(toUpstreamTools(req.Tools, req.Model))
		if err != nil { return Envelope{}, fmt.Errorf("encode tools: %w", err) }
		env.Tools = encTools
		if req.ToolChoice == nil { env.ToolChoice = map[string]any{"type":"auto"} }
	}
	if allowTools && req.ToolChoice != nil {
		env.ToolChoice = toUpstreamToolChoice(*req.ToolChoice)
	}
	return env, nil
}

func foldSystemIntoMessages(system []canonical.ContentBlock, messages []canonical.Message) []canonical.Message {
	text := renderSystemText(system)
	if text == "" {
		return messages
	}
	block := canonical.ContentBlock{Type:canonical.BlockText, Text:"<system>\n" + text + "\n</system>"}
	out := append([]canonical.Message(nil), messages...)
	if len(out) > 0 && out[0].Role == canonical.RoleUser {
		content := make([]canonical.ContentBlock, 0, len(out[0].Content)+1)
		content = append(content, block)
		content = append(content, out[0].Content...)
		out[0].Content = content
		return out
	}
	return append([]canonical.Message{{Role:canonical.RoleUser, Content:[]canonical.ContentBlock{block}}}, out...)
}

func renderSystemText(blocks []canonical.ContentBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
			continue
		}
		if raw, err := json.Marshal(block); err == nil {
			parts = append(parts, string(raw))
		}
	}
	return strings.Join(parts, "\n\n")
}

func toUpstreamMessages(messages []canonical.Message, model string, now time.Time) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	timestamp := "<timestamp>" + now.Format("Mon Jan 02 2006 15:04:05 GMT-0700") + "</timestamp>\n"
	for index, msg := range messages {
		blocks := make([]map[string]any, 0, len(msg.Content)+1)
		blocks = append(blocks, map[string]any{"type":"text","text":timestamp})
		for _, b := range msg.Content {
			switch b.Type {
			case canonical.BlockText:
				blocks = append(blocks, map[string]any{"type":"text","text":b.Text})
			case canonical.BlockImage:
				blocks = append(blocks, map[string]any{"type":"image","source":map[string]any{"type":"url","url":b.URL}})
			case canonical.BlockThinking:
				blocks = append(blocks, map[string]any{"type":"thinking","thinking":b.Text})
			case canonical.BlockToolUse:
				var input any = map[string]any{}
				if len(b.Input) > 0 { _ = json.Unmarshal(b.Input, &input) }
				blocks = append(blocks, map[string]any{"type":"tool_use","id":b.ToolID,"name":b.ToolName,"input":input})
			case canonical.BlockToolResult:
				blocks = append(blocks, map[string]any{"type":"tool_result","tool_use_id":b.ToolID,"content":b.Text,"is_error":b.IsError})
			}
		}
		if index == len(messages)-1 && len(blocks) > 0 {
			blocks[len(blocks)-1]["cache_control"] = map[string]any{"type":"ephemeral"}
		}
		upstream := map[string]any{"role":string(msg.Role),"content":blocks}
		if msg.Role == canonical.RoleAssistant {
			upstream["model"] = model
		}
		out = append(out, upstream)
	}
	return out
}

func toUpstreamTools(tools []canonical.Tool, model string) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if tool.Name == "" { continue }
		if tool.Name == "apply_patch" && isGPT5Family(model) {
			out = append(out, map[string]any{"type":"apply_patch"})
			continue
		}
		var schema any = map[string]any{"type":"object","properties":map[string]any{}}
		if len(tool.InputSchema) > 0 { _ = json.Unmarshal(tool.InputSchema, &schema) }
		out = append(out, map[string]any{"name":tool.Name,"description":tool.Description,"input_schema":schema})
	}
	return out
}

func isGPT5Family(model string) bool {
	return len(model) >= 7 && model[:6] == "gpt-5."
}

func toUpstreamToolChoice(choice canonical.ToolChoice) map[string]any {
	switch choice.Type {
	case canonical.ToolChoiceAny:
		return map[string]any{"type":"any"}
	case canonical.ToolChoiceTool:
		return map[string]any{"type":"tool","name":choice.Name}
	default:
		return map[string]any{"type":"auto"}
	}
}

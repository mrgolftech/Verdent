package protocol

import (
	"encoding/json"
	"errors"
	"fmt"

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

	system := append([]canonical.ContentBlock(nil), req.System...)
	system = append(system, cfg.SystemTrailer...)
	messages := normalizeMessages(req.Messages)
	encSystem, err := codec.EncodeJSON(system)
	if err != nil { return Envelope{}, fmt.Errorf("encode system: %w", err) }
	encMessages, err := codec.EncodeJSON(toUpstreamMessages(messages, req.Model))
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

	env := Envelope{
		Channel: cfg.Channel, Model: req.Model, SessionID: opt.IDs.SessionID, ConvID: opt.IDs.ConvID, ReactID: opt.IDs.ReactID,
		ReactType: cfg.ReactType, Stream: true, MaxTokens: maxTokens, Temperature: temperature,
		System: encSystem, Messages: encMessages, AgentName: cfg.AgentName, Env: opt.Environment, Encrypt: true,
		TraceTags: []string{}, TraceMetadata: trace, ModelCatalogVersion: opt.ModelCatalogVersion, Effort: req.Effort,
		ContextWindowTokens: req.ContextWindowTokens, IsEco: false, IsAuto: false, NativeAPI: cfg.NativeAPI,
	}

	if len(req.Tools) > 0 {
		encTools, err := codec.EncodeJSON(toUpstreamTools(req.Tools, req.Model))
		if err != nil { return Envelope{}, fmt.Errorf("encode tools: %w", err) }
		env.Tools = encTools
		if req.ToolChoice == nil { env.ToolChoice = map[string]any{"type":"auto"} }
	}
	if req.ToolChoice != nil {
		env.ToolChoice = toUpstreamToolChoice(*req.ToolChoice)
	}
	return env, nil
}

func toUpstreamMessages(messages []canonical.Message, model string) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, msg := range messages {
		blocks := make([]map[string]any, 0, len(msg.Content))
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
		out = append(out, map[string]any{"role":string(msg.Role),"content":blocks,"model":model})
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
	case canonical.ToolChoiceNone:
		return map[string]any{"type":"none"}
	case canonical.ToolChoiceTool:
		return map[string]any{"type":"tool","name":choice.Name}
	default:
		return map[string]any{"type":"auto"}
	}
}

package canonical

import "encoding/json"

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type BlockType string

const (
	BlockText       BlockType = "text"
	BlockImage      BlockType = "image"
	BlockThinking   BlockType = "thinking"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
)

type ContentBlock struct {
	Type      BlockType       `json:"type"`
	Text      string          `json:"text,omitempty"`
	URL       string          `json:"url,omitempty"`
	ToolID    string          `json:"tool_id,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

type ToolKind string

const (
	ToolKindFunction ToolKind = "function"
	ToolKindCustom   ToolKind = "custom"
)

type Tool struct {
	Kind        ToolKind        `json:"kind,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Format      json.RawMessage `json:"format,omitempty"`
}

type ToolChoiceType string

const (
	ToolChoiceAuto ToolChoiceType = "auto"
	ToolChoiceAny  ToolChoiceType = "any"
	ToolChoiceNone ToolChoiceType = "none"
	ToolChoiceTool ToolChoiceType = "tool"
)

type ToolChoice struct {
	Type ToolChoiceType `json:"type"`
	Name string         `json:"name,omitempty"`
}

type Request struct {
	Model               string         `json:"model"`
	System              []ContentBlock `json:"system,omitempty"`
	Messages            []Message      `json:"messages"`
	Tools               []Tool         `json:"tools,omitempty"`
	ToolChoice          *ToolChoice    `json:"tool_choice,omitempty"`
	MaxTokens           int            `json:"max_tokens,omitempty"`
	Temperature         *float64       `json:"temperature,omitempty"`
	Effort              string         `json:"effort,omitempty"`
	ContextWindowTokens int            `json:"context_window_tokens,omitempty"`
	UserQuery           string         `json:"user_query,omitempty"`
}

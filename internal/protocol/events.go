package protocol

import "encoding/json"

type EventType string

const (
	EventMessageStart EventType = "message_start"
	EventTextDelta    EventType = "text_delta"
	EventThinking     EventType = "thinking_delta"
	EventToolStart    EventType = "tool_start"
	EventToolDelta    EventType = "tool_delta"
	EventToolEnd      EventType = "tool_end"
	EventUsage        EventType = "usage"
	EventMessageEnd   EventType = "message_end"
	EventError        EventType = "error"
)

// Event is the provider-neutral stream representation consumed by API adapters.
type Event struct {
	Type      EventType       `json:"type"`
	Index     int             `json:"index,omitempty"`
	Text      string          `json:"text,omitempty"`
	ToolID    string          `json:"tool_id,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Usage     *Usage          `json:"usage,omitempty"`
	Err       string          `json:"error,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`
}

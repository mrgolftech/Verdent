package responses

import "encoding/json"

type Request struct {
	Model              string          `json:"model"`
	Instructions       string          `json:"instructions,omitempty"`
	Input              json.RawMessage `json:"input"`
	Tools              []Tool          `json:"tools,omitempty"`
	ToolChoice         json.RawMessage `json:"tool_choice,omitempty"`
	ParallelToolCalls  bool            `json:"parallel_tool_calls,omitempty"`
	Reasoning          *Reasoning      `json:"reasoning,omitempty"`
	Stream             bool            `json:"stream,omitempty"`
	MaxOutputTokens    int             `json:"max_output_tokens,omitempty"`
	Temperature        *float64        `json:"temperature,omitempty"`
	PreviousResponseID string          `json:"previous_response_id,omitempty"`
}

type Reasoning struct {
	Effort string `json:"effort,omitempty"`
}

type Tool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
	Format      json.RawMessage `json:"format,omitempty"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type Response struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	CreatedAt  int64  `json:"created_at"`
	Status     string `json:"status"`
	Model      string `json:"model"`
	Output     []any  `json:"output"`
	OutputText string `json:"output_text,omitempty"`
	Usage      Usage  `json:"usage"`
	Error      any    `json:"error"`
}

type Event struct {
	Type           string    `json:"type"`
	SequenceNumber int       `json:"sequence_number"`
	Response       *Response `json:"response,omitempty"`
	OutputIndex    *int      `json:"output_index,omitempty"`
	ContentIndex   *int      `json:"content_index,omitempty"`
	SummaryIndex   *int      `json:"summary_index,omitempty"`
	ItemID         string    `json:"item_id,omitempty"`
	Item           any       `json:"item,omitempty"`
	Part           any       `json:"part,omitempty"`
	Delta          string    `json:"delta,omitempty"`
	Text           string    `json:"text,omitempty"`
	Arguments      string    `json:"arguments,omitempty"`
	Input          string    `json:"input,omitempty"`
}

package responses

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/mrgolftech/Verdent/internal/protocol"
)

type toolState struct {
	ID        string
	Name      string
	Custom    bool
	Arguments strings.Builder
}

type Assembler struct {
	custom    map[string]bool
	text      strings.Builder
	reasoning strings.Builder
	tools     []toolState
	byIndex   map[int]int
	usage     Usage
}

func NewAssembler(custom map[string]bool) *Assembler {
	return &Assembler{custom: custom, byIndex: map[int]int{}}
}

func (a *Assembler) Consume(ev protocol.Event) {
	switch ev.Type {
	case protocol.EventTextDelta:
		a.text.WriteString(ev.Text)
	case protocol.EventThinking:
		a.reasoning.WriteString(ev.Text)
	case protocol.EventToolStart:
		index := len(a.tools)
		a.byIndex[ev.Index] = index
		state := toolState{ID: ev.ToolID, Name: ev.ToolName, Custom: a.custom[ev.ToolName]}
		if len(ev.Arguments) > 0 {
			state.Arguments.Write(ev.Arguments)
		}
		a.tools = append(a.tools, state)
	case protocol.EventToolDelta:
		if index, ok := a.byIndex[ev.Index]; ok {
			a.tools[index].Arguments.WriteString(ev.ArgumentDelta)
		}
	case protocol.EventUsage:
		if ev.Usage != nil {
			if ev.Usage.InputTokens > 0 {
				a.usage.InputTokens = ev.Usage.InputTokens
			}
			if ev.Usage.OutputTokens > 0 {
				a.usage.OutputTokens = ev.Usage.OutputTokens
			}
			a.usage.TotalTokens = a.usage.InputTokens + a.usage.OutputTokens
		}
	}
}

func (a *Assembler) Response(id, model string, created int64) Response {
	output := make([]any, 0, len(a.tools)+2)

	if reasoning := a.reasoning.String(); reasoning != "" {
		output = append(output, map[string]any{
			"id": responseItemID("rs_"),
			"type": "reasoning",
			"summary": []any{map[string]any{"type": "summary_text", "text": reasoning}},
		})
	}

	for _, tool := range a.tools {
		raw := tool.Arguments.String()
		if raw == "" {
			raw = "{}"
		}
		if tool.Custom {
			output = append(output, map[string]any{
				"id": responseItemID("ctc_"),
				"type": "custom_tool_call",
				"status": "completed",
				"call_id": tool.ID,
				"name": tool.Name,
				"input": customInput(raw),
			})
			continue
		}
		output = append(output, map[string]any{
			"id": responseItemID("fc_"),
			"type": "function_call",
			"status": "completed",
			"call_id": tool.ID,
			"name": tool.Name,
			"arguments": raw,
		})
	}

	text := a.text.String()
	if text != "" {
		output = append(output, map[string]any{
			"id": responseItemID("msg_"),
			"type": "message",
			"status": "completed",
			"role": "assistant",
			"content": []any{map[string]any{
				"type": "output_text",
				"text": text,
				"annotations": []any{},
			}},
		})
	}

	return Response{
		ID: id, Object: "response", CreatedAt: created, Status: "completed",
		Model: model, Output: output, OutputText: text, Usage: a.usage, Error: nil,
	}
}

func responseItemID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return prefix + "fallback"
	}
	return prefix + hex.EncodeToString(buf)
}

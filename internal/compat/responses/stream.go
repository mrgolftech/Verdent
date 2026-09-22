package responses

import (
	"strings"

	"github.com/mrgolftech/Verdent/internal/protocol"
)

type streamTool struct {
	OutputIndex int
	ItemID      string
	CallID      string
	Name        string
	Custom      bool
	Input       strings.Builder
	Done        bool
}

type StreamEncoder struct {
	ID      string
	Model   string
	Created int64

	custom map[string]bool
	seq    int
	started bool
	nextOutput int

	reasoningID    string
	reasoningIndex int
	reasoning      strings.Builder
	reasoningOpen  bool

	messageID    string
	messageIndex int
	text         strings.Builder
	messageOpen  bool

	tools     map[int]*streamTool
	toolOrder []int
	usage     Usage
	completed bool
}

func NewStreamEncoder(id, model string, created int64, custom map[string]bool) *StreamEncoder {
	return &StreamEncoder{
		ID: id, Model: model, Created: created,
		custom: custom, tools: map[int]*streamTool{},
	}
}

func (s *StreamEncoder) Encode(ev protocol.Event) []Event {
	if s.completed {
		return nil
	}
	var out []Event
	if !s.started {
		out = append(out, s.startEvents()...)
	}

	switch ev.Type {
	case protocol.EventThinking:
		out = append(out, s.reasoningDelta(ev.Text)...)
	case protocol.EventTextDelta:
		out = append(out, s.textDelta(ev.Text)...)
	case protocol.EventToolStart:
		out = append(out, s.toolStart(ev)...)
	case protocol.EventToolDelta:
		out = append(out, s.toolDelta(ev)...)
	case protocol.EventToolEnd:
		out = append(out, s.toolDone(ev.Index)...)
	case protocol.EventUsage:
		s.consumeUsage(ev)
	case protocol.EventMessageEnd:
		out = append(out, s.finish()...)
	case protocol.EventError:
		s.completed = true
		failed := s.shell("failed")
		failed.Error = map[string]any{"message": ev.Err}
		out = append(out, s.event("response.failed", Event{Response: &failed}))
	}
	return out
}

func (s *StreamEncoder) startEvents() []Event {
	s.started = true
	created := s.shell("in_progress")
	inProgress := s.shell("in_progress")
	return []Event{
		s.event("response.created", Event{Response: &created}),
		s.event("response.in_progress", Event{Response: &inProgress}),
	}
}

func (s *StreamEncoder) reasoningDelta(delta string) []Event {
	var out []Event
	if !s.reasoningOpen {
		s.reasoningOpen = true
		s.reasoningID = responseItemID("rs_")
		s.reasoningIndex = s.nextOutput
		s.nextOutput++
		idx := s.reasoningIndex
		summary := 0
		out = append(out,
			s.event("response.output_item.added", Event{
				OutputIndex: &idx,
				Item: map[string]any{"id": s.reasoningID, "type": "reasoning", "summary": []any{}},
			}),
			s.event("response.reasoning_summary_part.added", Event{
				OutputIndex: &idx, ItemID: s.reasoningID, SummaryIndex: &summary,
				Part: map[string]any{"type": "summary_text", "text": ""},
			}),
		)
	}
	s.reasoning.WriteString(delta)
	idx := s.reasoningIndex
	summary := 0
	out = append(out, s.event("response.reasoning_summary_text.delta", Event{
		OutputIndex: &idx, ItemID: s.reasoningID, SummaryIndex: &summary, Delta: delta,
	}))
	return out
}

func (s *StreamEncoder) textDelta(delta string) []Event {
	var out []Event
	if !s.messageOpen {
		s.messageOpen = true
		s.messageID = responseItemID("msg_")
		s.messageIndex = s.nextOutput
		s.nextOutput++
		idx := s.messageIndex
		content := 0
		out = append(out,
			s.event("response.output_item.added", Event{
				OutputIndex: &idx,
				Item: map[string]any{
					"id": s.messageID, "type": "message", "status": "in_progress",
					"role": "assistant", "content": []any{},
				},
			}),
			s.event("response.content_part.added", Event{
				OutputIndex: &idx, ContentIndex: &content, ItemID: s.messageID,
				Part: map[string]any{"type": "output_text", "text": "", "annotations": []any{}},
			}),
		)
	}
	s.text.WriteString(delta)
	idx := s.messageIndex
	content := 0
	out = append(out, s.event("response.output_text.delta", Event{
		OutputIndex: &idx, ContentIndex: &content, ItemID: s.messageID, Delta: delta,
	}))
	return out
}

func (s *StreamEncoder) toolStart(ev protocol.Event) []Event {
	idx := s.nextOutput
	s.nextOutput++
	state := &streamTool{
		OutputIndex: idx, ItemID: responseItemID("fc_"),
		CallID: ev.ToolID, Name: ev.ToolName, Custom: s.custom[ev.ToolName],
	}
	if state.Custom {
		state.ItemID = responseItemID("ctc_")
	}
	s.tools[ev.Index] = state
	s.toolOrder = append(s.toolOrder, ev.Index)

	var item any
	if state.Custom {
		item = map[string]any{
			"id": state.ItemID, "type": "custom_tool_call", "status": "in_progress",
			"call_id": state.CallID, "name": state.Name, "input": "",
		}
	} else {
		item = map[string]any{
			"id": state.ItemID, "type": "function_call", "status": "in_progress",
			"call_id": state.CallID, "name": state.Name, "arguments": "",
		}
	}
	out := []Event{s.event("response.output_item.added", Event{OutputIndex: &idx, Item: item})}
	if len(ev.Arguments) > 0 {
		out = append(out, s.toolInput(state, string(ev.Arguments))...)
	}
	return out
}

func (s *StreamEncoder) toolDelta(ev protocol.Event) []Event {
	state := s.tools[ev.Index]
	if state == nil {
		return nil
	}
	return s.toolInput(state, ev.ArgumentDelta)
}

func (s *StreamEncoder) toolInput(state *streamTool, delta string) []Event {
	state.Input.WriteString(delta)
	idx := state.OutputIndex
	if state.Custom {
		return []Event{s.event("response.custom_tool_call_input.delta", Event{
			OutputIndex: &idx, ItemID: state.ItemID, Delta: delta,
		})}
	}
	return []Event{s.event("response.function_call_arguments.delta", Event{
		OutputIndex: &idx, ItemID: state.ItemID, Delta: delta,
	})}
}

func (s *StreamEncoder) toolDone(providerIndex int) []Event {
	state := s.tools[providerIndex]
	if state == nil || state.Done {
		return nil
	}
	state.Done = true
	idx := state.OutputIndex
	raw := state.Input.String()
	if raw == "" {
		raw = "{}"
	}
	if state.Custom {
		input := customInput(raw)
		return []Event{
			s.event("response.custom_tool_call_input.done", Event{
				OutputIndex: &idx, ItemID: state.ItemID, Input: input,
			}),
			s.event("response.output_item.done", Event{
				OutputIndex: &idx,
				Item: map[string]any{
					"id": state.ItemID, "type": "custom_tool_call", "status": "completed",
					"call_id": state.CallID, "name": state.Name, "input": input,
				},
			}),
		}
	}
	return []Event{
		s.event("response.function_call_arguments.done", Event{
			OutputIndex: &idx, ItemID: state.ItemID, Arguments: raw,
		}),
		s.event("response.output_item.done", Event{
			OutputIndex: &idx,
			Item: map[string]any{
				"id": state.ItemID, "type": "function_call", "status": "completed",
				"call_id": state.CallID, "name": state.Name, "arguments": raw,
			},
		}),
	}
}

func (s *StreamEncoder) consumeUsage(ev protocol.Event) {
	if ev.Usage == nil {
		return
	}
	if ev.Usage.InputTokens > 0 {
		s.usage.InputTokens = ev.Usage.InputTokens
	}
	if ev.Usage.OutputTokens > 0 {
		s.usage.OutputTokens = ev.Usage.OutputTokens
	}
	s.usage.TotalTokens = s.usage.InputTokens + s.usage.OutputTokens
}

func (s *StreamEncoder) finish() []Event {
	if s.completed {
		return nil
	}
	var out []Event
	for _, providerIndex := range s.toolOrder {
		out = append(out, s.toolDone(providerIndex)...)
	}
	if s.reasoningOpen {
		idx := s.reasoningIndex
		summary := 0
		text := s.reasoning.String()
		part := map[string]any{"type": "summary_text", "text": text}
		out = append(out,
			s.event("response.reasoning_summary_text.done", Event{
				OutputIndex: &idx, ItemID: s.reasoningID, SummaryIndex: &summary, Text: text,
			}),
			s.event("response.reasoning_summary_part.done", Event{
				OutputIndex: &idx, ItemID: s.reasoningID, SummaryIndex: &summary, Part: part,
			}),
			s.event("response.output_item.done", Event{
				OutputIndex: &idx,
				Item: map[string]any{"id": s.reasoningID, "type": "reasoning", "summary": []any{part}},
			}),
		)
	}
	if s.messageOpen {
		idx := s.messageIndex
		content := 0
		text := s.text.String()
		part := map[string]any{"type": "output_text", "text": text, "annotations": []any{}}
		out = append(out,
			s.event("response.output_text.done", Event{
				OutputIndex: &idx, ContentIndex: &content, ItemID: s.messageID, Text: text,
			}),
			s.event("response.content_part.done", Event{
				OutputIndex: &idx, ContentIndex: &content, ItemID: s.messageID, Part: part,
			}),
			s.event("response.output_item.done", Event{
				OutputIndex: &idx,
				Item: map[string]any{
					"id": s.messageID, "type": "message", "status": "completed",
					"role": "assistant", "content": []any{part},
				},
			}),
		)
	}
	completed := s.completedResponse()
	s.completed = true
	out = append(out, s.event("response.completed", Event{Response: &completed}))
	return out
}

func (s *StreamEncoder) completedResponse() Response {
	output := make([]any, 0, len(s.toolOrder)+2)
	if s.reasoningOpen {
		text := s.reasoning.String()
		output = append(output, map[string]any{
			"id": s.reasoningID, "type": "reasoning",
			"summary": []any{map[string]any{"type": "summary_text", "text": text}},
		})
	}
	for _, providerIndex := range s.toolOrder {
		state := s.tools[providerIndex]
		raw := state.Input.String()
		if raw == "" {
			raw = "{}"
		}
		if state.Custom {
			output = append(output, map[string]any{
				"id": state.ItemID, "type": "custom_tool_call", "status": "completed",
				"call_id": state.CallID, "name": state.Name, "input": customInput(raw),
			})
		} else {
			output = append(output, map[string]any{
				"id": state.ItemID, "type": "function_call", "status": "completed",
				"call_id": state.CallID, "name": state.Name, "arguments": raw,
			})
		}
	}
	text := s.text.String()
	if s.messageOpen {
		output = append(output, map[string]any{
			"id": s.messageID, "type": "message", "status": "completed", "role": "assistant",
			"content": []any{map[string]any{
				"type": "output_text", "text": text, "annotations": []any{},
			}},
		})
	}
	return Response{
		ID: s.ID, Object: "response", CreatedAt: s.Created, Status: "completed",
		Model: s.Model, Output: output, OutputText: text, Usage: s.usage, Error: nil,
	}
}

func (s *StreamEncoder) shell(status string) Response {
	return Response{
		ID: s.ID, Object: "response", CreatedAt: s.Created,
		Status: status, Model: s.Model, Output: []any{}, Usage: s.usage, Error: nil,
	}
}

func (s *StreamEncoder) event(kind string, event Event) Event {
	s.seq++
	event.Type = kind
	event.SequenceNumber = s.seq
	return event
}

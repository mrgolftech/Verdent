package protocol

import (
	"encoding/json"
	"fmt"
	"strings"
)

type toolStreamState struct {
	ID   string
	Name string
}

type StreamDecoder struct {
	tools   map[int]toolStreamState
	stopped bool
}

func NewStreamDecoder() *StreamDecoder {
	return &StreamDecoder{tools: make(map[int]toolStreamState)}
}

func (d *StreamDecoder) Decode(frame SSEFrame) ([]Event, error) {
	if d.stopped { return nil, nil }
	if frame.Data == "" { return nil, nil }
	if frame.Data == "[DONE]" {
		d.stopped = true
		return []Event{{Type: EventMessageEnd}}, nil
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(frame.Data), &obj); err != nil {
		return nil, fmt.Errorf("decode upstream event: %w", err)
	}
	typeName := strings.ToLower(strings.TrimSpace(stringValue(obj["type"])))
	if typeName == "" { typeName = strings.ToLower(strings.TrimSpace(frame.Event)) }
	if typeName == "heartbeat" || typeName == "ping" { return nil, nil }
	if typeName == "error" || typeName == "stream_error" {
		d.stopped = true
		msg := stringValue(obj["error"])
		if msg == "" { msg = stringValue(obj["message"]) }
		if msg == "" { msg = frame.Data }
		return []Event{{Type: EventError, Err: msg}}, nil
	}

	switch typeName {
	case "message_start":
		events := []Event{{Type: EventMessageStart}}
		if m, ok := obj["message"].(map[string]any); ok {
			if u := usageFrom(m["usage"]); u != nil { events = append(events, Event{Type: EventUsage, Usage: u}) }
		}
		return events, nil
	case "content_block_start":
		idx := intValue(obj["index"])
		block, _ := obj["content_block"].(map[string]any)
		return d.decodeBlockStart(idx, block), nil
	case "content_block_delta":
		idx := intValue(obj["index"])
		delta, _ := obj["delta"].(map[string]any)
		return d.decodeBlockDelta(idx, delta), nil
	case "content_block_stop":
		idx := intValue(obj["index"])
		if st, ok := d.tools[idx]; ok {
			delete(d.tools, idx)
			return []Event{{Type: EventToolEnd, Index: idx, ToolID: st.ID, ToolName: st.Name}}, nil
		}
		return nil, nil
	case "message_delta":
		if u := usageFrom(obj["usage"]); u != nil { return []Event{{Type: EventUsage, Usage: u}}, nil }
		return nil, nil
	case "message_stop":
		events := make([]Event, 0, 2)
		if u := usageFrom(obj["usage"]); u != nil { events = append(events, Event{Type: EventUsage, Usage: u}) }
		events = append(events, Event{Type: EventMessageEnd})
		d.stopped = true
		return events, nil
	case "message_integrity":
		d.stopped = true
		return []Event{{Type: EventMessageEnd}}, nil
	default:
		return nil, ErrUnsupportedEvent
	}
}

func (d *StreamDecoder) decodeBlockStart(idx int, block map[string]any) []Event {
	if block == nil { return nil }
	switch stringValue(block["type"]) {
	case "text":
		if v := stringValue(block["text"]); v != "" { return []Event{{Type: EventTextDelta, Index: idx, Text: v}} }
	case "thinking":
		if v := stringValue(block["thinking"]); v != "" { return []Event{{Type: EventThinking, Index: idx, Text: v}} }
	case "tool_use", "server_tool_use":
		st := toolStreamState{ID: stringValue(block["id"]), Name: stringValue(block["name"])}
		d.tools[idx] = st
		e := Event{Type: EventToolStart, Index: idx, ToolID: st.ID, ToolName: st.Name}
		if input, ok := block["input"]; ok && input != nil {
			if raw, err := json.Marshal(input); err == nil && string(raw) != "{}" { e.Arguments = raw }
		}
		return []Event{e}
	}
	return nil
}

func (d *StreamDecoder) decodeBlockDelta(idx int, delta map[string]any) []Event {
	if delta == nil { return nil }
	if v := stringValue(delta["text"]); v != "" { return []Event{{Type: EventTextDelta, Index: idx, Text: v}} }
	if v := stringValue(delta["thinking"]); v != "" { return []Event{{Type: EventThinking, Index: idx, Text: v}} }
	fragment := stringValue(delta["partial_json"])
	if fragment == "" { fragment = stringValue(delta["input_json"]) }
	if fragment != "" {
		st := d.tools[idx]
		return []Event{{Type: EventToolDelta, Index: idx, ToolID: st.ID, ToolName: st.Name, ArgumentDelta: fragment}}
	}
	return nil
}

func usageFrom(v any) *Usage {
	m, ok := v.(map[string]any)
	if !ok || m == nil { return nil }
	return &Usage{
		InputTokens: intValue(m["input_tokens"]),
		OutputTokens: intValue(m["output_tokens"]),
		CacheReadTokens: intValue(m["cache_read_input_tokens"]),
		CacheCreateTokens: intValue(m["cache_creation_input_tokens"]),
	}
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func intValue(v any) int {
	switch n := v.(type) {
	case float64: return int(n)
	case int: return n
	case json.Number:
		i, _ := n.Int64(); return int(i)
	default: return 0
	}
}

package protocol

import (
	"strings"
	"testing"
)

func TestStreamDecoderTextThinkingToolAndUsage(t *testing.T) {
	fixture := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":12}}}`+"\n\n",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"plan"}}`+"\n\n",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":" more"}}`+"\n\n",
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file","input":{}}}`+"\n\n",
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\\\"path\\\":\\\"/tmp/a"}}`+"\n\n",
		`data: {"type":"content_block_delta","index":1,"delta":{"partial_json":"\\\"}"}}`+"\n\n",
		`data: {"type":"content_block_stop","index":1}`+"\n\n",
		`data: {"type":"content_block_start","index":2,"content_block":{"type":"text","text":"done"}}`+"\n\n",
		`data: {"type":"message_stop","usage":{"output_tokens":7}}`+"\n\n",
	}, "")

	scanner := NewSSEScanner(strings.NewReader(fixture))
	decoder := NewStreamDecoder()
	var got []Event
	for {
		frame, err := scanner.NextFrame()
		if err != nil { break }
		events, err := decoder.Decode(frame)
		if err != nil && err != ErrUnsupportedEvent { t.Fatal(err) }
		got = append(got, events...)
	}

	if len(got) != 11 { t.Fatalf("expected 11 events, got %d: %#v", len(got), got) }
	if got[1].Type != EventUsage || got[1].Usage.InputTokens != 12 { t.Fatalf("bad input usage: %#v", got[1]) }
	if got[2].Type != EventThinking || got[2].Text != "plan" { t.Fatalf("bad thinking start: %#v", got[2]) }
	if got[4].Type != EventToolStart || got[4].ToolName != "read_file" { t.Fatalf("bad tool start: %#v", got[4]) }
	if got[5].ArgumentDelta != `+"`"+`{"path":"/tmp/a`+"`"+` { t.Fatalf("bad first tool delta: %q", got[5].ArgumentDelta) }
	if got[8].Type != EventTextDelta || got[8].Text != "done" { t.Fatalf("bad text: %#v", got[8]) }
	if got[9].Type != EventUsage || got[9].Usage.OutputTokens != 7 { t.Fatalf("bad output usage: %#v", got[9]) }\n\tif got[10].Type != EventMessageEnd { t.Fatalf("missing message end: %#v", got[10]) }
}

func TestStreamDecoderError(t *testing.T) {
	d := NewStreamDecoder()
	events, err := d.Decode(SSEFrame{Event:"error", Data:`+"`"+`{"message":"weekly limit reached"}`+"`"+`})
	if err != nil { t.Fatal(err) }
	if len(events) != 1 || events[0].Type != EventError { t.Fatalf("unexpected events: %#v", events) }
}

package protocol

import (
	"io"
	"strings"
	"testing"
)

func TestSSEScanner(t *testing.T) {
	input := ": heartbeat\n" +
		"data: {\"type\":\"content_block_delta\"}\n\n" +
		"event: delta\n" +
		"data: line-one\n" +
		"data: line-two\n\n"

	s := NewSSEScanner(strings.NewReader(input))
	first, err := s.NextFrame()
	if err != nil { t.Fatal(err) }
	if first.Event != "" || first.Data != "{\"type\":\"content_block_delta\"}" {
		t.Fatalf("unexpected first frame: %#v", first)
	}
	second, err := s.NextFrame()
	if err != nil { t.Fatal(err) }
	if second.Event != "delta" || second.Data != "line-one\nline-two" {
		t.Fatalf("unexpected second frame: %#v", second)
	}
	if _, err := s.NextFrame(); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

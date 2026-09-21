package protocol

import (
	"io"
	"strings"
	"testing"
)

func TestSSEScanner(t *testing.T) {
	input := ": heartbeat\n" +
		"data: {\"type\":\"content_block_delta\"}\n\n" +
		"event: ignored-name\n" +
		"data: line-one\n" +
		"data: line-two\n\n"

	s := NewSSEScanner(strings.NewReader(input))
	first, err := s.Next()
	if err != nil { t.Fatal(err) }
	if first != "{\"type\":\"content_block_delta\"}" {
		t.Fatalf("unexpected first event: %q", first)
	}
	second, err := s.Next()
	if err != nil { t.Fatal(err) }
	if second != "line-one\nline-two" {
		t.Fatalf("unexpected multiline event: %q", second)
	}
	if _, err := s.Next(); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
}

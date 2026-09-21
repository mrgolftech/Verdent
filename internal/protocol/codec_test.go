package protocol

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestCodecRoundTrip(t *testing.T) {
	codec, err := newCodec("protocol-sign-for-test-only", bytes.NewReader(bytes.Repeat([]byte{0x2a}, nonceSize)))
	if err != nil { t.Fatal(err) }

	encoded, err := codec.EncodeJSON(map[string]any{"role": "user", "text": "hello"})
	if err != nil { t.Fatal(err) }

	wire, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil { t.Fatal(err) }
	if got := wire[:nonceSize]; !bytes.Equal(got, bytes.Repeat([]byte{0x2a}, nonceSize)) {
		t.Fatalf("unexpected nonce: %x", got)
	}

	plain, err := codec.Decode(encoded)
	if err != nil { t.Fatal(err) }
	want := "{\"role\":\"user\",\"text\":\"hello\"}"
	if string(plain) != want {
		t.Fatalf("round-trip mismatch\\nwant: %s\\n got: %s", want, plain)
	}
}

func TestCodecRejectsShortSign(t *testing.T) {
	if _, err := NewCodec("short"); err == nil {
		t.Fatal("expected short sign to be rejected")
	}
}

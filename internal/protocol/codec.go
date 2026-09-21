package protocol

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const nonceSize = 12

// Codec implements the payload codec used by the Verdent transport layer.
// Protocol-sensitive input is supplied by configuration; no credential or
// protocol secret is embedded in the package.
type Codec struct {
	aead        cipher.AEAD
	nonceReader io.Reader
}

// NewCodec derives a 32-byte AES key from a protocol sign string by base64
// encoding the sign text and taking the first 32 bytes.
func NewCodec(sign string) (*Codec, error) {
	return newCodec(sign, rand.Reader)
}

func newCodec(sign string, nonceReader io.Reader) (*Codec, error) {
	encoded := base64.StdEncoding.EncodeToString([]byte(sign))
	if len(encoded) < 32 {
		return nil, errors.New("protocol sign is too short to derive a 32-byte key")
	}
	key := []byte(encoded[:32])
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	return &Codec{aead: aead, nonceReader: nonceReader}, nil
}

func (c *Codec) EncodeJSON(v any) (string, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	return c.Encode(plain)
}

func (c *Codec) Encode(plain []byte) (string, error) {
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(c.nonceReader, nonce); err != nil {
		return "", fmt.Errorf("read nonce: %w", err)
	}
	sealed := c.aead.Seal(nil, nonce, plain, nil)
	wire := append(append(make([]byte, 0, len(nonce)+len(sealed)), nonce...), sealed...)
	return base64.StdEncoding.EncodeToString(wire), nil
}

func (c *Codec) Decode(encoded string) ([]byte, error) {
	wire, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode base64 payload: %w", err)
	}
	if len(wire) < nonceSize+c.aead.Overhead() {
		return nil, errors.New("encoded payload is too short")
	}
	nonce, ciphertext := wire[:nonceSize], wire[nonceSize:]
	plain, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt payload: %w", err)
	}
	return plain, nil
}

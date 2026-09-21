package protocol

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

// SSEScanner reads Server-Sent Events and returns the concatenated data field
// for each event. Framing is deliberately separate from Verdent event decoding.
type SSEScanner struct {
	scanner *bufio.Scanner
	data    []string
	done    bool
}

func NewSSEScanner(r io.Reader) *SSEScanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 4*1024*1024)
	return &SSEScanner{scanner: s}
}

func (s *SSEScanner) Next() (string, error) {
	if s.done { return "", io.EOF }
	for s.scanner.Scan() {
		line := strings.TrimSuffix(s.scanner.Text(), "\r")
		if line == "" {
			if len(s.data) == 0 { continue }
			out := strings.Join(s.data, "\n")
			s.data = s.data[:0]
			return out, nil
		}
		if strings.HasPrefix(line, ":") { continue }
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			s.data = append(s.data, value)
		}
	}
	if err := s.scanner.Err(); err != nil { return "", err }
	s.done = true
	if len(s.data) > 0 {
		out := strings.Join(s.data, "\n")
		s.data = nil
		return out, nil
	}
	return "", io.EOF
}

var ErrUnsupportedEvent = errors.New("unsupported upstream event")

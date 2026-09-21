package protocol

import (
	"bufio"
	"errors"
	"io"
	"strings"
)

type SSEFrame struct {
	Event string
	Data  string
}

// SSEScanner reads Server-Sent Events without interpreting provider payloads.
type SSEScanner struct {
	scanner *bufio.Scanner
	data    []string
	event   string
	done    bool
}

func NewSSEScanner(r io.Reader) *SSEScanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 4*1024*1024)
	return &SSEScanner{scanner: s}
}

func (s *SSEScanner) Next() (string, error) {
	f, err := s.NextFrame()
	if err != nil { return "", err }
	return f.Data, nil
}

func (s *SSEScanner) NextFrame() (SSEFrame, error) {
	if s.done { return SSEFrame{}, io.EOF }
	for s.scanner.Scan() {
		line := strings.TrimSuffix(s.scanner.Text(), "\r")
		if line == "" {
			if len(s.data) == 0 && s.event == "" { continue }
			out := SSEFrame{Event: s.event, Data: strings.Join(s.data, "\n")}
			s.data = s.data[:0]
			s.event = ""
			return out, nil
		}
		if strings.HasPrefix(line, ":") { continue }
		if strings.HasPrefix(line, "event:") {
			s.event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			s.data = append(s.data, value)
		}
	}
	if err := s.scanner.Err(); err != nil { return SSEFrame{}, err }
	s.done = true
	if len(s.data) > 0 || s.event != "" {
		out := SSEFrame{Event: s.event, Data: strings.Join(s.data, "\n")}
		s.data = nil
		s.event = ""
		return out, nil
	}
	return SSEFrame{}, io.EOF
}

var ErrUnsupportedEvent = errors.New("unsupported upstream event")

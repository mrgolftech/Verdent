package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	openai "github.com/mrgolftech/Verdent/internal/compat/openai"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

type benchmarkRequest struct {
	Model               string `json:"model"`
	Prompt              string `json:"prompt"`
	ContextWindowTokens int    `json:"context_window_tokens"`
	Concurrency         int    `json:"concurrency"`
	MaxOutputTokens     int    `json:"max_output_tokens"`
}

type benchmarkEvent struct {
	Type                string  `json:"type"`
	Index               int     `json:"index,omitempty"`
	Status              string  `json:"status,omitempty"`
	AccountID           string  `json:"account_id,omitempty"`
	TTFTMS              int64   `json:"ttft_ms,omitempty"`
	ElapsedMS           int64   `json:"elapsed_ms,omitempty"`
	DurationMS          int64   `json:"duration_ms,omitempty"`
	ThroughputTPS       float64 `json:"throughput_tps,omitempty"`
	ThroughputEstimated bool    `json:"throughput_estimated"`
	InputTokens         int     `json:"input_tokens,omitempty"`
	OutputTokens        int     `json:"output_tokens,omitempty"`
	TotalTokens         int     `json:"total_tokens,omitempty"`
	Preview             string  `json:"preview,omitempty"`
	Error               string  `json:"error,omitempty"`
}

func (s *Server) handleAdminBenchmarkChat(w http.ResponseWriter, r *http.Request) {
	var input benchmarkRequest
	if err := readAdminJSON(r, &input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := validateBenchmarkRequest(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "HTTP streaming is unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	events := make(chan benchmarkEvent, maxInt(32, input.Concurrency*8))
	var wg sync.WaitGroup
	wg.Add(input.Concurrency)
	for i := 0; i < input.Concurrency; i++ {
		index := i + 1
		go func() {
			defer wg.Done()
			s.runBenchmarkRequest(r.Context(), index, input, func(event benchmarkEvent) {
				select {
				case events <- event:
				case <-r.Context().Done():
				}
			})
		}()
	}
	go func() {
		wg.Wait()
		close(events)
	}()

	meta := benchmarkEvent{Type: "batch", Status: "running"}
	if err := writeBenchmarkEvent(w, meta); err != nil {
		return
	}
	flusher.Flush()

	for event := range events {
		if err := writeBenchmarkEvent(w, event); err != nil {
			return
		}
		flusher.Flush()
	}
	_ = writeBenchmarkEvent(w, benchmarkEvent{Type: "batch", Status: "done"})
	flusher.Flush()
}

func validateBenchmarkRequest(input *benchmarkRequest) error {
	input.Model = strings.TrimSpace(input.Model)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if input.Model == "" {
		return fmt.Errorf("model is required")
	}
	if input.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	if input.Concurrency <= 0 {
		input.Concurrency = 1
	}
	if input.Concurrency > 64 {
		return fmt.Errorf("concurrency must be between 1 and 64")
	}
	if input.ContextWindowTokens < 0 || input.ContextWindowTokens > 2_000_000 {
		return fmt.Errorf("context_window_tokens must be between 0 and 2000000")
	}
	if input.MaxOutputTokens <= 0 {
		input.MaxOutputTokens = 512
	}
	if input.MaxOutputTokens > 64_000 {
		return fmt.Errorf("max_output_tokens must be between 1 and 64000")
	}
	return nil
}

func (s *Server) runBenchmarkRequest(ctx context.Context, index int, input benchmarkRequest, emit func(benchmarkEvent)) {
	started := time.Now()
	state := benchmarkEvent{Type: "request", Index: index, Status: "running"}
	emit(state)

	if s.Accounts == nil {
		state.Status = "error"
		state.Error = "no account router configured"
		state.DurationMS = time.Since(started).Milliseconds()
		emit(state)
		return
	}
	selected := s.Accounts.Select("", "")
	if selected == nil {
		state.Status = "error"
		state.Error = "no eligible Verdent account"
		state.DurationMS = time.Since(started).Milliseconds()
		emit(state)
		return
	}
	fresh, err := s.ensureFreshAccount(ctx, selected)
	if err != nil {
		state.Status = "error"
		state.Error = err.Error()
		state.DurationMS = time.Since(started).Milliseconds()
		emit(state)
		return
	}
	state.AccountID = fresh.Credential.ID
	emit(state)

	content, _ := json.Marshal(input.Prompt)
	chatReq := openai.ChatRequest{
		Model:               input.Model,
		Messages:            []openai.Message{{Role: "user", Content: content}},
		MaxCompletionTokens: input.MaxOutputTokens,
		ContextWindowTokens: input.ContextWindowTokens,
		Stream:              true,
		StreamOptions:       &openai.StreamOptions{IncludeUsage: true},
	}
	canonicalReq, err := openai.ToCanonical(chatReq)
	if err != nil {
		state.Status = "error"
		state.Error = err.Error()
		state.DurationMS = time.Since(started).Milliseconds()
		emit(state)
		return
	}

	client, err := s.NewClient(*fresh, s.ProtocolConfig, s.RequestTimeout)
	if err != nil {
		state.Status = "error"
		state.Error = err.Error()
		state.DurationMS = time.Since(started).Milliseconds()
		emit(state)
		return
	}

	requestCtx := ctx
	if s.RequestTimeout > 0 {
		var cancel context.CancelFunc
		requestCtx, cancel = context.WithTimeout(ctx, s.RequestTimeout)
		defer cancel()
	}
	resp, err := client.Do(requestCtx, fresh.Credential.Token, canonicalReq, protocol.EnvelopeOptions{IDs: requestIDs()})
	if err != nil {
		state.Status = "error"
		state.Error = err.Error()
		state.DurationMS = time.Since(started).Milliseconds()
		emit(state)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		raw := strings.TrimSpace(string(body))
		if raw == "" {
			raw = http.StatusText(resp.StatusCode)
		}
		s.recordUpstreamFailure(fresh.Credential.ID, resp.StatusCode, raw, resp.Header.Get("Retry-After"))
		state.Status = "error"
		state.Error = fmt.Sprintf("upstream HTTP %d: %s", resp.StatusCode, raw)
		state.DurationMS = time.Since(started).Milliseconds()
		emit(state)
		return
	}

	scanner := protocol.NewSSEScanner(bufio.NewReader(resp.Body))
	decoder := protocol.NewStreamDecoder()
	var firstTokenAt time.Time
	var visible strings.Builder
	var usage protocol.Usage
	estimatedOutputTokens := 0
	lastEmit := time.Time{}
	ended := false

	emitProgress := func(force bool) {
		now := time.Now()
		if !force && !lastEmit.IsZero() && now.Sub(lastEmit) < 100*time.Millisecond {
			return
		}
		lastEmit = now
		state.Status = "running"
		state.ElapsedMS = now.Sub(started).Milliseconds()
		if !firstTokenAt.IsZero() {
			state.TTFTMS = firstTokenAt.Sub(started).Milliseconds()
			genSeconds := now.Sub(firstTokenAt).Seconds()
			if genSeconds > 0 {
				if usage.OutputTokens > 0 {
					state.ThroughputTPS = float64(usage.OutputTokens) / genSeconds
					state.ThroughputEstimated = false
				} else {
					state.ThroughputTPS = float64(estimatedOutputTokens) / genSeconds
					state.ThroughputEstimated = true
				}
			}
		}
		state.InputTokens = usage.InputTokens
		state.OutputTokens = usage.OutputTokens
		state.TotalTokens = usage.InputTokens + usage.OutputTokens
		state.Preview = truncateRunes(visible.String(), 500)
		emit(state)
	}

	for {
		frame, scanErr := scanner.NextFrame()
		if scanErr == io.EOF {
			break
		}
		if scanErr != nil {
			state.Status = "error"
			state.Error = scanErr.Error()
			break
		}
		providerEvents, decodeErr := decoder.Decode(frame)
		if decodeErr == protocol.ErrUnsupportedEvent {
			continue
		}
		if decodeErr != nil {
			state.Status = "error"
			state.Error = decodeErr.Error()
			break
		}
		for _, event := range providerEvents {
			switch event.Type {
			case protocol.EventTextDelta:
				if event.Text != "" {
					if firstTokenAt.IsZero() {
						firstTokenAt = time.Now()
					}
					visible.WriteString(event.Text)
					estimatedOutputTokens += estimateGeneratedTokens(event.Text)
					emitProgress(state.TTFTMS == 0)
				}
			case protocol.EventThinking:
				if event.Text != "" {
					if firstTokenAt.IsZero() {
						firstTokenAt = time.Now()
					}
					estimatedOutputTokens += estimateGeneratedTokens(event.Text)
					emitProgress(state.TTFTMS == 0)
				}
			case protocol.EventToolStart:
				if firstTokenAt.IsZero() {
					firstTokenAt = time.Now()
				}
				estimatedOutputTokens += estimateGeneratedTokens(event.ToolName)
				estimatedOutputTokens += estimateGeneratedTokens(string(event.Arguments))
				emitProgress(state.TTFTMS == 0)
			case protocol.EventToolDelta:
				if event.ArgumentDelta != "" {
					if firstTokenAt.IsZero() {
						firstTokenAt = time.Now()
					}
					estimatedOutputTokens += estimateGeneratedTokens(event.ArgumentDelta)
					emitProgress(state.TTFTMS == 0)
				}
			case protocol.EventUsage:
				if event.Usage != nil {
					if event.Usage.InputTokens > 0 {
						usage.InputTokens = event.Usage.InputTokens
					}
					if event.Usage.OutputTokens > 0 {
						usage.OutputTokens = event.Usage.OutputTokens
					}
					if event.Usage.CacheReadTokens > 0 {
						usage.CacheReadTokens = event.Usage.CacheReadTokens
					}
					if event.Usage.CacheCreateTokens > 0 {
						usage.CacheCreateTokens = event.Usage.CacheCreateTokens
					}
					emitProgress(true)
				}
			case protocol.EventError:
				s.recordUpstreamFailure(fresh.Credential.ID, 0, event.Err, "")
				state.Status = "error"
				state.Error = event.Err
			case protocol.EventMessageEnd:
				ended = true
			}
			if state.Status == "error" {
				break
			}
		}
		if state.Status == "error" || ended {
			break
		}
	}

	finished := time.Now()
	state.DurationMS = finished.Sub(started).Milliseconds()
	state.ElapsedMS = state.DurationMS
	if !firstTokenAt.IsZero() {
		state.TTFTMS = firstTokenAt.Sub(started).Milliseconds()
		genSeconds := finished.Sub(firstTokenAt).Seconds()
		if genSeconds > 0 {
			if usage.OutputTokens > 0 {
				state.ThroughputTPS = float64(usage.OutputTokens) / genSeconds
				state.ThroughputEstimated = false
			} else {
				state.ThroughputTPS = float64(estimatedOutputTokens) / genSeconds
				state.ThroughputEstimated = true
			}
		}
	}
	state.InputTokens = usage.InputTokens
	state.OutputTokens = usage.OutputTokens
	state.TotalTokens = usage.InputTokens + usage.OutputTokens
	state.Preview = truncateRunes(visible.String(), 500)
	if state.Status != "error" {
		state.Status = "done"
	}
	emit(state)
}

func writeBenchmarkEvent(w io.Writer, event benchmarkEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

func estimateGeneratedTokens(text string) int {
	if text == "" {
		return 0
	}
	tokens := 0
	asciiRun := 0
	flushASCII := func() {
		if asciiRun > 0 {
			tokens += (asciiRun + 3) / 4
			asciiRun = 0
		}
	}
	for _, r := range text {
		switch {
		case r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			asciiRun++
		case unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r):
			flushASCII()
			tokens++
		case unicode.IsSpace(r):
			flushASCII()
		default:
			flushASCII()
			tokens++
		}
	}
	flushASCII()
	if tokens == 0 && utf8.RuneCountInString(text) > 0 {
		return 1
	}
	return tokens
}

func truncateRunes(text string, limit int) string {
	if limit <= 0 || text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

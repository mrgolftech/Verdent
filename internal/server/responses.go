package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	responsesapi "github.com/mrgolftech/Verdent/internal/compat/responses"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	var in responsesapi.Request
	dec := json.NewDecoder(io.LimitReader(r.Body, 16<<20))
	if err := dec.Decode(&in); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON request: "+err.Error())
		return
	}

	conversion, err := responsesapi.ToCanonical(in)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	account, err := s.selectAccount(r)
	if err != nil {
		writeAPIError(w, http.StatusServiceUnavailable, "no_account", err.Error())
		return
	}
	client, err := s.NewClient(*account, s.ProtocolConfig, s.RequestTimeout)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "client_config", err.Error())
		return
	}

	ctx := r.Context()
	if s.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.RequestTimeout)
		defer cancel()
	}

	resp, err := client.Do(ctx, account.Credential.Token, conversion.Request, protocol.EnvelopeOptions{IDs: requestIDs()})
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, "upstream_request", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.handleUpstreamHTTPError(w, account.Credential.ID, resp)
		return
	}

	id := randomID("resp_")
	created := time.Now().Unix()
	if in.Stream {
		s.streamResponses(w, resp, id, in.Model, created, conversion.CustomTools)
		return
	}
	s.collectResponses(w, resp, id, in.Model, created, conversion.CustomTools)
}

func (s *Server) collectResponses(
	w http.ResponseWriter,
	resp *http.Response,
	id, model string,
	created int64,
	customTools map[string]bool,
) {
	scanner := protocol.NewSSEScanner(resp.Body)
	decoder := protocol.NewStreamDecoder()
	assembler := responsesapi.NewAssembler(customTools)

	for {
		frame, err := scanner.NextFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, "upstream_stream", err.Error())
			return
		}
		events, err := decoder.Decode(frame)
		if err == protocol.ErrUnsupportedEvent {
			continue
		}
		if err != nil {
			writeAPIError(w, http.StatusBadGateway, "upstream_decode", err.Error())
			return
		}
		for _, event := range events {
			if event.Type == protocol.EventError {
				writeAPIError(w, http.StatusBadGateway, "upstream_error", event.Err)
				return
			}
			assembler.Consume(event)
		}
	}

	writeJSON(w, http.StatusOK, assembler.Response(id, model, created))
}

func (s *Server) streamResponses(
	w http.ResponseWriter,
	resp *http.Response,
	id, model string,
	created int64,
	customTools map[string]bool,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "streaming_unsupported", "HTTP streaming is unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	encoder := responsesapi.NewStreamEncoder(id, model, created, customTools)
	scanner := protocol.NewSSEScanner(bufio.NewReader(resp.Body))
	decoder := protocol.NewStreamDecoder()
	ended := false

	for {
		frame, err := scanner.NextFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeResponsesStreamError(w, flusher, "upstream_stream", err.Error())
			return
		}
		events, err := decoder.Decode(frame)
		if err == protocol.ErrUnsupportedEvent {
			continue
		}
		if err != nil {
			writeResponsesStreamError(w, flusher, "upstream_decode", err.Error())
			return
		}
		for _, event := range events {
			out := encoder.Encode(event)
			for _, apiEvent := range out {
				if err := writeResponsesEvent(w, apiEvent); err != nil {
					return
				}
				if apiEvent.Type == "response.completed" || apiEvent.Type == "response.failed" {
					ended = true
				}
			}
		}
		flusher.Flush()
	}

	if !ended {
		for _, apiEvent := range encoder.Encode(protocol.Event{Type: protocol.EventMessageEnd}) {
			if err := writeResponsesEvent(w, apiEvent); err != nil {
				return
			}
		}
	}
	flusher.Flush()
}

func writeResponsesEvent(w io.Writer, event responsesapi.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
	return err
}

func writeResponsesStreamError(w io.Writer, flusher http.Flusher, code, message string) {
	event := responsesapi.Event{
		Type: "error",
		SequenceNumber: 0,
		Response: &responsesapi.Response{
			Object: "response",
			Status: "failed",
			Error: map[string]any{"message": message, "code": code},
		},
	}
	_ = writeResponsesEvent(w, event)
	flusher.Flush()
}

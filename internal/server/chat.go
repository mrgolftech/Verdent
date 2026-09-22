package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	openai "github.com/mrgolftech/Verdent/internal/compat/openai"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

func (s *Server) handleChatCompletions(w http.ResponseWriter,r *http.Request) {
	var in openai.ChatRequest
	dec:=json.NewDecoder(io.LimitReader(r.Body,16<<20))
	if err:=dec.Decode(&in);err!=nil { writeAPIError(w,http.StatusBadRequest,"invalid_json","Invalid JSON request: "+err.Error()); return }
	canonicalReq,err:=openai.ToCanonical(in)
	if err!=nil { writeAPIError(w,http.StatusBadRequest,"invalid_request",err.Error()); return }
	a,err:=s.selectAccount(r)
	if err!=nil { writeAPIError(w,http.StatusServiceUnavailable,"no_account",err.Error()); return }
	client,err:=s.NewClient(*a,s.ProtocolConfig,s.RequestTimeout)
	if err!=nil { writeAPIError(w,http.StatusInternalServerError,"client_config",err.Error()); return }

	ctx:=r.Context()
	if s.RequestTimeout>0 { var cancel context.CancelFunc; ctx,cancel=context.WithTimeout(ctx,s.RequestTimeout); defer cancel() }
	resp,err:=client.Do(ctx,a.Credential.Token,canonicalReq,protocol.EnvelopeOptions{IDs:requestIDs()})
	if err!=nil { writeAPIError(w,http.StatusBadGateway,"upstream_request",err.Error()); return }
	defer resp.Body.Close()
	if resp.StatusCode<200 || resp.StatusCode>=300 { s.handleUpstreamHTTPError(w,a.Credential.ID,resp); return }

	id:=randomID("chatcmpl-")
	created:=time.Now().Unix()
	if in.Stream { s.streamChat(w,resp,id,in.Model,created,in.StreamOptions!=nil && in.StreamOptions.IncludeUsage); return }
	s.collectChat(w,resp,id,in.Model,created)
}

func (s *Server) collectChat(w http.ResponseWriter,resp *http.Response,id,model string,created int64) {
	scanner:=protocol.NewSSEScanner(resp.Body)
	decoder:=protocol.NewStreamDecoder()
	assembler:=openai.NewAssembler()
	for {
		frame,err:=scanner.NextFrame()
		if err==io.EOF { break }
		if err!=nil { writeAPIError(w,http.StatusBadGateway,"upstream_stream",err.Error()); return }
		events,err:=decoder.Decode(frame)
		if err==protocol.ErrUnsupportedEvent { continue }
		if err!=nil { writeAPIError(w,http.StatusBadGateway,"upstream_decode",err.Error()); return }
		for _,ev:=range events {
			if ev.Type==protocol.EventError { writeAPIError(w,http.StatusBadGateway,"upstream_error",ev.Err); return }
			assembler.Consume(ev)
		}
	}
	writeJSON(w,http.StatusOK,assembler.Completion(id,model,created))
}

func (s *Server) streamChat(w http.ResponseWriter,resp *http.Response,id,model string,created int64,includeUsage bool) {
	flusher,ok:=w.(http.Flusher)
	if !ok { writeAPIError(w,http.StatusInternalServerError,"streaming_unsupported","HTTP streaming is unsupported"); return }
	w.Header().Set("Content-Type","text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control","no-cache")
	w.Header().Set("Connection","keep-alive")
	w.WriteHeader(http.StatusOK)
	encoder:=openai.NewStreamEncoder(id,model,created,includeUsage)
	scanner:=protocol.NewSSEScanner(bufio.NewReader(resp.Body))
	decoder:=protocol.NewStreamDecoder()
	ended:=false
	for {
		frame,err:=scanner.NextFrame()
		if err==io.EOF { break }
		if err!=nil { writeSSEError(w,"upstream_stream",err.Error()); flusher.Flush(); return }
		events,err:=decoder.Decode(frame)
		if err==protocol.ErrUnsupportedEvent { continue }
		if err!=nil { writeSSEError(w,"upstream_decode",err.Error()); flusher.Flush(); return }
		for _,ev:=range events {
			chunks,err:=encoder.Encode(ev)
			if err!=nil { writeSSEError(w,"upstream_error",err.Error()); flusher.Flush(); return }
			for _,chunk:=range chunks { data,_:=json.Marshal(chunk); _,_=fmt.Fprintf(w,"data: %s\n\n",data) }
			if ev.Type==protocol.EventMessageEnd { ended=true }
		}
		flusher.Flush()
	}
	if !ended {
		chunks,_:=encoder.Encode(protocol.Event{Type:protocol.EventMessageEnd})
		for _,chunk:=range chunks { data,_:=json.Marshal(chunk); _,_=fmt.Fprintf(w,"data: %s\n\n",data) }
	}
	_,_=io.WriteString(w,"data: [DONE]\n\n")
	flusher.Flush()
}

func writeSSEError(w io.Writer,code,message string) {
	data,_:=json.Marshal(map[string]any{"error":map[string]any{"message":message,"type":"server_error","code":code}})
	_,_=fmt.Fprintf(w,"data: %s\n\n",data)
}

func (s *Server) handleUpstreamHTTPError(w http.ResponseWriter,accountID string,resp *http.Response) {
	body,_:=io.ReadAll(io.LimitReader(resp.Body,1<<20))
	raw:=strings.TrimSpace(string(body))
	if protocol.IsAccountSuspended(raw) { s.Accounts.MarkSuspended(accountID,raw) } else if protocol.IsRateLimit(resp.StatusCode,raw) { s.Accounts.MarkRateLimited(accountID,time.Now().Add(retryAfter(resp.Header.Get("Retry-After"))),raw) }
	message:=raw; if message=="" { message=http.StatusText(resp.StatusCode) }
	writeAPIError(w,http.StatusBadGateway,"upstream_"+strconv.Itoa(resp.StatusCode),message)
}

func retryAfter(raw string) time.Duration {
	if sec,err:=strconv.Atoi(strings.TrimSpace(raw));err==nil && sec>0 { d:=time.Duration(sec)*time.Second; if d>time.Hour{return time.Hour}; return d }
	return time.Minute
}

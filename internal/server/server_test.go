package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mrgolftech/Verdent/internal/account"
	openai "github.com/mrgolftech/Verdent/internal/compat/openai"
	"github.com/mrgolftech/Verdent/internal/protocol"
)

func testServer(t *testing.T,upstream http.Handler) (*Server,*httptest.Server) {
	t.Helper()
	u:=httptest.NewServer(upstream)
	router:=account.NewRouter([]account.Credential{{ID:"a",Token:"token",DeviceID:"device"}})
	s:=New(router,protocol.Config{Endpoint:u.URL,CatalogEndpoint:u.URL,AppVersion:"2.test",BetaHeader:"beta",Sign:"protocol-sign-for-test-only"})
	s.RequestTimeout=5*time.Second
	return s,u
}

func TestChatCompletionNonStreamToolCall(t *testing.T) {
	s,u:=testServer(t,http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		if r.URL.Path!="/" { t.Errorf("unexpected upstream path %q",r.URL.Path) }
		w.Header().Set("Content-Type","text/event-stream")
		io.WriteString(w,`data: {"type":"message_start","message":{"usage":{"input_tokens":8}}}`+"\n\n")
		io.WriteString(w,`data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"call_1","name":"read_file","input":{}}}`+"\n\n")
		io.WriteString(w,`data: {"type":"content_block_delta","index":2,"delta":{"partial_json":"{\"path\":\"/tmp/a\"}"}}`+"\n\n")
		io.WriteString(w,`data: {"type":"content_block_stop","index":2}`+"\n\n")
		io.WriteString(w,`data: {"type":"message_stop","usage":{"output_tokens":3}}`+"\n\n")
	}))
	defer u.Close()
	api:=httptest.NewServer(s.Handler()); defer api.Close()
	body:=`{"model":"m","messages":[{"role":"user","content":"read"}],"tools":[{"type":"function","function":{"name":"read_file","parameters":{"type":"object"}}}]}`
	resp,err:=http.Post(api.URL+"/v1/chat/completions","application/json",strings.NewReader(body)); if err!=nil{t.Fatal(err)}; defer resp.Body.Close()
	if resp.StatusCode!=200 { raw,_:=io.ReadAll(resp.Body); t.Fatalf("status=%d body=%s",resp.StatusCode,raw) }
	var got openai.Completion; if err:=json.NewDecoder(resp.Body).Decode(&got);err!=nil{t.Fatal(err)}
	if got.Choices[0].FinishReason!="tool_calls" || len(got.Choices[0].Message.ToolCalls)!=1 { t.Fatalf("bad completion: %#v",got) }
	if got.Choices[0].Message.ToolCalls[0].Function.Arguments!=`{"path":"/tmp/a"}` { t.Fatalf("bad args: %#v",got) }
	if got.Usage.TotalTokens!=11 { t.Fatalf("bad usage: %#v",got.Usage) }
}

func TestChatCompletionStreamingUsage(t *testing.T) {
	s,u:=testServer(t,http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		w.Header().Set("Content-Type","text/event-stream")
		f:=w.(http.Flusher)
		io.WriteString(w,`data: {"type":"message_start","message":{"usage":{"input_tokens":2}}}`+"\n\n"); f.Flush()
		io.WriteString(w,`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":"hi"}}`+"\n\n"); f.Flush()
		io.WriteString(w,`data: {"type":"message_stop","usage":{"output_tokens":1}}`+"\n\n"); f.Flush()
	}))
	defer u.Close()
	api:=httptest.NewServer(s.Handler()); defer api.Close()
	body:=`{"model":"m","messages":[{"role":"user","content":"hello"}],"stream":true,"stream_options":{"include_usage":true}}`
	resp,err:=http.Post(api.URL+"/v1/chat/completions","application/json",bytes.NewBufferString(body)); if err!=nil{t.Fatal(err)}; defer resp.Body.Close()
	if ct:=resp.Header.Get("Content-Type"); !strings.HasPrefix(ct,"text/event-stream") { t.Fatalf("bad content type %q",ct) }
	scan:=bufio.NewScanner(resp.Body); var payloads []string
	for scan.Scan(){ line:=scan.Text(); if strings.HasPrefix(line,"data: "){ payloads=append(payloads,strings.TrimPrefix(line,"data: ")) } }
	if len(payloads)<5 || payloads[len(payloads)-1]!="[DONE]" { t.Fatalf("bad SSE payloads: %#v",payloads) }
	if !strings.Contains(strings.Join(payloads,"\n"),`"content":"hi"`) { t.Fatalf("missing text delta: %#v",payloads) }
	if !strings.Contains(strings.Join(payloads,"\n"),`"total_tokens":3`) { t.Fatalf("missing usage: %#v",payloads) }
}

func TestModelsEndpoint(t *testing.T) {
	s,u:=testServer(t,http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		w.Header().Set("Content-Type","application/json")
		io.WriteString(w,`{"data":{"model_config":[{"key":"model-a","label":"A"}]}}`)
	}))
	defer u.Close()
	api:=httptest.NewServer(s.Handler()); defer api.Close()
	resp,err:=http.Get(api.URL+"/v1/models"); if err!=nil{t.Fatal(err)}; defer resp.Body.Close()
	var got struct{ Data []struct{ID string `json:"id"`} `json:"data"` }; if err:=json.NewDecoder(resp.Body).Decode(&got);err!=nil{t.Fatal(err)}
	if len(got.Data)!=1 || got.Data[0].ID!="model-a" { t.Fatalf("bad models: %#v",got) }
}

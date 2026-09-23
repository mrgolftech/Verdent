package protocol

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"sync/atomic"
	"time"

	"github.com/mrgolftech/Verdent/internal/canonical"
)

func TestClientDo(t *testing.T) {
	var gotHeader string
	var got Envelope
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		gotHeader=r.Header.Get("X-Version-Code")
		body,_:=io.ReadAll(r.Body)
		if err:=json.Unmarshal(body,&got); err!=nil { t.Errorf("decode envelope: %v",err) }
		w.Header().Set("Content-Type","text/event-stream")
		_,_ = io.WriteString(w, "data: [DONE]\\n\\n")
	}))
	defer server.Close()
	cfg:=Config{Endpoint:server.URL,AppVersion:"2.test",BetaHeader:"beta",Sign:"protocol-sign-for-test-only",DeviceID:"dev"}
	client,err:=NewClient(cfg,server.Client())
	if err!=nil { t.Fatal(err) }
	resp,err:=client.Do(context.Background(),"token",canonical.Request{
		Model:"model-a",Messages:[]canonical.Message{{Role:canonical.RoleUser,Content:[]canonical.ContentBlock{{Type:canonical.BlockText,Text:"hi"}}}},
	},EnvelopeOptions{IDs:RequestIDs{SessionID:"s",ConvID:"c",ReactID:"r"}})
	if err!=nil { t.Fatal(err) }
	defer resp.Body.Close()
	if gotHeader!="2.test" { t.Fatalf("bad version header %q",gotHeader) }
	if got.Model!="model-a" || got.SessionID!="s" || !got.Encrypt { t.Fatalf("bad envelope: %#v",got) }
}


func TestClientRetriesRateLaneWithRetryAfterMs(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		n:=calls.Add(1)
		if n==1 {
			w.Header().Set("Content-Type","application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_,_=io.WriteString(w,`{"code":20004,"retryAfterMs":1}`)
			return
		}
		w.Header().Set("Content-Type","text/event-stream")
		_,_=io.WriteString(w,"data: [DONE]\n\n")
	}))
	defer server.Close()

	cfg:=Config{
		Endpoint:server.URL,AppVersion:"2.15.1",BetaHeader:"hybrid-stream@20250919",
		Sign:"protocol-sign-for-test-only",DeviceID:"dev",
		RetryDelays:[]time.Duration{time.Millisecond},
	}
	client,err:=NewClientWithGate(cfg,server.Client(),NewRequestGate(0))
	if err!=nil{t.Fatal(err)}
	resp,err:=client.Do(context.Background(),"token",canonical.Request{
		Model:"model-a",Messages:[]canonical.Message{{Role:canonical.RoleUser,Content:[]canonical.ContentBlock{{Type:canonical.BlockText,Text:"hi"}}}},
	},EnvelopeOptions{IDs:RequestIDs{SessionID:"s",ConvID:"c",ReactID:"r"}})
	if err!=nil{t.Fatal(err)}
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusOK { t.Fatalf("status=%d",resp.StatusCode) }
	if calls.Load()!=2 { t.Fatalf("calls=%d",calls.Load()) }
}

func TestClientDoesNotRetryNonRetryableServerError(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_,_=io.WriteString(w,`{"error":"permanent"}`)
	}))
	defer server.Close()

	cfg:=Config{
		Endpoint:server.URL,AppVersion:"2.15.1",BetaHeader:"beta",
		Sign:"protocol-sign-for-test-only",DeviceID:"dev",
		RetryDelays:[]time.Duration{time.Millisecond},
	}
	client,err:=NewClientWithGate(cfg,server.Client(),NewRequestGate(0))
	if err!=nil{t.Fatal(err)}
	resp,err:=client.Do(context.Background(),"token",canonical.Request{
		Model:"model-a",Messages:[]canonical.Message{{Role:canonical.RoleUser,Content:[]canonical.ContentBlock{{Type:canonical.BlockText,Text:"hi"}}}},
	},EnvelopeOptions{IDs:RequestIDs{SessionID:"s",ConvID:"c",ReactID:"r"}})
	if err!=nil{t.Fatal(err)}
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusInternalServerError { t.Fatalf("status=%d",resp.StatusCode) }
	if calls.Load()!=1 { t.Fatalf("calls=%d",calls.Load()) }
	body,_:=io.ReadAll(resp.Body)
	if string(body)!=`{"error":"permanent"}` { t.Fatalf("body=%q",body) }
}


func TestClientDoesNotRetryAccountSuspension(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
		calls.Add(1)
		w.Header().Set("Content-Type","application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_,_=io.WriteString(w,`{"error_code":80006,"msg":"Due to a violation of our policies, your free mode access has been suspended"}`)
	}))
	defer server.Close()

	cfg:=Config{
		Endpoint:server.URL,AppVersion:"2.15.1",BetaHeader:"beta",
		Sign:"protocol-sign-for-test-only",DeviceID:"dev",
		RetryDelays:[]time.Duration{time.Millisecond,time.Millisecond,time.Millisecond},
	}
	client,err:=NewClientWithGate(cfg,server.Client(),NewRequestGate(0))
	if err!=nil{t.Fatal(err)}
	resp,err:=client.Do(context.Background(),"token",canonical.Request{
		Model:"model-a",Messages:[]canonical.Message{{Role:canonical.RoleUser,Content:[]canonical.ContentBlock{{Type:canonical.BlockText,Text:"hi"}}}},
	},EnvelopeOptions{IDs:RequestIDs{SessionID:"s",ConvID:"c",ReactID:"r"}})
	if err!=nil{t.Fatal(err)}
	defer resp.Body.Close()
	if calls.Load()!=1 { t.Fatalf("suspension must not retry, calls=%d",calls.Load()) }
	body,_:=io.ReadAll(resp.Body)
	if !IsAccountSuspended(string(body)) { t.Fatalf("expected suspension body, got %q",body) }
}

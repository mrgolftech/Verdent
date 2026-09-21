package protocol

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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

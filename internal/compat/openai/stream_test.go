package openai

import (
	"encoding/json"
	"testing"

	"github.com/mrgolftech/Verdent/internal/protocol"
)

func TestAssemblerBuildsNativeToolCompletion(t *testing.T) {
	a:=NewAssembler()
	for _,ev:=range []protocol.Event{
		{Type:protocol.EventThinking,Text:"plan "},
		{Type:protocol.EventToolStart,Index:4,ToolID:"call_1",ToolName:"read_file"},
		{Type:protocol.EventToolDelta,Index:4,ArgumentDelta:`{"path":`},
		{Type:protocol.EventToolDelta,Index:4,ArgumentDelta:`"/tmp/a"}`},
		{Type:protocol.EventUsage,Usage:&protocol.Usage{InputTokens:10,OutputTokens:5}},
	}{ a.Consume(ev) }
	got:=a.Completion("chatcmpl-1","m",123)
	if got.Choices[0].FinishReason!="tool_calls" { t.Fatalf("bad finish: %#v",got) }
	msg:=got.Choices[0].Message
	if len(msg.ToolCalls)!=1 || msg.ToolCalls[0].Function.Arguments!=`{"path":"/tmp/a"}` { t.Fatalf("bad tool call: %#v",msg.ToolCalls) }
	if msg.ReasoningContent!="plan " || got.Usage.TotalTokens!=15 { t.Fatalf("bad reasoning/usage: %#v",got) }
}

func TestStreamEncoderToolDeltasAndUsage(t *testing.T) {
	s:=NewStreamEncoder("chatcmpl-1","m",123,true)
	events:=[]protocol.Event{
		{Type:protocol.EventMessageStart},
		{Type:protocol.EventToolStart,Index:7,ToolID:"call_1",ToolName:"read_file"},
		{Type:protocol.EventToolDelta,Index:7,ArgumentDelta:`{"path":"/a"}`},
		{Type:protocol.EventUsage,Usage:&protocol.Usage{InputTokens:11,OutputTokens:3}},
		{Type:protocol.EventMessageEnd},
	}
	var chunks []Chunk
	for _,ev:=range events { c,err:=s.Encode(ev); if err!=nil{t.Fatal(err)}; chunks=append(chunks,c...) }
	if len(chunks)!=5 { t.Fatalf("expected 5 chunks, got %d: %#v",len(chunks),chunks) }
	if chunks[1].Choices[0].Delta.ToolCalls[0].Index!=0 || chunks[1].Choices[0].Delta.ToolCalls[0].ID!="call_1" { t.Fatalf("bad tool start: %#v",chunks[1]) }
	if chunks[2].Choices[0].Delta.ToolCalls[0].Function.Arguments!=`{"path":"/a"}` { t.Fatalf("bad args delta: %#v",chunks[2]) }
	if chunks[3].Choices[0].FinishReason==nil || *chunks[3].Choices[0].FinishReason!="tool_calls" { t.Fatalf("bad finish chunk: %#v",chunks[3]) }
	if len(chunks[4].Choices)!=0 || chunks[4].Usage==nil || chunks[4].Usage.TotalTokens!=14 { t.Fatalf("bad usage chunk: %#v",chunks[4]) }
	if _,err:=json.Marshal(chunks);err!=nil { t.Fatal(err) }
}

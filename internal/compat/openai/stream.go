package openai

import (
	"errors"

	"github.com/mrgolftech/Verdent/internal/protocol"
)

type StreamEncoder struct {
	ID string
	Model string
	Created int64
	IncludeUsage bool
	started bool
	toolSeen bool
	toolIndexes map[int]int
	usage Usage
}

func NewStreamEncoder(id,model string,created int64,includeUsage bool) *StreamEncoder {
	return &StreamEncoder{ID:id,Model:model,Created:created,IncludeUsage:includeUsage,toolIndexes:make(map[int]int)}
}

func (s *StreamEncoder) Encode(ev protocol.Event) ([]Chunk,error) {
	if ev.Type==protocol.EventError { return nil,errors.New(ev.Err) }
	chunks:=make([]Chunk,0,3)
	ensureStarted:=func(){
		if s.started { return }
		s.started=true
		chunks=append(chunks,s.chunk(ChunkDelta{Role:"assistant"},nil))
	}
	switch ev.Type {
	case protocol.EventMessageStart:
		ensureStarted()
	case protocol.EventTextDelta:
		ensureStarted(); chunks=append(chunks,s.chunk(ChunkDelta{Content:ev.Text},nil))
	case protocol.EventThinking:
		ensureStarted(); chunks=append(chunks,s.chunk(ChunkDelta{ReasoningContent:ev.Text},nil))
	case protocol.EventToolStart:
		ensureStarted(); s.toolSeen=true
		idx:=len(s.toolIndexes); s.toolIndexes[ev.Index]=idx
		args:=""; if len(ev.Arguments)>0 { args=string(ev.Arguments) }
		d:=ToolCallDelta{Index:idx,ID:ev.ToolID,Type:"function",Function:FunctionCallDelta{Name:ev.ToolName,Arguments:args}}
		chunks=append(chunks,s.chunk(ChunkDelta{ToolCalls:[]ToolCallDelta{d}},nil))
	case protocol.EventToolDelta:
		ensureStarted(); idx,ok:=s.toolIndexes[ev.Index]; if !ok { idx=len(s.toolIndexes); s.toolIndexes[ev.Index]=idx; s.toolSeen=true }
		d:=ToolCallDelta{Index:idx,Function:FunctionCallDelta{Arguments:ev.ArgumentDelta}}
		chunks=append(chunks,s.chunk(ChunkDelta{ToolCalls:[]ToolCallDelta{d}},nil))
	case protocol.EventUsage:
		if ev.Usage!=nil {
			if ev.Usage.InputTokens>0 { s.usage.PromptTokens=ev.Usage.InputTokens }
			if ev.Usage.OutputTokens>0 { s.usage.CompletionTokens=ev.Usage.OutputTokens }
			s.usage.TotalTokens=s.usage.PromptTokens+s.usage.CompletionTokens
		}
	case protocol.EventMessageEnd:
		ensureStarted(); finish:="stop"; if s.toolSeen { finish="tool_calls" }
		chunks=append(chunks,s.chunk(ChunkDelta{},&finish))
		if s.IncludeUsage { chunks=append(chunks,Chunk{ID:s.ID,Object:"chat.completion.chunk",Created:s.Created,Model:s.Model,Choices:[]ChunkChoice{},Usage:&s.usage}) }
	}
	return chunks,nil
}

func (s *StreamEncoder) chunk(delta ChunkDelta,finish *string) Chunk {
	return Chunk{ID:s.ID,Object:"chat.completion.chunk",Created:s.Created,Model:s.Model,Choices:[]ChunkChoice{{Index:0,Delta:delta,FinishReason:finish}}}
}

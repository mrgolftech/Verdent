package openai

import (
	"strings"

	"github.com/mrgolftech/Verdent/internal/protocol"
)

type toolAccumulator struct { id,name string; arguments strings.Builder }

type Assembler struct {
	content strings.Builder
	reasoning strings.Builder
	tools []toolAccumulator
	toolByProviderIndex map[int]int
	usage Usage
}

func NewAssembler() *Assembler { return &Assembler{toolByProviderIndex:make(map[int]int)} }

func (a *Assembler) Consume(ev protocol.Event) {
	switch ev.Type {
	case protocol.EventTextDelta:
		a.content.WriteString(ev.Text)
	case protocol.EventThinking:
		a.reasoning.WriteString(ev.Text)
	case protocol.EventToolStart:
		idx:=len(a.tools); a.toolByProviderIndex[ev.Index]=idx
		t:=toolAccumulator{id:ev.ToolID,name:ev.ToolName}
		if len(ev.Arguments)>0 { t.arguments.Write(ev.Arguments) }
		a.tools=append(a.tools,t)
	case protocol.EventToolDelta:
		if idx,ok:=a.toolByProviderIndex[ev.Index];ok { a.tools[idx].arguments.WriteString(ev.ArgumentDelta) }
	case protocol.EventUsage:
		if ev.Usage!=nil {
			if ev.Usage.InputTokens>0 { a.usage.PromptTokens=ev.Usage.InputTokens }
			if ev.Usage.OutputTokens>0 { a.usage.CompletionTokens=ev.Usage.OutputTokens }
			a.usage.TotalTokens=a.usage.PromptTokens+a.usage.CompletionTokens
		}
	}
}

func (a *Assembler) Completion(id,model string,created int64) Completion {
	content:=a.content.String()
	msg:=CompletionMessage{Role:"assistant",ReasoningContent:a.reasoning.String()}
	if content!="" { msg.Content=&content }
	for _,t:=range a.tools {
		args:=t.arguments.String(); if args=="" { args="{}" }
		msg.ToolCalls=append(msg.ToolCalls,ToolCall{ID:t.id,Type:"function",Function:FunctionCall{Name:t.name,Arguments:args}})
	}
	finish:="stop"; if len(msg.ToolCalls)>0 { finish="tool_calls" }
	return Completion{ID:id,Object:"chat.completion",Created:created,Model:model,Choices:[]CompletionChoice{{Index:0,Message:msg,FinishReason:finish}},Usage:a.usage}
}

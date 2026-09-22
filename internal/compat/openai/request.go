package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mrgolftech/Verdent/internal/canonical"
)

func ToCanonical(in ChatRequest) (canonical.Request, error) {
	if strings.TrimSpace(in.Model) == "" { return canonical.Request{}, errors.New("model is required") }
	if len(in.Messages) == 0 { return canonical.Request{}, errors.New("messages are required") }

	out := canonical.Request{Model: in.Model, Temperature: in.Temperature, Effort: in.ReasoningEffort}
	if in.MaxCompletionTokens > 0 { out.MaxTokens = in.MaxCompletionTokens } else { out.MaxTokens = in.MaxTokens }

	for _, tool := range in.Tools {
		if tool.Type != "" && tool.Type != "function" { return canonical.Request{}, fmt.Errorf("unsupported tool type %q", tool.Type) }
		if strings.TrimSpace(tool.Function.Name) == "" { return canonical.Request{}, errors.New("function tool name is required") }
		schema := tool.Function.Parameters
		if len(schema) == 0 || bytes.Equal(bytes.TrimSpace(schema), []byte("null")) { schema = json.RawMessage(`{"type":"object","properties":{}}`) }
		if !json.Valid(schema) { return canonical.Request{}, fmt.Errorf("invalid JSON schema for tool %q", tool.Function.Name) }
		out.Tools = append(out.Tools, canonical.Tool{Name:tool.Function.Name, Description:tool.Function.Description, InputSchema:schema})
	}
	choice, err := parseToolChoice(in.ToolChoice)
	if err != nil { return canonical.Request{}, err }
	out.ToolChoice = choice

	for _, msg := range in.Messages {
		blocks, err := parseMessageBlocks(msg)
		if err != nil { return canonical.Request{}, err }
		switch msg.Role {
		case "system", "developer":
			out.System = append(out.System, blocks...)
		case "user":
			out.Messages = append(out.Messages, canonical.Message{Role:canonical.RoleUser, Content:blocks})
			if q := lastText(blocks); q != "" { out.UserQuery = q }
		case "assistant":
			out.Messages = append(out.Messages, canonical.Message{Role:canonical.RoleAssistant, Content:blocks})
		case "tool":
			if msg.ToolCallID == "" { return canonical.Request{}, errors.New("tool message requires tool_call_id") }
			text := blocksText(blocks)
			out.Messages = append(out.Messages, canonical.Message{Role:canonical.RoleUser, Content:[]canonical.ContentBlock{{Type:canonical.BlockToolResult, ToolID:msg.ToolCallID, Text:text}}})
		default:
			return canonical.Request{}, fmt.Errorf("unsupported message role %q", msg.Role)
		}
	}
	if len(out.Messages) == 0 { return canonical.Request{}, errors.New("conversation contains no user/assistant messages") }
	return out, nil
}

func parseMessageBlocks(msg Message) ([]canonical.ContentBlock, error) {
	blocks, err := parseContent(msg.Content)
	if err != nil { return nil, fmt.Errorf("%s message content: %w", msg.Role, err) }
	if msg.Role == "assistant" {
		for _, call := range msg.ToolCalls {
			if call.ID == "" || call.Function.Name == "" { return nil, errors.New("assistant tool call requires id and function name") }
			args := strings.TrimSpace(call.Function.Arguments)
			if args == "" { args = "{}" }
			if !json.Valid([]byte(args)) { return nil, fmt.Errorf("tool call %q has invalid JSON arguments", call.Function.Name) }
			blocks = append(blocks, canonical.ContentBlock{Type:canonical.BlockToolUse, ToolID:call.ID, ToolName:call.Function.Name, Input:json.RawMessage(args)})
		}
	}
	return blocks, nil
}

func parseContent(raw json.RawMessage) ([]canonical.ContentBlock, error) {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) { return nil, nil }
	if trim[0] == '"' {
		var s string
		if err := json.Unmarshal(trim, &s); err != nil { return nil, err }
		if s == "" { return nil, nil }
		return []canonical.ContentBlock{{Type:canonical.BlockText, Text:s}}, nil
	}
	if trim[0] != '[' { return nil, errors.New("content must be a string, array, or null") }
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
		ImageURL json.RawMessage `json:"image_url"`
	}
	if err := json.Unmarshal(trim, &parts); err != nil { return nil, err }
	out := make([]canonical.ContentBlock,0,len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text", "input_text":
			if part.Text != "" { out=append(out,canonical.ContentBlock{Type:canonical.BlockText,Text:part.Text}) }
		case "image_url":
			url, err := imageURL(part.ImageURL)
			if err != nil { return nil, err }
			if url != "" { out=append(out,canonical.ContentBlock{Type:canonical.BlockImage,URL:url}) }
		default:
			return nil, fmt.Errorf("unsupported content part type %q", part.Type)
		}
	}
	return out,nil
}

func imageURL(raw json.RawMessage) (string,error) {
	trim:=bytes.TrimSpace(raw)
	if len(trim)==0 || bytes.Equal(trim,[]byte("null")) { return "",nil }
	if trim[0]=='"' { var s string; if err:=json.Unmarshal(trim,&s); err!=nil{return "",err}; return s,nil }
	var obj struct{ URL string `json:"url"` }
	if err:=json.Unmarshal(trim,&obj);err!=nil{return "",err}
	return obj.URL,nil
}

func parseToolChoice(raw json.RawMessage) (*canonical.ToolChoice,error) {
	trim:=bytes.TrimSpace(raw)
	if len(trim)==0 || bytes.Equal(trim,[]byte("null")) { return nil,nil }
	if trim[0]=='"' {
		var s string; if err:=json.Unmarshal(trim,&s);err!=nil{return nil,err}
		switch s {
		case "auto": return &canonical.ToolChoice{Type:canonical.ToolChoiceAuto},nil
		case "none": return &canonical.ToolChoice{Type:canonical.ToolChoiceNone},nil
		case "required": return &canonical.ToolChoice{Type:canonical.ToolChoiceAny},nil
		default: return nil,fmt.Errorf("unsupported tool_choice %q",s)
		}
	}
	var obj struct { Type string `json:"type"`; Function struct{Name string `json:"name"`} `json:"function"` }
	if err:=json.Unmarshal(trim,&obj);err!=nil{return nil,err}
	if obj.Type!="function" || obj.Function.Name=="" { return nil,errors.New("only function tool_choice objects are supported") }
	return &canonical.ToolChoice{Type:canonical.ToolChoiceTool,Name:obj.Function.Name},nil
}

func lastText(blocks []canonical.ContentBlock) string {
	for i:=len(blocks)-1;i>=0;i-- { if blocks[i].Type==canonical.BlockText && blocks[i].Text!="" { return blocks[i].Text } }
	return ""
}

func blocksText(blocks []canonical.ContentBlock) string {
	var b strings.Builder
	for _,block:=range blocks { if block.Type==canonical.BlockText { b.WriteString(block.Text) } }
	return b.String()
}

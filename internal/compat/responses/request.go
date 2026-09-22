package responses

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mrgolftech/Verdent/internal/canonical"
)

type Conversion struct {
	Request     canonical.Request
	CustomTools map[string]bool
}

func ToCanonical(in Request) (Conversion, error) {
	if strings.TrimSpace(in.Model) == "" {
		return Conversion{}, errors.New("model is required")
	}
	if in.PreviousResponseID != "" {
		return Conversion{}, errors.New("previous_response_id is not supported yet; resend full input history")
	}

	out := canonical.Request{
		Model:       in.Model,
		MaxTokens:   in.MaxOutputTokens,
		Temperature: in.Temperature,
	}
	if in.Reasoning != nil {
		out.Effort = in.Reasoning.Effort
	}
	if strings.TrimSpace(in.Instructions) != "" {
		out.System = []canonical.ContentBlock{{Type: canonical.BlockText, Text: in.Instructions}}
	}

	custom := map[string]bool{}
	for _, tool := range in.Tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			return Conversion{}, errors.New("tool name is required")
		}
		switch tool.Type {
		case "function":
			schema := tool.Parameters
			if len(bytes.TrimSpace(schema)) == 0 {
				schema = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			if !json.Valid(schema) {
				return Conversion{}, fmt.Errorf("invalid parameters schema for tool %q", name)
			}
			out.Tools = append(out.Tools, canonical.Tool{
				Kind: canonical.ToolKindFunction, Name: name,
				Description: tool.Description, InputSchema: schema,
			})
		case "custom":
			if name != "apply_patch" {
				return Conversion{}, fmt.Errorf("custom tool %q is not supported yet; only native apply_patch is enabled", name)
			}
			custom[name] = true
			out.Tools = append(out.Tools, canonical.Tool{
				Kind: canonical.ToolKindCustom, Name: name,
				Description: tool.Description, Format: tool.Format,
			})
		default:
			return Conversion{}, fmt.Errorf("unsupported Responses tool type %q", tool.Type)
		}
	}

	choice, err := parseToolChoice(in.ToolChoice)
	if err != nil {
		return Conversion{}, err
	}
	out.ToolChoice = choice

	messages, query, err := parseInput(in.Input)
	if err != nil {
		return Conversion{}, err
	}
	out.Messages = messages
	out.UserQuery = query
	if len(out.Messages) == 0 {
		return Conversion{}, errors.New("input contains no conversation items")
	}

	return Conversion{Request: out, CustomTools: custom}, nil
}

func parseToolChoice(raw json.RawMessage) (*canonical.ToolChoice, error) {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return nil, nil
	}
	if trim[0] == '"' {
		var value string
		if err := json.Unmarshal(trim, &value); err != nil {
			return nil, err
		}
		switch value {
		case "auto":
			return &canonical.ToolChoice{Type: canonical.ToolChoiceAuto}, nil
		case "none":
			return &canonical.ToolChoice{Type: canonical.ToolChoiceNone}, nil
		case "required":
			return &canonical.ToolChoice{Type: canonical.ToolChoiceAny}, nil
		default:
			return nil, fmt.Errorf("unsupported tool_choice %q", value)
		}
	}

	var obj struct {
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(trim, &obj); err != nil {
		return nil, err
	}
	if (obj.Type == "function" || obj.Type == "custom") && obj.Name != "" {
		return &canonical.ToolChoice{Type: canonical.ToolChoiceTool, Name: obj.Name}, nil
	}
	return nil, errors.New("unsupported tool_choice object")
}

func parseInput(raw json.RawMessage) ([]canonical.Message, string, error) {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return nil, "", nil
	}
	if trim[0] == '"' {
		var text string
		if err := json.Unmarshal(trim, &text); err != nil {
			return nil, "", err
		}
		return []canonical.Message{{
			Role: canonical.RoleUser,
			Content: []canonical.ContentBlock{{Type: canonical.BlockText, Text: text}},
		}}, text, nil
	}
	if trim[0] != '[' {
		return nil, "", errors.New("input must be a string or array")
	}

	var items []json.RawMessage
	if err := json.Unmarshal(trim, &items); err != nil {
		return nil, "", err
	}

	var out []canonical.Message
	query := ""
	for _, rawItem := range items {
		var head struct {
			Type string `json:"type"`
			Role string `json:"role"`
		}
		if err := json.Unmarshal(rawItem, &head); err != nil {
			return nil, "", err
		}

		switch head.Type {
		case "message":
			var item struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			}
			if err := json.Unmarshal(rawItem, &item); err != nil {
				return nil, "", err
			}
			blocks, err := parseContent(item.Content)
			if err != nil {
				return nil, "", err
			}
			role := canonical.RoleUser
			switch item.Role {
			case "assistant":
				role = canonical.RoleAssistant
			case "user", "developer", "system":
				role = canonical.RoleUser
			default:
				return nil, "", fmt.Errorf("unsupported message role %q", item.Role)
			}
			out = append(out, canonical.Message{Role: role, Content: blocks})
			if role == canonical.RoleUser {
				if text := lastText(blocks); text != "" {
					query = text
				}
			}

		case "function_call":
			var item struct {
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if err := json.Unmarshal(rawItem, &item); err != nil {
				return nil, "", err
			}
			if item.CallID == "" || item.Name == "" {
				return nil, "", errors.New("function_call requires call_id and name")
			}
			args := strings.TrimSpace(item.Arguments)
			if args == "" {
				args = "{}"
			}
			if !json.Valid([]byte(args)) {
				return nil, "", fmt.Errorf("function_call %q has invalid JSON arguments", item.Name)
			}
			out = append(out, canonical.Message{
				Role: canonical.RoleAssistant,
				Content: []canonical.ContentBlock{{
					Type: canonical.BlockToolUse, ToolID: item.CallID,
					ToolName: item.Name, Input: json.RawMessage(args),
				}},
			})

		case "function_call_output":
			var item struct {
				CallID string          `json:"call_id"`
				Output json.RawMessage `json:"output"`
			}
			if err := json.Unmarshal(rawItem, &item); err != nil {
				return nil, "", err
			}
			if item.CallID == "" {
				return nil, "", errors.New("function_call_output requires call_id")
			}
			text, err := outputText(item.Output)
			if err != nil {
				return nil, "", err
			}
			out = append(out, canonical.Message{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.BlockToolResult, ToolID: item.CallID, Text: text,
				}},
			})

		case "custom_tool_call":
			var item struct {
				CallID string `json:"call_id"`
				Name   string `json:"name"`
				Input  string `json:"input"`
			}
			if err := json.Unmarshal(rawItem, &item); err != nil {
				return nil, "", err
			}
			if item.CallID == "" || item.Name == "" {
				return nil, "", errors.New("custom_tool_call requires call_id and name")
			}
			wrapped, _ := json.Marshal(map[string]string{"input": item.Input})
			out = append(out, canonical.Message{
				Role: canonical.RoleAssistant,
				Content: []canonical.ContentBlock{{
					Type: canonical.BlockToolUse, ToolID: item.CallID,
					ToolName: item.Name, Input: wrapped,
				}},
			})

		case "custom_tool_call_output":
			var item struct {
				CallID string          `json:"call_id"`
				Output json.RawMessage `json:"output"`
			}
			if err := json.Unmarshal(rawItem, &item); err != nil {
				return nil, "", err
			}
			if item.CallID == "" {
				return nil, "", errors.New("custom_tool_call_output requires call_id")
			}
			text, err := outputText(item.Output)
			if err != nil {
				return nil, "", err
			}
			out = append(out, canonical.Message{
				Role: canonical.RoleUser,
				Content: []canonical.ContentBlock{{
					Type: canonical.BlockToolResult, ToolID: item.CallID, Text: text,
				}},
			})

		case "reasoning":
			continue

		default:
			return nil, "", fmt.Errorf("unsupported Responses input item type %q", head.Type)
		}
	}

	return out, query, nil
}

func parseContent(raw json.RawMessage) ([]canonical.ContentBlock, error) {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return nil, nil
	}
	if trim[0] == '"' {
		var text string
		if err := json.Unmarshal(trim, &text); err != nil {
			return nil, err
		}
		return []canonical.ContentBlock{{Type: canonical.BlockText, Text: text}}, nil
	}

	var parts []struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		ImageURL string `json:"image_url"`
	}
	if err := json.Unmarshal(trim, &parts); err != nil {
		return nil, err
	}

	out := make([]canonical.ContentBlock, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text", "text":
			out = append(out, canonical.ContentBlock{Type: canonical.BlockText, Text: part.Text})
		case "input_image":
			if part.ImageURL != "" {
				out = append(out, canonical.ContentBlock{Type: canonical.BlockImage, URL: part.ImageURL})
			}
		default:
			return nil, fmt.Errorf("unsupported content type %q", part.Type)
		}
	}
	return out, nil
}

func outputText(raw json.RawMessage) (string, error) {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return "", nil
	}
	if trim[0] == '"' {
		var text string
		if err := json.Unmarshal(trim, &text); err != nil {
			return "", err
		}
		return text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(trim, &parts); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, part := range parts {
		if part.Text != "" {
			b.WriteString(part.Text)
		}
	}
	return b.String(), nil
}

func lastText(blocks []canonical.ContentBlock) string {
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Type == canonical.BlockText && blocks[i].Text != "" {
			return blocks[i].Text
		}
	}
	return ""
}

func customInput(raw string) string {
	trim := strings.TrimSpace(raw)
	if trim == "" {
		return ""
	}
	var obj map[string]any
	if json.Unmarshal([]byte(trim), &obj) == nil {
		if value, ok := obj["input"].(string); ok {
			return value
		}
	}
	var value string
	if json.Unmarshal([]byte(trim), &value) == nil {
		return value
	}

	// Verdent's outer SSE JSON decoding can turn escaped control characters
	// inside partial_json into literal newlines/tabs, making the inner JSON
	// fragment formally invalid. Native freeform tools such as apply_patch
	// still need the exact payload, so unwrap the common {"input":"..."} shape
	// and re-escape raw control characters before decoding the JSON string.
	const prefix = `{"input":"`
	const suffix = `"}`
	if strings.HasPrefix(trim, prefix) && strings.HasSuffix(trim, suffix) {
		inner := trim[len(prefix) : len(trim)-len(suffix)]
		escaped := strings.NewReplacer(
			"\r", "\\r",
			"\n", "\\n",
			"\t", "\\t",
		).Replace(inner)
		if json.Unmarshal([]byte(`"`+escaped+`"`), &value) == nil {
			return value
		}
		return inner
	}
	return raw
}

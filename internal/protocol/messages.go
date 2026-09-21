package protocol

import "github.com/mrgolftech/Verdent/internal/canonical"

const trailingContinuationText = "Please continue."

func normalizeMessages(in []canonical.Message) []canonical.Message {
	out := make([]canonical.Message, 0, len(in)+1)
	for _, msg := range in {
		if len(msg.Content) == 0 { continue }
		if len(out) > 0 && out[len(out)-1].Role == msg.Role {
			out[len(out)-1].Content = append(out[len(out)-1].Content, msg.Content...)
			continue
		}
		copyMsg := canonical.Message{Role: msg.Role, Content: append([]canonical.ContentBlock(nil), msg.Content...)}
		out = append(out, copyMsg)
	}
	if len(out) == 0 { return out }
	last := out[len(out)-1]
	if last.Role == canonical.RoleAssistant && !hasToolUse(last.Content) {
		out = append(out, canonical.Message{
			Role: canonical.RoleUser,
			Content: []canonical.ContentBlock{{Type: canonical.BlockText, Text: trailingContinuationText}},
		})
	}
	return out
}

func hasToolUse(blocks []canonical.ContentBlock) bool {
	for _, b := range blocks {
		if b.Type == canonical.BlockToolUse { return true }
	}
	return false
}

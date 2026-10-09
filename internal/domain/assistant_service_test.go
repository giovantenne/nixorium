package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func serviceMessagesFixture() []AssistantMessage {
	return []AssistantMessage{
		{Role: "assistant", ToolCalls: []AssistantToolCall{{ID: "core1", Name: "read_core", Arguments: json.RawMessage(`{"path":"modules"}`)}}},
		{Role: "tool", ToolCallID: "core1", Content: `{"coreRef":{"rev":"` + strings.Repeat("a", 40) + `","sha256":"` + strings.Repeat("b", 64) + `"}}`},
	}
}

func TestValidAssistantServiceMessages(t *testing.T) {
	valid := serviceMessagesFixture()
	if !ValidAssistantServiceMessages(valid) || !ValidAssistantServiceMessages(append(valid, []AssistantMessage{
		{Role: "assistant", ToolCalls: []AssistantToolCall{{ID: "core2", Name: "read_core", Arguments: json.RawMessage(`{}`)}}},
		{Role: "tool", ToolCallID: "core2", Content: valid[1].Content},
	}...)) {
		t.Fatal("valid service blocks rejected")
	}
	mutations := map[string]func([]AssistantMessage) []AssistantMessage{
		"empty":             func([]AssistantMessage) []AssistantMessage { return nil },
		"missing tool":      func(m []AssistantMessage) []AssistantMessage { return m[:1] },
		"assistant role":    func(m []AssistantMessage) []AssistantMessage { m[0].Role = "user"; return m },
		"assistant content": func(m []AssistantMessage) []AssistantMessage { m[0].Content = "text"; return m },
		"no calls":          func(m []AssistantMessage) []AssistantMessage { m[0].ToolCalls = nil; return m },
		"local tool":        func(m []AssistantMessage) []AssistantMessage { m[0].ToolCalls[0].Name = "read_file"; return m },
		"large arguments": func(m []AssistantMessage) []AssistantMessage {
			m[0].ToolCalls[0].Arguments = json.RawMessage(`{"path":"` + strings.Repeat("x", 4096) + `"}`)
			return m
		},
		"invalid arguments": func(m []AssistantMessage) []AssistantMessage {
			m[0].ToolCalls[0].Arguments = json.RawMessage(`{`)
			return m
		},
		"tool role": func(m []AssistantMessage) []AssistantMessage { m[1].Role = "assistant"; return m },
		"wrong id":  func(m []AssistantMessage) []AssistantMessage { m[1].ToolCallID = "other"; return m },
		"extra key": func(m []AssistantMessage) []AssistantMessage {
			m[1].Content = strings.Replace(m[1].Content, `{"coreRef":`, `{"extra":1,"coreRef":`, 1)
			return m
		},
		"nested extra key": func(m []AssistantMessage) []AssistantMessage {
			m[1].Content = strings.Replace(m[1].Content, `{"rev":`, `{"extra":"x","rev":`, 1)
			return m
		},
		"invalid rev": func(m []AssistantMessage) []AssistantMessage {
			m[1].Content = strings.Replace(m[1].Content, strings.Repeat("a", 40), "bad", 1)
			return m
		},
		"invalid hash": func(m []AssistantMessage) []AssistantMessage {
			m[1].Content = strings.Replace(m[1].Content, strings.Repeat("b", 64), strings.Repeat("B", 64), 1)
			return m
		},
		"large content": func(m []AssistantMessage) []AssistantMessage { m[1].Content += strings.Repeat(" ", 1024); return m },
		"duplicate key": func(m []AssistantMessage) []AssistantMessage {
			m[1].Content = strings.Replace(m[1].Content, `{"rev":`, `{"rev":"bad","rev":`, 1)
			return m
		},
		"too many messages": func(m []AssistantMessage) []AssistantMessage { return append(m, make([]AssistantMessage, 63)...) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			if ValidAssistantServiceMessages(mutate(serviceMessagesFixture())) {
				t.Fatal("invalid service messages accepted")
			}
		})
	}
}

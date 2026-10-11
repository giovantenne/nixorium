package domain

import "strings"

// AssistantFinalAnswer is shared by the transcript writer and the visible chat.
func AssistantFinalAnswer(text, summary string) string {
	if summary == "" || strings.Contains(text, summary) {
		return text
	}
	if text == "" {
		return summary
	}
	return text + "\n\n" + summary
}

func AssistantHistoryText(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

func AssistantConversationTitle(messages []AssistantMessage) string {
	for _, message := range messages {
		if message.Role == "user" {
			line, _, _ := strings.Cut(message.Content, "\n")
			return AssistantHistoryText(strings.TrimSuffix(line, "\r"), 60)
		}
	}
	return ""
}

func AssistantPreviousAnswer(messages []AssistantMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role == "assistant" && strings.TrimSpace(m.Content) != "" && len(m.ToolCalls) == 0 {
			return AssistantHistoryText(m.Content, 2000)
		}
	}
	return ""
}

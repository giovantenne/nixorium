package domain

import "encoding/json"

// AssistantConfig describes the controller's installed, explicit opt-in.
type AssistantConfig struct {
	Enabled      bool
	Endpoint     string
	SubscribeURL string
}

type AssistantAccount struct {
	Name           string `json:"name"`
	Plan           string `json:"plan"`
	Status         string `json:"status,omitempty"`
	PeriodEnd      string `json:"periodEnd"`
	TurnsRemaining int    `json:"turnsRemaining,omitempty"`
}

type AssistantActivation struct {
	Token   string           `json:"token"`
	Account AssistantAccount `json:"account"`
}

type AssistantTurnLimits struct {
	Steps           int `json:"steps"`
	Seconds         int `json:"seconds"`
	MaxRequestBytes int `json:"maxRequestBytes"`
}

type AssistantTurn struct {
	TurnID string              `json:"turnId"`
	Limits AssistantTurnLimits `json:"limits"`
}

type AssistantToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type AssistantMessage struct {
	Role       string              `json:"role"`
	Content    string              `json:"content"`
	ToolCalls  []AssistantToolCall `json:"toolCalls,omitempty"`
	ToolCallID string              `json:"toolCallId,omitempty"`
}

type AssistantContext struct {
	Nixorium string   `json:"nixorium"`
	Hosts    []string `json:"hosts,omitempty"`
	Files    []string `json:"files"`
}

type AssistantStepRequest struct {
	Context  *AssistantContext  `json:"context,omitempty"`
	Messages []AssistantMessage `json:"messages"`
}

type AssistantEvent struct {
	Type         string
	Delta        string
	Text         string
	Calls        []AssistantToolCall
	Summary      string
	Code         string
	Message      string
	InputTokens  int
	OutputTokens int
}

type AssistantError struct {
	Status  int
	Code    string
	Message string
}

func (err *AssistantError) Error() string { return "Assistant service: " + err.Code }

type AssistantConsent struct {
	SchemaVersion int    `json:"schemaVersion"`
	AcceptedAt    string `json:"acceptedAt"`
	TextVersion   int    `json:"textVersion"`
}

const AssistantTextLimit = 128 * 1024

type AssistantConversation struct {
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Base       string `json:"base"`
	CreatedAt  string `json:"createdAt"`
}

type AssistantTurnResult struct {
	Summary  string
	Messages []AssistantMessage
}

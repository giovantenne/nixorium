package domain

import (
	"encoding/json"
	"regexp"
	"time"
)

// AssistantConfig describes the controller's installed Assistant configuration.
type AssistantConfig struct {
	Enabled      bool
	Endpoint     string
	SubscribeURL string
}

type AssistantAccount struct {
	Name           string `json:"name"`
	LabName        string `json:"labName,omitempty"`
	Plan           string `json:"plan"`
	Status         string `json:"status,omitempty"`
	PeriodEnd      string `json:"periodEnd"`
	TurnsRemaining int    `json:"turnsRemaining,omitempty"`
	// TrialRequests is the request allowance of a trial account, sent only by
	// services that announce configurable trials; zero means unknown.
	TrialRequests int    `json:"trialRequests,omitempty"`
	UpgradeURL    string `json:"upgradeUrl,omitempty"`
}

// AssistantOffer is announced by the service. TrialRequests is zero only for
// the installed fallback used when the offer cannot be fetched; TrialDays is
// zero when the service does not announce the trial length.
type AssistantOffer struct {
	PriceText     string `json:"priceText"`
	TrialRequests int    `json:"trialRequests"`
	TrialDays     int    `json:"trialDays,omitempty"`
	TrialURL      string `json:"trialUrl"`
	SubscribeURL  string `json:"subscribeUrl"`
}

// Bounds of the trial terms the client accepts from the service.
const (
	AssistantTrialRequestsMax = 1000
	AssistantTrialDaysMax     = 3650
)

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

type AssistantCore struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Rev   string `json:"rev"`
}

var assistantCoreName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)
var assistantCoreRevision = regexp.MustCompile(`^[0-9a-f]{40}$`)

func ValidAssistantCore(core AssistantCore) bool {
	return assistantCoreName.MatchString(core.Owner) && assistantCoreName.MatchString(core.Repo) && assistantCoreRevision.MatchString(core.Rev)
}

type AssistantContext struct {
	Nixorium string         `json:"nixorium"`
	Core     *AssistantCore `json:"core,omitempty"`
	Hosts    []string       `json:"hosts,omitempty"`
	Files    []string       `json:"files"`
	// Capabilities use an optional HTTP header, preserving old request bodies.
	Capabilities []string `json:"-"`
}

type AssistantStepRequest struct {
	Context  *AssistantContext  `json:"context,omitempty"`
	Messages []AssistantMessage `json:"messages"`
}

type AssistantEvent struct {
	// Local transcript metrics are never part of the service event protocol.
	TranscriptBytes int `json:"-"`
	MaxRequestBytes int `json:"-"`
	Type            string
	ToolName        string
	ToolResult      string
	Messages        []AssistantMessage `json:"messages,omitempty"`
	Delta           string
	Text            string
	Calls           []AssistantToolCall
	Summary         string
	Code            string
	Message         string
	InputTokens     int
	OutputTokens    int
}

var AssistantServiceTools = []string{"read_core"}

var assistantCoreHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

func ValidAssistantServiceMessages(messages []AssistantMessage) bool {
	if len(messages) == 0 || len(messages) > 256 {
		return false
	}
	ids := map[string]bool{}
	for index := 0; index < len(messages); {
		assistant := messages[index]
		if assistant.Role != "assistant" || assistant.Content != "" || assistant.ToolCallID != "" || len(assistant.ToolCalls) == 0 {
			return false
		}
		index++
		for _, call := range assistant.ToolCalls {
			args, err := json.Marshal(call.Arguments)
			if call.Name != "read_core" || call.ID == "" || ids[call.ID] || err != nil || len(args) > 4096 || len(call.Arguments) == 0 || index >= len(messages) {
				return false
			}
			ids[call.ID] = true
			tool := messages[index]
			if tool.Role != "tool" || tool.ToolCallID != call.ID || len(tool.ToolCalls) != 0 || ValidateStrictRemoteJSON([]byte(tool.Content), 1024) != nil {
				return false
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal([]byte(tool.Content), &fields) != nil || len(fields) != 1 || fields["coreRef"] == nil {
				return false
			}
			var reference map[string]string
			if json.Unmarshal(fields["coreRef"], &reference) != nil || len(reference) != 2 || !assistantCoreRevision.MatchString(reference["rev"]) || !assistantCoreHash.MatchString(reference["sha256"]) {
				return false
			}
			index++
		}
	}
	return true
}

type AssistantError struct {
	Status  int
	Code    string
	Message string
}

func (err *AssistantError) Error() string { return "Assistant service: " + err.Code }

type AssistantConsent struct {
	ShareConversations bool   `json:"shareConversations"`
	SchemaVersion      int    `json:"schemaVersion"`
	AcceptedAt         string `json:"acceptedAt"`
	TextVersion        int    `json:"textVersion"`
}

const AssistantTextLimit = 128 * 1024

type AssistantConversation struct {
	ID             string  `json:"id"`
	Repository     string  `json:"repository"`
	Base           string  `json:"base"`
	CreatedAt      string  `json:"createdAt"`
	LastActivityAt string  `json:"lastActivityAt,omitempty"`
	Title          string  `json:"title,omitempty"`
	SavedAt        string  `json:"savedAt,omitempty"`
	LastAnswer     *string `json:"lastAnswer,omitempty"`
}

type AssistantConversationHistory struct {
	Conversation AssistantConversation
	Messages     []AssistantMessage
	ChangedFiles int
	State        string
}

type AssistantTurnResult struct {
	Outcome  string
	Summary  string
	Messages []AssistantMessage
}

type AssistantEvaluation struct {
	OK           bool     `json:"ok"`
	ChangedHosts []string `json:"changedHosts"`
	Error        string   `json:"error,omitempty"`
}

type AssistantSavePlan struct {
	State          string                `json:"state"`
	Message        string                `json:"message"`
	Repository     string                `json:"repository"`
	ConversationID string                `json:"conversationId"`
	Base           string                `json:"base"`
	Added          []string              `json:"added"`
	Changed        []string              `json:"changed"`
	Removed        []string              `json:"removed"`
	ChangedHosts   []string              `json:"changedHosts"`
	Evaluation     AssistantEvaluation   `json:"evaluation"`
	Diff           string                `json:"diff"`
	ReviewToken    string                `json:"reviewToken"`
	ExpiresAt      time.Time             `json:"expiresAt"`
	Proposal       TemplateResetProposal `json:"-"`
}

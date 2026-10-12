package adapters

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

type AssistantClient struct {
	Endpoint string
	HTTP     *http.Client
	Version  string
}

var assistantTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43,256}$`)
var assistantTurnPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var assistantUpgradeExpiryPattern = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)
var assistantUpgradeSignaturePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func assistantProtocolError() error {
	return &domain.AssistantError{Code: "protocol_error", Message: "The Assistant service returned an invalid response."}
}

func assistantDecodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return assistantProtocolError()
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return assistantProtocolError()
	}
	return nil
}

func (client AssistantClient) request(ctx context.Context, method, path, token string, body any, timeout time.Duration) (*http.Response, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, assistantProtocolError()
		}
		if len(data) > 2000000 {
			return nil, &domain.AssistantError{Status: 413, Code: "request_too_large"}
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(client.Endpoint, "/")+path, bytes.NewReader(data))
	if err != nil {
		return nil, assistantProtocolError()
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if strings.HasSuffix(path, "/steps") {
		request.Header.Set("Accept", "text/event-stream")
	}
	if token != "" {
		if !assistantTokenPattern.MatchString(token) {
			return nil, &domain.AssistantError{Status: 401, Code: "invalid_token"}
		}
		request.Header.Set("Authorization", "Bearer "+token)
	}
	httpClient := http.Client{}
	if client.HTTP != nil {
		httpClient = *client.HTTP
	}
	httpClient.Timeout = timeout
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return httpClient.Do(request)
}

func assistantResponse(response *http.Response, expected int, target any) error {
	defer response.Body.Close()
	if expected == http.StatusNoContent && response.StatusCode == expected {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil {
		return err
	}
	if len(data) > 65536 {
		return assistantProtocolError()
	}
	if response.StatusCode != expected {
		var wire struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if assistantDecodeStrict(data, &wire) != nil || wire.Error.Code == "" {
			return assistantProtocolError()
		}
		return &domain.AssistantError{Status: response.StatusCode, Code: wire.Error.Code, Message: wire.Error.Message}
	}
	return assistantDecodeStrict(data, target)
}

func (client AssistantClient) Activate(ctx context.Context, code string) (domain.AssistantActivation, error) {
	var result domain.AssistantActivation
	body := struct {
		Code   string `json:"code"`
		Client struct {
			Nixorium string `json:"nixorium"`
		} `json:"client"`
	}{Code: code}
	body.Client.Nixorium = client.Version
	response, err := client.request(ctx, "POST", "/activate", "", body, 15*time.Second)
	if err != nil {
		return result, err
	}
	if err = assistantResponse(response, 200, &result); err != nil {
		return domain.AssistantActivation{}, err
	}
	if !assistantTokenPattern.MatchString(result.Token) || result.Account.Name == "" || result.Account.Plan == "" || result.Account.PeriodEnd == "" {
		return domain.AssistantActivation{}, assistantProtocolError()
	}
	return result, nil
}

func (client AssistantClient) Offer(ctx context.Context) (domain.AssistantOffer, error) {
	var offer domain.AssistantOffer
	response, err := client.request(ctx, "GET", "/offer", "", nil, 15*time.Second)
	if err != nil {
		return offer, err
	}
	if err = assistantResponse(response, 200, &offer); err != nil {
		return domain.AssistantOffer{}, err
	}
	validLink := func(value string) bool {
		link, err := url.Parse(value)
		return err == nil && link.Host != "" && link.User == nil && link.Fragment == "" && link.Opaque == "" &&
			(link.Scheme == "https" || (link.Scheme == "http" && (link.Hostname() == "localhost" || link.Hostname() == "127.0.0.1")))
	}
	if offer.TrialRequests != 5 || !validLink(offer.TrialURL) || !validLink(offer.SubscribeURL) {
		return domain.AssistantOffer{}, assistantProtocolError()
	}
	return offer, nil
}

func (client AssistantClient) Account(ctx context.Context, token string) (domain.AssistantAccount, error) {
	var wire struct {
		Account domain.AssistantAccount `json:"account"`
	}
	response, err := client.request(ctx, "GET", "/account", token, nil, 15*time.Second)
	if err != nil {
		return wire.Account, err
	}
	if err = assistantResponse(response, 200, &wire); err != nil {
		return domain.AssistantAccount{}, err
	}
	return client.checkedAccount(wire.Account)
}

func (client AssistantClient) checkedAccount(account domain.AssistantAccount) (domain.AssistantAccount, error) {
	if account.Status != "active" && account.Status != "inactive" {
		return domain.AssistantAccount{}, assistantProtocolError()
	}
	if account.UpgradeURL != "" && (account.Plan != "trial" || !client.validUpgradeURL(account.UpgradeURL)) {
		return domain.AssistantAccount{}, assistantProtocolError()
	}
	return account, nil
}

// SetLabName names this laboratory (an empty name clears it) and returns the
// refreshed account. The service trims and validates the text.
func (client AssistantClient) SetLabName(ctx context.Context, token, name string) (domain.AssistantAccount, error) {
	var wire struct {
		Account domain.AssistantAccount `json:"account"`
	}
	response, err := client.request(ctx, "PATCH", "/account", token, struct {
		LabName string `json:"labName"`
	}{name}, 15*time.Second)
	if err != nil {
		return domain.AssistantAccount{}, err
	}
	if err = assistantResponse(response, 200, &wire); err != nil {
		return domain.AssistantAccount{}, err
	}
	return client.checkedAccount(wire.Account)
}

func (client AssistantClient) validUpgradeURL(value string) bool {
	link, err := url.Parse(value)
	base, baseErr := url.Parse(client.Endpoint)
	if err != nil || baseErr != nil || len(value) > 2048 || link.User != nil || link.Fragment != "" || link.Opaque != "" ||
		link.Scheme != base.Scheme || link.Host != base.Host || link.Path != "/upgrade" || link.RawPath != "" {
		return false
	}
	if link.Scheme != "https" && !(link.Scheme == "http" && (link.Hostname() == "localhost" || link.Hostname() == "127.0.0.1")) {
		return false
	}
	query, err := url.ParseQuery(link.RawQuery)
	return err == nil && len(query) == 3 && len(query["account"]) == 1 && len(query["expires"]) == 1 && len(query["sig"]) == 1 &&
		assistantTurnPattern.MatchString(query.Get("account")) && assistantUpgradeExpiryPattern.MatchString(query.Get("expires")) &&
		assistantUpgradeSignaturePattern.MatchString(query.Get("sig"))
}

// BillingPortal creates a temporary Stripe session without opening an Assistant turn.
func (client AssistantClient) BillingPortal(ctx context.Context, token string) (string, error) {
	var wire struct {
		URL string `json:"url"`
	}
	response, err := client.request(ctx, "POST", "/billing-portal", token, nil, 15*time.Second)
	if err != nil {
		return "", err
	}
	if err = assistantResponse(response, 200, &wire); err != nil {
		return "", err
	}
	link, err := url.Parse(wire.URL)
	if err != nil || link.Scheme != "https" || link.Host != "billing.stripe.com" || link.User != nil || link.Fragment != "" || link.Opaque != "" {
		return "", assistantProtocolError()
	}
	return wire.URL, nil
}

func (client AssistantClient) OpenTurn(ctx context.Context, token string) (domain.AssistantTurn, error) {
	var result domain.AssistantTurn
	body := struct {
		Client struct {
			Nixorium string `json:"nixorium"`
			Tools    int    `json:"tools"`
		} `json:"client"`
	}{}
	body.Client.Nixorium, body.Client.Tools = client.Version, 1
	response, err := client.request(ctx, "POST", "/turns", token, body, 15*time.Second)
	if err != nil {
		return result, err
	}
	if err = assistantResponse(response, 201, &result); err != nil {
		return domain.AssistantTurn{}, err
	}
	if !assistantTurnPattern.MatchString(result.TurnID) || result.Limits.Steps < 1 || result.Limits.Steps > 24 || result.Limits.Seconds < 1 || result.Limits.Seconds > 900 || result.Limits.MaxRequestBytes < 1 || result.Limits.MaxRequestBytes > 2000000 {
		return domain.AssistantTurn{}, assistantProtocolError()
	}
	return result, nil
}

func (client AssistantClient) Cancel(ctx context.Context, token, turnID string) error {
	if !assistantTurnPattern.MatchString(turnID) {
		return assistantProtocolError()
	}
	response, err := client.request(ctx, "POST", "/turns/"+turnID+"/cancel", token, nil, 15*time.Second)
	if err != nil {
		return err
	}
	return assistantResponse(response, 204, nil)
}

func (client AssistantClient) Step(ctx context.Context, token, turnID string, request domain.AssistantStepRequest, emit func(domain.AssistantEvent)) error {
	if !assistantTurnPattern.MatchString(turnID) {
		return assistantProtocolError()
	}
	response, err := client.request(ctx, "POST", "/turns/"+turnID+"/steps", token, request, 5*time.Minute)
	if err != nil {
		return err
	}
	if response.StatusCode != 200 {
		return assistantResponse(response, 200, nil)
	}
	defer response.Body.Close()
	if strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]) != "text/event-stream" {
		return assistantProtocolError()
	}
	return parseAssistantSSE(response.Body, emit)
}

type assistantCountingReader struct {
	source io.Reader
	bytes  int
}

func (reader *assistantCountingReader) Read(buffer []byte) (int, error) {
	n, err := reader.source.Read(buffer)
	reader.bytes += n
	if reader.bytes > 4*1024*1024 {
		return n, assistantProtocolError()
	}
	return n, err
}

func parseAssistantSSE(source io.Reader, emit func(domain.AssistantEvent)) error {
	reader := &assistantCountingReader{source: io.LimitReader(source, 4*1024*1024+1)}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 256*1024+1)
	name, data := "", ""
	eventBytes, textBytes := 0, 0
	terminal := false
	serviceSeen := false
	dispatch := func() error {
		if name == "" && data == "" {
			eventBytes = 0
			return nil
		}
		if terminal || name == "" || data == "" {
			return assistantProtocolError()
		}
		if serviceSeen && name != "tool_calls" && name != "final" && name != "error" {
			return assistantProtocolError()
		}
		event, err := decodeAssistantEvent(name, []byte(strings.TrimSuffix(data, "\n")))
		if err != nil {
			return err
		}
		if name == "service_messages" {
			if serviceSeen {
				return assistantProtocolError()
			}
			serviceSeen = true
		}
		textBytes += len(event.Delta) + len(event.Summary)
		if textBytes > domain.AssistantTextLimit {
			return assistantProtocolError()
		}
		terminal = name == "tool_calls" || name == "final" || name == "error"
		emit(event)
		name, data, eventBytes = "", "", 0
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := dispatch(); err != nil {
				return err
			}
			continue
		}
		eventBytes += len(line) + 1
		if eventBytes > 256*1024 {
			return assistantProtocolError()
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			return assistantProtocolError()
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			if name != "" {
				return assistantProtocolError()
			}
			name = value
		case "data":
			data += value + "\n"
		default:
			return assistantProtocolError()
		}
	}
	if scanner.Err() != nil {
		return assistantProtocolError()
	}
	if name != "" || data != "" {
		return assistantProtocolError()
	}
	if !terminal {
		return assistantProtocolError()
	}
	return nil
}

func decodeAssistantEvent(name string, data []byte) (domain.AssistantEvent, error) {
	event := domain.AssistantEvent{Type: name}
	var target any
	switch name {
	case "text":
		target = &struct {
			Delta string `json:"delta"`
		}{}
	case "activity":
		target = &struct {
			Text string `json:"text"`
		}{}
	case "tool_calls":
		target = &struct {
			Calls []domain.AssistantToolCall `json:"calls"`
		}{}
	case "service_messages":
		target = &struct {
			Messages []domain.AssistantMessage `json:"messages"`
		}{}
	case "final":
		target = &struct {
			Summary string `json:"summary"`
		}{}
	case "usage":
		target = &struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
		}{}
	case "error":
		target = &struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}{}
	default:
		return event, assistantProtocolError()
	}
	if err := assistantDecodeStrict(data, target); err != nil {
		return event, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return event, assistantProtocolError()
	}
	required := map[string][]string{
		"text": {"delta"}, "activity": {"text"}, "tool_calls": {"calls"}, "final": {"summary"},
		"usage": {"inputTokens", "outputTokens"}, "error": {"code", "message"},
		"service_messages": {"messages"},
	}
	for _, field := range required[name] {
		value, exists := fields[field]
		if !exists || string(value) == "null" {
			return event, assistantProtocolError()
		}
	}
	// Decode again into the event payload only after its exact schema passed.
	var payload struct {
		Delta        string
		Text         string
		Calls        []domain.AssistantToolCall
		Messages     []domain.AssistantMessage
		Summary      string
		Code         string
		Message      string
		InputTokens  int
		OutputTokens int
	}
	if json.Unmarshal(data, &payload) != nil {
		return event, assistantProtocolError()
	}
	event.Delta, event.Text, event.Calls, event.Summary, event.Code, event.Message, event.InputTokens, event.OutputTokens = payload.Delta, payload.Text, payload.Calls, payload.Summary, payload.Code, payload.Message, payload.InputTokens, payload.OutputTokens
	event.Messages = payload.Messages
	if name == "usage" && (event.InputTokens < 0 || event.OutputTokens < 0) {
		return event, assistantProtocolError()
	}
	if name == "error" && event.Code == "" {
		return event, assistantProtocolError()
	}
	if name == "tool_calls" {
		if len(event.Calls) == 0 {
			return event, assistantProtocolError()
		}
		ids := map[string]bool{}
		for _, call := range event.Calls {
			if call.ID == "" || ids[call.ID] {
				return event, assistantProtocolError()
			}
			ids[call.ID] = true
			known := false
			for _, tool := range domain.AssistantToolsV1() {
				if tool.Name == call.Name {
					known = true
					break
				}
			}
			if !known {
				return event, assistantProtocolError()
			}
			if len(call.Arguments) == 0 {
				return event, assistantProtocolError()
			}
		}
	}
	return event, nil
}

func (client AssistantClient) ShareConversation(ctx context.Context, token, id string, payload domain.AssistantStepRequest) error {
	if !assistantTurnPattern.MatchString(id) {
		return assistantProtocolError()
	}
	response, err := client.request(ctx, "POST", "/conversations/"+id, token, payload, 15*time.Second)
	if err != nil {
		return err
	}
	return assistantResponse(response, 204, nil)
}
func (client AssistantClient) ReportConversation(ctx context.Context, token, id, rating, comment string, payload domain.AssistantStepRequest) error {
	if !assistantTurnPattern.MatchString(id) || (rating != "helpful" && rating != "wrong") || len([]rune(comment)) > 500 {
		return assistantProtocolError()
	}
	body := struct {
		Conversation string `json:"conversation"`
		Rating       string `json:"rating"`
		Comment      string `json:"comment"`
		domain.AssistantStepRequest
	}{id, rating, comment, payload}
	response, err := client.request(ctx, "POST", "/feedback", token, body, 15*time.Second)
	if err != nil {
		return err
	}
	return assistantResponse(response, 204, nil)
}

package adapters

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
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
var assistantShortUpgradePathPattern = regexp.MustCompile(`^/u/[A-Za-z0-9_-]{40}$`)
var assistantClientVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]{0,63}$`)

// assistantProtocolError reports data the client refuses to accept. The reason
// is a short, specific, private diagnostic for the controller; it is never sent
// to the service.
func assistantProtocolError(reason string) error {
	return &domain.AssistantError{Code: "protocol_error", Message: "The Assistant service returned an invalid response.", Detail: assistantErrorDetail(reason)}
}

// assistantInterruptedError reports a stream that ended before the answer was
// complete. It is a service interruption, not invalid data.
func assistantInterruptedError(reason string) error {
	return &domain.AssistantError{Code: "service_interrupted", Message: "The Assistant service interrupted the answer. Send the message again.", Detail: assistantErrorDetail(reason)}
}

// assistantErrorDetail keeps a reason short and printable: it may quote a field
// name chosen by the service.
func assistantErrorDetail(reason string) string {
	var out []rune
	for _, r := range reason {
		if r < ' ' || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			r = '?'
		}
		out = append(out, r)
		if len(out) == 160 {
			break
		}
	}
	return string(out)
}

// assistantDecodeReason returns "" when data is exactly one JSON value that fits
// target without unknown fields, otherwise a short reason.
func assistantDecodeReason(data []byte, target any) string {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var typeError *json.UnmarshalTypeError
		var syntaxError *json.SyntaxError
		switch {
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			return "unexpected field " + strings.TrimPrefix(err.Error(), "json: unknown field ")
		case errors.As(err, &typeError) && typeError.Field != "":
			return "field " + typeError.Field + " has the wrong type"
		case errors.As(err, &typeError):
			return "value has the wrong type"
		case errors.As(err, &syntaxError), errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
			return "malformed JSON"
		}
		return "JSON does not match the expected shape"
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return "extra data after the JSON value"
	}
	return ""
}

func assistantDecodeStrict(data []byte, target any) error {
	if reason := assistantDecodeReason(data, target); reason != "" {
		return assistantProtocolError(reason)
	}
	return nil
}

func (client AssistantClient) request(ctx context.Context, method, path, token string, body any, timeout time.Duration) (*http.Response, error) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, assistantProtocolError("request body cannot be encoded")
		}
		if len(data) > 2000000 {
			return nil, &domain.AssistantError{Status: 413, Code: "request_too_large"}
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(client.Endpoint, "/")+path, bytes.NewReader(data))
	if err != nil {
		return nil, assistantProtocolError("request cannot be built")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	// The version tells the service that this client accepts any announced trial
	// size and the optional trial fields; older controllers send no header.
	if assistantClientVersionPattern.MatchString(client.Version) {
		request.Header.Set("X-Nixorium-Client-Version", client.Version)
	}
	if strings.HasSuffix(path, "/steps") {
		request.Header.Set("Accept", "text/event-stream")
		if step, ok := body.(domain.AssistantStepRequest); ok && step.Context != nil {
			capabilities := []string{}
			seen := map[string]bool{}
			for _, capability := range step.Context.Capabilities {
				if (capability == "prepare_npm_v1" || capability == "host_diagnostics_v1" || capability == "git_history_v1" || capability == "prefetch_v1" || capability == "home_files_v1") && !seen[capability] {
					capabilities = append(capabilities, capability)
					seen[capability] = true
				}
			}
			if len(capabilities) > 0 {
				request.Header.Set("X-Nixorium-Capabilities", strings.Join(capabilities, ","))
			}
		}
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
		return assistantProtocolError("response body exceeds 64 KiB")
	}
	if response.StatusCode != expected {
		var wire struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if reason := assistantDecodeReason(data, &wire); reason != "" {
			return assistantProtocolError("error response: " + reason)
		}
		if wire.Error.Code == "" {
			return assistantProtocolError("error response has no code")
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
	if !assistantTokenPattern.MatchString(result.Token) || result.Account.Name == "" || result.Account.Plan == "" || result.Account.PeriodEnd == "" ||
		!validAssistantTrialRequests(result.Account) {
		return domain.AssistantActivation{}, assistantProtocolError("activation response has an invalid token or account")
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
	if offer.TrialRequests < 1 || offer.TrialRequests > domain.AssistantTrialRequestsMax ||
		offer.TrialDays < 0 || offer.TrialDays > domain.AssistantTrialDaysMax {
		return domain.AssistantOffer{}, assistantProtocolError("offer out of range")
	}
	if !validLink(offer.TrialURL) || !validLink(offer.SubscribeURL) {
		return domain.AssistantOffer{}, assistantProtocolError("offer link is not valid")
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
		return domain.AssistantAccount{}, assistantProtocolError("account status is not active or inactive")
	}
	if account.UpgradeURL != "" && (account.Plan != "trial" || !client.validUpgradeURL(account.UpgradeURL)) {
		return domain.AssistantAccount{}, assistantProtocolError("account upgrade link is not valid")
	}
	if !validAssistantTrialRequests(account) {
		return domain.AssistantAccount{}, assistantProtocolError("account trial allowance is out of range")
	}
	return account, nil
}

// validAssistantTrialRequests accepts an absent allowance or one within the
// announced trial range on a trial account.
func validAssistantTrialRequests(account domain.AssistantAccount) bool {
	return account.TrialRequests == 0 ||
		(account.Plan == "trial" && account.TrialRequests > 0 && account.TrialRequests <= domain.AssistantTrialRequestsMax)
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
		link.Scheme != base.Scheme || link.Host != base.Host || link.RawPath != "" {
		return false
	}
	if link.Scheme != "https" && !(link.Scheme == "http" && (link.Hostname() == "localhost" || link.Hostname() == "127.0.0.1")) {
		return false
	}
	if assistantShortUpgradePathPattern.MatchString(link.Path) {
		return link.RawQuery == "" && !link.ForceQuery
	}
	if link.Path != "/upgrade" {
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
		return "", assistantProtocolError("billing portal link is not valid")
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
	if !assistantTurnPattern.MatchString(result.TurnID) {
		return domain.AssistantTurn{}, assistantProtocolError("turn id is not valid")
	}
	if result.Limits.Steps < 1 || result.Limits.Steps > 24 || result.Limits.Seconds < 1 || result.Limits.Seconds > 900 || result.Limits.MaxRequestBytes < 1 || result.Limits.MaxRequestBytes > 2000000 {
		return domain.AssistantTurn{}, assistantProtocolError("turn limits out of range")
	}
	return result, nil
}

func (client AssistantClient) Cancel(ctx context.Context, token, turnID string) error {
	if !assistantTurnPattern.MatchString(turnID) {
		return assistantProtocolError("turn id is not valid")
	}
	response, err := client.request(ctx, "POST", "/turns/"+turnID+"/cancel", token, nil, 15*time.Second)
	if err != nil {
		return err
	}
	return assistantResponse(response, 204, nil)
}

func (client AssistantClient) Step(ctx context.Context, token, turnID string, request domain.AssistantStepRequest, emit func(domain.AssistantEvent)) error {
	if !assistantTurnPattern.MatchString(turnID) {
		return assistantProtocolError("turn id is not valid")
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
		return assistantProtocolError("content-type is not text/event-stream")
	}
	err = parseAssistantSSE(response.Body, emit)
	var serviceError *domain.AssistantError
	if errors.As(err, &serviceError) && serviceError.Code == "service_interrupted" && errors.Is(ctx.Err(), context.Canceled) {
		// The user (or the runner) cancelled the request: not a service interruption.
		return assistantProtocolError("request cancelled")
	}
	return err
}

type assistantCountingReader struct {
	source io.Reader
	bytes  int
}

func (reader *assistantCountingReader) Read(buffer []byte) (int, error) {
	n, err := reader.source.Read(buffer)
	reader.bytes += n
	if reader.bytes > 4*1024*1024 {
		return n, assistantProtocolError("stream exceeds 4 MiB")
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
		if terminal {
			return assistantProtocolError("event after terminal")
		}
		if name == "" {
			return assistantProtocolError("event without a name")
		}
		if data == "" {
			return assistantProtocolError("event " + name + " has no data")
		}
		if serviceSeen && name != "tool_calls" && name != "final" && name != "error" {
			return assistantProtocolError("event " + name + " after service_messages")
		}
		event, err := decodeAssistantEvent(name, []byte(strings.TrimSuffix(data, "\n")))
		if err != nil {
			return err
		}
		if name == "service_messages" {
			serviceSeen = true
		}
		textBytes += len(event.Delta) + len(event.Summary)
		if textBytes > domain.AssistantTextLimit {
			return assistantProtocolError("text exceeds limit")
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
			return assistantProtocolError("event exceeds 256 KiB")
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			return assistantProtocolError("line without a field")
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			if name != "" {
				return assistantProtocolError("duplicate event name")
			}
			name = value
		case "data":
			data += value + "\n"
		default:
			return assistantProtocolError("unknown stream field " + strconv.Quote(field))
		}
	}
	if err := scanner.Err(); err != nil {
		var serviceError *domain.AssistantError
		switch {
		case errors.As(err, &serviceError):
			return err
		case errors.Is(err, bufio.ErrTooLong):
			return assistantProtocolError("event exceeds 256 KiB")
		}
		return assistantInterruptedError("stream read failed: " + err.Error())
	}
	if name != "" || data != "" {
		if terminal {
			return assistantProtocolError("event after terminal")
		}
		return assistantInterruptedError("stream ended inside an event")
	}
	if !terminal {
		return assistantInterruptedError("stream ended before a terminal event")
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
		return event, assistantProtocolError("unknown event name " + strconv.Quote(name))
	}
	if reason := assistantDecodeReason(data, target); reason != "" {
		return event, assistantProtocolError(name + " event: " + reason)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return event, assistantProtocolError(name + " event is not a JSON object")
	}
	required := map[string][]string{
		"text": {"delta"}, "activity": {"text"}, "tool_calls": {"calls"}, "final": {"summary"},
		"usage": {"inputTokens", "outputTokens"}, "error": {"code", "message"},
		"service_messages": {"messages"},
	}
	for _, field := range required[name] {
		value, exists := fields[field]
		if !exists || string(value) == "null" {
			return event, assistantProtocolError(name + " event is missing field " + field)
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
		return event, assistantProtocolError(name + " event payload cannot be decoded")
	}
	event.Delta, event.Text, event.Calls, event.Summary, event.Code, event.Message, event.InputTokens, event.OutputTokens = payload.Delta, payload.Text, payload.Calls, payload.Summary, payload.Code, payload.Message, payload.InputTokens, payload.OutputTokens
	event.Messages = payload.Messages
	if name == "usage" && (event.InputTokens < 0 || event.OutputTokens < 0) {
		return event, assistantProtocolError("usage event has negative token counts")
	}
	if name == "error" && event.Code == "" {
		return event, assistantProtocolError("error event has an empty code")
	}
	if name == "tool_calls" {
		if len(event.Calls) == 0 {
			return event, assistantProtocolError("tool_calls event has no calls")
		}
		ids := map[string]bool{}
		for _, call := range event.Calls {
			if call.ID == "" {
				return event, assistantProtocolError("tool call without an id")
			}
			if ids[call.ID] {
				return event, assistantProtocolError("duplicate tool call id")
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
				return event, assistantProtocolError("unknown tool " + strconv.Quote(call.Name))
			}
			if len(call.Arguments) == 0 {
				return event, assistantProtocolError("tool call " + call.Name + " has no arguments")
			}
		}
	}
	return event, nil
}

func (client AssistantClient) ShareConversation(ctx context.Context, token, id string, payload domain.AssistantStepRequest) error {
	if !assistantTurnPattern.MatchString(id) {
		return assistantProtocolError("conversation id is not valid")
	}
	response, err := client.request(ctx, "POST", "/conversations/"+id, token, payload, 15*time.Second)
	if err != nil {
		return err
	}
	return assistantResponse(response, 204, nil)
}
func (client AssistantClient) ReportConversation(ctx context.Context, token, id, rating, comment string, payload domain.AssistantStepRequest) error {
	if !assistantTurnPattern.MatchString(id) || (rating != "helpful" && rating != "wrong") || len([]rune(comment)) > 500 {
		return assistantProtocolError("conversation id, rating or comment is not valid")
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

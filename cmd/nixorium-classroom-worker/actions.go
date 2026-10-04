package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// Page actions on selected computers reuse the classroom worker's own typed
// plan/apply handling, exactly as the teacher dashboard does. The review
// stays in this process: the page receives an opaque one-time identifier
// and the text to show, never the review token.

const (
	actionInternetBlock = "internet-block"
	actionInternetAllow = "internet-allow"
	actionRestart       = "restart"
	actionShutdown      = "shutdown"
	maxActionComputers  = 200
	// powerLabelTime keeps "Restarting…" on a card while the computer is away.
	powerLabelTime = 3 * time.Minute
)

type classroomHandler func(context.Context, domain.ClassroomRequest) domain.ClassroomResponse

type pendingAction struct {
	internet *domain.InternetPlan
	power    *domain.ShutdownPlanReport
	word     string
	expires  time.Time
}

type actionPlanRequest struct {
	Action    string   `json:"action"`
	Computers []string `json:"computers"`
}

type actionApplyRequest struct {
	ID   string `json:"id"`
	Word string `json:"word"`
}

type actionRow struct {
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
	Skip bool   `json:"skip,omitempty"`
}

// actionReview is what the confirmation dialog shows.
type actionReview struct {
	ID       string      `json:"id,omitempty"`
	Ready    bool        `json:"ready"`
	Title    string      `json:"title"`
	Message  string      `json:"message,omitempty"`
	Rows     []actionRow `json:"rows"`
	Warnings []string    `json:"warnings,omitempty"`
	Word     string      `json:"word,omitempty"`
	Confirm  string      `json:"confirm,omitempty"`
}

type actionResult struct {
	State   string      `json:"state"`
	Message string      `json:"message,omitempty"`
	Rows    []actionRow `json:"rows"`
}

// computerLabels are short card labels from the latest actions.
type computerLabels struct {
	internet    string
	power       string
	powerExpiry time.Time
}

func decodePageJSON(writer http.ResponseWriter, request *http.Request, value any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		http.Error(writer, "Invalid request.", http.StatusBadRequest)
		return false
	}
	return true
}

func writePageJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(value)
}

func validActionComputers(names []string) bool {
	if len(names) == 0 || len(names) > maxActionComputers {
		return false
	}
	seen := map[string]bool{}
	for _, name := range names {
		if !computerNamePattern.MatchString(name) || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

func plural(count int, one string) string {
	if count == 1 {
		return "1 " + one
	}
	return strconv.Itoa(count) + " " + one + "s"
}

func (server *viewServer) planAction(writer http.ResponseWriter, request *http.Request) {
	var body actionPlanRequest
	if !decodePageJSON(writer, request, &body) {
		return
	}
	if server.actions == nil || !validActionComputers(body.Computers) {
		http.Error(writer, "Invalid request.", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Minute)
	defer cancel()
	operation, err := domain.NewClassroomRequest(domain.ClassroomInternetPlanOperation)
	if err != nil {
		http.Error(writer, "The action could not start.", http.StatusInternalServerError)
		return
	}
	operation.Requested = strings.Join(body.Computers, ",")
	var review actionReview
	var pending pendingAction
	switch body.Action {
	case actionInternetBlock, actionInternetAllow:
		operation.InternetAction = domain.InternetUnblock
		if body.Action == actionInternetBlock {
			operation.InternetAction = domain.InternetBlock
		}
		response := server.actions(ctx, operation)
		if response.InternetPlan == nil {
			review = actionReview{Title: "Internet", Message: response.Message}
			break
		}
		review, pending = internetReview(*response.InternetPlan)
	case actionRestart, actionShutdown:
		operation.Operation = domain.ClassroomPowerPlanOperation
		operation.PowerAction = domain.ClientPowerOff
		if body.Action == actionRestart {
			operation.PowerAction = domain.ClientRestart
		}
		// Computers that cannot report their session stay eligible; the
		// review shows them and the word confirms the interruption.
		operation.SessionPolicy = domain.ShutdownAcknowledgeUnknown
		response := server.actions(ctx, operation)
		if response.PowerPlan == nil {
			review = actionReview{Title: "Power", Message: response.Message}
			break
		}
		review, pending = powerReview(*response.PowerPlan)
	default:
		http.Error(writer, "Unknown action.", http.StatusBadRequest)
		return
	}
	if review.Ready {
		id, err := randomToken()
		if err != nil {
			http.Error(writer, "The action could not start.", http.StatusInternalServerError)
			return
		}
		server.mutex.Lock()
		server.pruneActionsLocked()
		server.pending[id] = pending
		server.mutex.Unlock()
		review.ID = id
	}
	writePageJSON(writer, review)
}

func internetReview(plan domain.InternetPlan) (actionReview, pendingAction) {
	block := plan.Action == domain.InternetBlock
	review := actionReview{Title: "Allow Internet", Message: plan.Message, Rows: []actionRow{}}
	if block {
		review.Title = "Block Internet"
	}
	eligible := 0
	for _, target := range plan.Targets {
		row := actionRow{Name: target.Name}
		switch {
		case !target.Eligible:
			row.Skip, row.Note = true, "Switched off or not reachable; it will not change."
		case target.Observed.State == plan.Action.DesiredState():
			row.Note = "Already like this."
			eligible++
		default:
			eligible++
		}
		review.Rows = append(review.Rows, row)
	}
	for _, issue := range plan.Issues {
		review.Warnings = append(review.Warnings, issue.Message)
	}
	review.Ready = !plan.HasErrors() && eligible > 0
	if review.Ready {
		verb := "Allow Internet on "
		if block {
			verb = "Block Internet on "
		}
		review.Confirm = verb + plural(eligible, "computer")
	}
	return review, pendingAction{internet: &plan, expires: plan.ExpiresAt}
}

func powerReview(plan domain.ShutdownPlanReport) (actionReview, pendingAction) {
	restart := plan.Action == domain.ClientRestart
	review := actionReview{Title: "Shut down", Message: plan.Message, Rows: []actionRow{}}
	if restart {
		review.Title = "Restart"
	}
	busy := 0
	for _, target := range plan.Targets {
		row := actionRow{Name: target.Name}
		switch {
		case !target.Eligible:
			row.Skip, row.Note = true, "Switched off or not reachable."
		case target.Session == domain.ShutdownSessionActive:
			row.Note = "Someone is using it: unsaved work is lost."
			busy++
		case target.Session == domain.ShutdownSessionUnknown:
			row.Note = "It cannot tell whether someone is using it."
			busy++
		}
		review.Rows = append(review.Rows, row)
	}
	for _, issue := range plan.Issues {
		review.Warnings = append(review.Warnings, issue.Message)
	}
	review.Ready = !plan.HasErrors() && plan.Eligible > 0 && plan.ReviewToken != ""
	pending := pendingAction{power: &plan, expires: plan.ExpiresAt}
	if review.Ready {
		verb := "Shut down "
		if restart {
			verb = "Restart "
		}
		review.Confirm = verb + plural(plan.Eligible, "computer")
		// Interrupting someone's work needs the same word as the TUI.
		if busy > 0 {
			review.Word = plan.Action.Confirmation()
			pending.word = review.Word
		}
	}
	return review, pending
}

func (server *viewServer) pruneActionsLocked() {
	now := server.now()
	for id, pending := range server.pending {
		if now.After(pending.expires) {
			delete(server.pending, id)
		}
	}
}

func (server *viewServer) applyAction(writer http.ResponseWriter, request *http.Request) {
	var body actionApplyRequest
	if !decodePageJSON(writer, request, &body) {
		return
	}
	server.mutex.Lock()
	pending, found := server.pending[body.ID]
	// One use only: a second click, or a replayed request, needs a new review.
	delete(server.pending, body.ID)
	server.mutex.Unlock()
	if !found || server.now().After(pending.expires) {
		writePageJSON(writer, actionResult{State: "expired", Message: "This review has expired. Choose the action again.", Rows: []actionRow{}})
		return
	}
	if pending.word != "" && strings.TrimSpace(body.Word) != pending.word {
		writePageJSON(writer, actionResult{State: "refused", Message: "Type " + pending.word + " to confirm. Choose the action again.", Rows: []actionRow{}})
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Minute)
	defer cancel()
	operation, err := domain.NewClassroomRequest(domain.ClassroomInternetApplyOperation)
	if err != nil {
		http.Error(writer, "The action could not start.", http.StatusInternalServerError)
		return
	}
	result := actionResult{Rows: []actionRow{}}
	switch {
	case pending.internet != nil:
		operation.InternetPlan = pending.internet
		response := server.actions(ctx, operation)
		if response.InternetReport == nil {
			result.State, result.Message = "failed", response.Message
			break
		}
		report := response.InternetReport
		result.State, result.Message = report.State, report.Message
		for _, target := range report.Targets {
			result.Rows = append(result.Rows, actionRow{Name: target.Name, Note: outcomeText(target.State, target.Detail), Skip: target.State != "verified"})
			if target.State == "verified" {
				server.setLabel(target.Name, func(labels *computerLabels) {
					labels.internet = pending.internet.Action.DesiredState()
				})
			}
		}
	case pending.power != nil:
		operation.Operation = domain.ClassroomPowerApplyOperation
		operation.PowerPlan = pending.power
		response := server.actions(ctx, operation)
		if response.PowerReport == nil {
			result.State, result.Message = "failed", response.Message
			break
		}
		report := response.PowerReport
		result.State, result.Message = report.State, report.Message
		label := "Shutting down…"
		if pending.power.Action == domain.ClientRestart {
			label = "Restarting…"
		}
		for _, target := range report.Targets {
			result.Rows = append(result.Rows, actionRow{Name: target.Name, Note: outcomeText(target.State, target.Detail), Skip: target.State != "accepted"})
			if target.State == "accepted" {
				server.setLabel(target.Name, func(labels *computerLabels) {
					labels.power, labels.powerExpiry = label, server.now().Add(powerLabelTime)
					// After a restart the Internet block is on again.
					labels.internet = ""
				})
			}
		}
	default:
		result.State, result.Message = "failed", "Unknown action."
	}
	writePageJSON(writer, result)
}

func outcomeText(state, detail string) string {
	if detail != "" {
		return detail
	}
	switch state {
	case "verified", "accepted":
		return "Done."
	case "not-sent":
		return "Not sent."
	case "unconfirmed":
		return "Sent, but not confirmed."
	}
	return state
}

func (server *viewServer) setLabel(name string, change func(*computerLabels)) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	labels := server.labels[name]
	change(&labels)
	server.labels[name] = labels
}

// labelsFor returns the card labels of a computer; expired ones disappear.
func (server *viewServer) labelsFor(name string) (string, string) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	labels := server.labels[name]
	power := labels.power
	if server.now().After(labels.powerExpiry) {
		power = ""
	}
	return labels.internet, power
}

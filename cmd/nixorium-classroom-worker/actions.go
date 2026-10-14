package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
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
	actionLock          = "lock"
	actionUnlock        = "unlock"
	actionSendFiles     = "send-files"
	actionShowScreen    = "show-screen"
	maxActionComputers  = 200
	// powerLabelTime is the longest "Restarting…" or "Shutting down…" stays
	// on a card; the label usually ends sooner, when the state shows the
	// computer off or back (see labelsFor).
	powerLabelTime = 3 * time.Minute
)

// Outcomes of a computer in an action's result, which the page counts in
// its one-line notice.
const (
	outcomeDone    = "done"
	outcomeOff     = "off"
	outcomeSkipped = "skipped"
	outcomeFailed  = "failed"
)

type classroomHandler func(context.Context, domain.ClassroomRequest) domain.ClassroomResponse

type pendingAction struct {
	broadcast *domain.BroadcastPlan
	share     *domain.SharePlan
	lock      *domain.LockPlan
	internet  *domain.InternetPlan
	power     *domain.ShutdownPlanReport
	expires   time.Time
}

type actionPlanRequest struct {
	Action    string   `json:"action"`
	Computers []string `json:"computers"`
	// Transfer names files prepared with /api/share for send-files.
	Transfer string `json:"transfer,omitempty"`
}

type actionApplyRequest struct {
	ID string `json:"id"`
}

type actionRow struct {
	Name string `json:"name"`
	Note string `json:"note,omitempty"`
	Skip bool   `json:"skip,omitempty"`
	// Outcome is set in results only: done, off, skipped or failed.
	Outcome string `json:"outcome,omitempty"`
}

// actionReview is what the confirmation dialog shows.
type actionReview struct {
	ID       string      `json:"id,omitempty"`
	Ready    bool        `json:"ready"`
	Title    string      `json:"title"`
	Message  string      `json:"message,omitempty"`
	Rows     []actionRow `json:"rows"`
	Warnings []string    `json:"warnings,omitempty"`
	Confirm  string      `json:"confirm,omitempty"`
}

type actionResult struct {
	State   string      `json:"state"`
	Message string      `json:"message,omitempty"`
	Rows    []actionRow `json:"rows"`
	// Broadcast identifies a started showing of the teacher's screen.
	Broadcast string `json:"broadcast,omitempty"`
}

// computerLabels are short card labels from the latest actions.
type computerLabels struct {
	internet    string
	power       string
	powerExpiry time.Time
	// powerRestart: the label waits for the computer to come back;
	// powerAway: the computer has stopped answering since the restart.
	powerRestart bool
	powerAway    bool
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
		// review names them and those someone is using before the click.
		operation.SessionPolicy = domain.ShutdownAcknowledgeUnknown
		response := server.actions(ctx, operation)
		if response.PowerPlan == nil {
			review = actionReview{Title: "Power", Message: response.Message}
			break
		}
		review, pending = powerReview(*response.PowerPlan)
	case actionLock, actionUnlock:
		operation.Operation = domain.ClassroomLockPlanOperation
		operation.LockAction = domain.LockOff
		if body.Action == actionLock {
			operation.LockAction = domain.LockOn
		}
		response := server.actions(ctx, operation)
		if response.LockPlan == nil {
			review = actionReview{Title: "Lock", Message: response.Message}
			break
		}
		review, pending = lockReview(*response.LockPlan)
	case actionShowScreen:
		if server.broadcasts == nil {
			http.Error(writer, "Not available.", http.StatusBadRequest)
			return
		}
		review, pending = broadcastReview(server.broadcasts.Plan(ctx, operation.Requested))
	case actionSendFiles:
		operation.Operation = domain.ClassroomSharePlanOperation
		operation.ShareTransfer = body.Transfer
		response := server.actions(ctx, operation)
		if response.SharePlan == nil {
			review = actionReview{Title: "Send files", Message: response.Message}
			break
		}
		review, pending = shareReview(*response.SharePlan)
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

func broadcastReview(plan domain.BroadcastPlan) (actionReview, pendingAction) {
	review := actionReview{Title: "Share screen", Message: plan.Message, Rows: []actionRow{}}
	eligible := 0
	for _, target := range plan.Targets {
		row := actionRow{Name: target.Name, Note: target.Detail}
		if target.Eligible {
			eligible++
		} else {
			row.Note += " It gets your screen when someone signs in."
		}
		review.Rows = append(review.Rows, row)
	}
	for _, issue := range plan.Issues {
		review.Warnings = append(review.Warnings, issue.Message)
	}
	review.Ready = !plan.HasErrors() && eligible > 0
	if review.Ready {
		review.Confirm = "Share your screen on " + plural(eligible, "computer")
	}
	return review, pendingAction{broadcast: &plan, expires: plan.ExpiresAt}
}

func shareReview(plan domain.SharePlan) (actionReview, pendingAction) {
	review := actionReview{Title: "Send files", Message: plan.Message, Rows: []actionRow{}}
	eligible := 0
	for _, target := range plan.Targets {
		review.Rows = append(review.Rows, actionRow{Name: target.Name, Note: target.Detail, Skip: !target.Eligible})
		if target.Eligible {
			eligible++
		}
	}
	for _, issue := range plan.Issues {
		review.Warnings = append(review.Warnings, issue.Message)
	}
	review.Ready = !plan.HasErrors() && eligible > 0
	if review.Ready {
		review.Confirm = "Send to " + plural(eligible, "computer")
	}
	return review, pendingAction{share: &plan, expires: plan.ExpiresAt}
}

func lockReview(plan domain.LockPlan) (actionReview, pendingAction) {
	lock := plan.Action.Locked()
	review := actionReview{Title: "Unlock", Message: plan.Message, Rows: []actionRow{}}
	if lock {
		review.Title = "Lock"
	}
	eligible := 0
	for _, target := range plan.Targets {
		row := actionRow{Name: target.Name, Note: target.Detail}
		switch {
		case !target.Eligible:
			row.Skip = true
		case target.Locked == lock:
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
		verb := "Unlock "
		if lock {
			verb = "Lock "
		}
		review.Confirm = verb + plural(eligible, "computer")
	}
	return review, pendingAction{lock: &plan, expires: plan.ExpiresAt}
}

func powerReview(plan domain.ShutdownPlanReport) (actionReview, pendingAction) {
	restart := plan.Action == domain.ClientRestart
	review := actionReview{Title: "Shut down", Message: plan.Message, Rows: []actionRow{}}
	if restart {
		review.Title = "Restart"
	}
	for _, target := range plan.Targets {
		row := actionRow{Name: target.Name}
		switch {
		case !target.Eligible:
			row.Skip, row.Note = true, app.ClassroomOffDetail
		case target.Session == domain.ShutdownSessionActive:
			row.Note = "Someone is using it: unsaved work is lost."
		case target.Session == domain.ShutdownSessionUnknown:
			row.Note = "It cannot tell whether someone is using it."
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
	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Minute)
	defer cancel()
	operation, err := domain.NewClassroomRequest(domain.ClassroomInternetApplyOperation)
	if err != nil {
		http.Error(writer, "The action could not start.", http.StatusInternalServerError)
		return
	}
	result := actionResult{Rows: []actionRow{}}
	switch {
	case pending.broadcast != nil:
		id, err := server.broadcasts.Start(ctx, *pending.broadcast)
		if err != nil {
			result.State, result.Message = "failed", err.Error()
			break
		}
		result.State, result.Message, result.Broadcast = "started", "Your screen is being shown.", id
		for _, target := range pending.broadcast.Targets {
			result.Rows = append(result.Rows, actionRow{Name: target.Name, Outcome: outcomeDone})
		}
	case pending.share != nil:
		operation.Operation = domain.ClassroomShareApplyOperation
		operation.SharePlan = pending.share
		response := server.actions(ctx, operation)
		if response.ShareReport == nil {
			result.State, result.Message = "failed", response.Message
			break
		}
		report := response.ShareReport
		result.State, result.Message = report.State, report.Message
		planned := map[string]plannedTarget{}
		for _, target := range pending.share.Targets {
			planned[target.Name] = plannedTarget{eligible: target.Eligible, detail: target.Detail}
		}
		for _, target := range report.Targets {
			result.Rows = append(result.Rows, resultRow(target.Name, target.State == "delivered", target.Detail, planned[target.Name]))
		}
	case pending.lock != nil:
		operation.Operation = domain.ClassroomLockApplyOperation
		operation.LockPlan = pending.lock
		response := server.actions(ctx, operation)
		if response.LockReport == nil {
			result.State, result.Message = "failed", response.Message
			break
		}
		report := response.LockReport
		result.State, result.Message = report.State, report.Message
		planned := map[string]plannedTarget{}
		for _, target := range pending.lock.Targets {
			planned[target.Name] = plannedTarget{eligible: target.Eligible, detail: target.Detail}
		}
		for _, target := range report.Targets {
			result.Rows = append(result.Rows, resultRow(target.Name, target.State == "verified", outcomeText(target.State, target.Detail), planned[target.Name]))
		}
	case pending.internet != nil:
		operation.InternetPlan = pending.internet
		response := server.actions(ctx, operation)
		if response.InternetReport == nil {
			result.State, result.Message = "failed", response.Message
			break
		}
		report := response.InternetReport
		result.State, result.Message = report.State, report.Message
		// The review called every computer it left out switched off or
		// not reachable.
		planned := map[string]plannedTarget{}
		for _, target := range pending.internet.Targets {
			planned[target.Name] = plannedTarget{eligible: target.Eligible, detail: app.ClassroomOffDetail}
		}
		for _, target := range report.Targets {
			result.Rows = append(result.Rows, resultRow(target.Name, target.State == "verified", outcomeText(target.State, target.Detail), planned[target.Name]))
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
		planned := map[string]plannedTarget{}
		for _, target := range pending.power.Targets {
			planned[target.Name] = plannedTarget{eligible: target.Eligible, detail: app.ClassroomOffDetail}
		}
		for _, target := range report.Targets {
			result.Rows = append(result.Rows, resultRow(target.Name, target.State == "accepted", outcomeText(target.State, target.Detail), planned[target.Name]))
			if target.State == "accepted" {
				server.setLabel(target.Name, func(labels *computerLabels) {
					labels.power, labels.powerExpiry = label, server.now().Add(powerLabelTime)
					labels.powerRestart, labels.powerAway = pending.power.Action == domain.ClientRestart, false
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

// plannedTarget is what the review found about one computer.
type plannedTarget struct {
	eligible bool
	detail   string
}

// resultRow is one computer of an action's result. A computer the review
// left out keeps the review's reason, which the teacher saw before
// confirming, and counts as off or skipped rather than failed.
func resultRow(name string, done bool, note string, planned plannedTarget) actionRow {
	row := actionRow{Name: name, Note: note, Skip: !done, Outcome: outcomeDone}
	switch {
	case done:
	case !planned.eligible:
		if planned.detail != "" {
			row.Note = planned.detail
		}
		row.Outcome = outcomeSkipped
		if planned.detail == app.ClassroomOffDetail {
			row.Outcome = outcomeOff
		}
	case note == app.ClassroomOffDetail:
		row.Outcome = outcomeOff
	default:
		row.Outcome = outcomeFailed
	}
	return row
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

// labelsFor returns the card labels of a computer in its current state.
// A power label ends as soon as the state shows the action done: a computer
// being shut down that stops answering is off, and a restarting one that
// stopped answering and answers again is back. Otherwise it expires after
// powerLabelTime.
func (server *viewServer) labelsFor(name, state string) (string, string) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	labels, found := server.labels[name]
	if !found || labels.power == "" {
		return labels.internet, ""
	}
	switch {
	case server.now().After(labels.powerExpiry):
		labels.power = ""
	case state == app.ClassroomUnreachable && !labels.powerRestart:
		labels.power = ""
	case state == app.ClassroomUnreachable:
		labels.powerAway = true
	case labels.powerAway && state != app.ClassroomConnecting:
		labels.power = ""
	}
	if labels.power == "" {
		labels.powerExpiry, labels.powerRestart, labels.powerAway = time.Time{}, false, false
	}
	server.labels[name] = labels
	return labels.internet, labels.power
}

// shareChosen has the page's own user choose a file or folder through that
// user's desktop helper, and returns the prepared transfer to review.
func (server *viewServer) shareChosen(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		Kind string `json:"kind"`
	}
	if !decodePageJSON(writer, request, &body) {
		return
	}
	if body.Kind != domain.DesktopChooseFile && body.Kind != domain.DesktopChooseFolder {
		http.Error(writer, "Choose a file or a folder.", http.StatusBadRequest)
		return
	}
	grant, ok := server.session(request)
	if !ok || server.desktops == nil {
		http.Error(writer, "Not available.", http.StatusForbidden)
		return
	}
	transfer, cancelled, err := server.desktops.Request(request.Context(), grant.uid, body.Kind)
	switch {
	case err != nil:
		writePageJSON(writer, map[string]any{"message": err.Error()})
	case cancelled:
		writePageJSON(writer, map[string]any{"cancelled": true})
	default:
		writePageJSON(writer, map[string]any{"transfer": transfer})
	}
}

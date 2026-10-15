package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

// wakeSource is a laboratory of three computers: pc01 on, pc02 off with a
// learned address, pc03 off and never seen.
type wakeSource struct {
	sent []string
}

func (source *wakeSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	meta := domain.LabMeta{}
	meta.Controller.Name = "controller"
	meta.Network.Base, meta.Network.PrefixLength = "10.42.0.0", 24
	meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.42.0.11"}, {Name: "pc02", IP: "10.42.0.12"}, {Name: "pc03", IP: "10.42.0.13"}}
	return meta, nil
}

func (source *wakeSource) SSHStatus(_ context.Context, hosts []domain.HostMeta, _ time.Duration) map[string]domain.SSHProbe {
	probes := map[string]domain.SSHProbe{}
	for _, host := range hosts {
		probes[host.Name] = domain.SSHProbe{Reachability: domain.ReachabilityUnreachable}
	}
	probes["pc01"] = domain.SSHProbe{Reachability: domain.ReachabilityReachable, SSH: domain.SSHAvailable}
	return probes
}

func (source *wakeSource) LearnWakeAddresses([]domain.HostMeta) {}

func (source *wakeSource) WakeAddresses() map[string]domain.WakeAddress {
	return map[string]domain.WakeAddress{"pc02": {MAC: "52:54:00:00:00:02"}}
}

func (source *wakeSource) SendWakePackets(_ context.Context, _ domain.WakeNetwork, macs []string) map[string]error {
	source.sent = append(source.sent, macs...)
	results := map[string]error{}
	for _, mac := range macs {
		results[mac] = nil
	}
	return results
}

func wakeWorker() (classroomWorker, *wakeSource) {
	source := &wakeSource{}
	return classroomWorker{repository: "/home/admin/nixorium-deployment", wake: app.NewWakeManager(source)}, source
}

// The teacher reaches Switch on only through the classroom socket, whose
// requests the worker validates like every classroom operation.
func TestTeacherSwitchesOnThroughTheClassroomWorker(t *testing.T) {
	worker, source := wakeWorker()
	for _, operation := range []domain.ClassroomOperation{domain.ClassroomWakePlanOperation, domain.ClassroomWakeApplyOperation} {
		request, err := domain.NewClassroomRequest(operation)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(request)
		if _, err := domain.DecodeClassroomRequest(encoded); err != nil {
			t.Fatalf("%s is not a classroom operation: %v", operation, err)
		}
	}
	plan := worker.handle(context.Background(), domain.ClassroomRequest{Operation: domain.ClassroomWakePlanOperation, Requested: "@lab"})
	if plan.WakePlan == nil || plan.WakePlan.HasErrors() || plan.WakePlan.Count(domain.WakeTargetSend) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	foreign := *plan.WakePlan
	foreign.Repository = "/tmp/other"
	if refused := worker.handle(context.Background(), domain.ClassroomRequest{Operation: domain.ClassroomWakeApplyOperation, WakePlan: &foreign}); refused.State != "failed" || refused.WakeReport != nil {
		t.Fatalf("foreign review = %+v", refused)
	}
	if refused := worker.handle(context.Background(), domain.ClassroomRequest{Operation: domain.ClassroomWakeApplyOperation}); refused.State != "failed" {
		t.Fatalf("missing review = %+v", refused)
	}
	// Names outside the inventory are refused before anything is checked.
	invalid := worker.handle(context.Background(), domain.ClassroomRequest{Operation: domain.ClassroomWakePlanOperation, Requested: "pc02,../pc01"})
	if invalid.WakePlan == nil || !invalid.WakePlan.HasErrors() || invalid.WakePlan.Targets[0].State != domain.WakeTargetInvalid {
		t.Fatalf("invalid = %+v", invalid)
	}
	applied := worker.handle(context.Background(), domain.ClassroomRequest{Operation: domain.ClassroomWakeApplyOperation, WakePlan: plan.WakePlan})
	if applied.WakeReport == nil || applied.WakeReport.Sent != 1 || len(source.sent) != 1 || source.sent[0] != "52:54:00:00:00:02" {
		t.Fatalf("applied = %+v, sent %v", applied, source.sent)
	}
}

func TestPageSwitchesOnAfterAReview(t *testing.T) {
	worker, source := wakeWorker()
	server, host, cookie := openedViewServer(t, &fakeActions{})
	server.actions = worker.handle
	state := app.ClassroomUnreachable
	server.source = stateViewSource{state: &state}
	origin := "http://" + host

	response := actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"wake","computers":["pc01","pc02","pc03"]}`)
	var review actionReview
	if err := json.Unmarshal(response.Body.Bytes(), &review); err != nil || !review.Ready || review.Title != "Switch on" || review.Confirm != "Switch on 1 computer" {
		t.Fatalf("review = %s", response.Body.String())
	}
	// The review names what is skipped and why.
	if len(review.Rows) != 3 || !review.Rows[0].Skip || review.Rows[0].Note != "Already on." || review.Rows[1].Skip ||
		!review.Rows[2].Skip || !strings.Contains(strings.ToLower(review.Rows[2].Note), "address not known yet: it is learned while the computer is on and the classroom view is open") {
		t.Fatalf("rows = %+v", review.Rows)
	}
	apply := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`"}`)
	var result actionResult
	if err := json.Unmarshal(apply.Body.Bytes(), &result); err != nil || len(result.Rows) != 3 {
		t.Fatalf("result = %s", apply.Body.String())
	}
	outcomes := []string{result.Rows[0].Outcome, result.Rows[1].Outcome, result.Rows[2].Outcome}
	if outcomes[0] != outcomeOn || outcomes[1] != outcomeSent || outcomes[2] != outcomeUnknown || result.Rows[0].Skip || result.Rows[1].Skip || !result.Rows[2].Skip {
		t.Fatalf("rows = %+v", result.Rows)
	}
	if len(source.sent) != 1 {
		t.Fatalf("sent = %v", source.sent)
	}
	// The card waits for the computer to answer.
	if _, power := server.labelsFor("pc02", app.ClassroomUnreachable); power != wakeLabel {
		t.Fatalf("label = %q", power)
	}
	if _, power := server.labelsFor("pc02", app.ClassroomConnecting); power != wakeLabel {
		t.Fatalf("label while connecting = %q", power)
	}
	if _, power := server.labelsFor("pc02", app.ClassroomNoSession); power != "" {
		t.Fatalf("label after answering = %q", power)
	}
	if _, power := server.labelsFor("pc01", app.ClassroomViewing); power != "" {
		t.Fatalf("a computer already on got %q", power)
	}
}

func TestPageSwitchOnLabelEndsAfterThreeMinutes(t *testing.T) {
	worker, _ := wakeWorker()
	server, host, cookie := openedViewServer(t, &fakeActions{})
	server.actions = worker.handle
	origin := "http://" + host
	var review actionReview
	response := actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"wake","computers":["pc02"]}`)
	if err := json.Unmarshal(response.Body.Bytes(), &review); err != nil || !review.Ready {
		t.Fatalf("review = %s", response.Body.String())
	}
	actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`"}`)
	if _, power := server.labelsFor("pc02", app.ClassroomUnreachable); power != wakeLabel {
		t.Fatalf("label = %q", power)
	}
	server.now = func() time.Time { return time.Now().Add(powerLabelTime + time.Minute) }
	if _, power := server.labelsFor("pc02", app.ClassroomUnreachable); power != "" {
		t.Fatalf("expired label = %q", power)
	}
}

func TestPageSwitchOnWithNothingToSendCannotBeConfirmed(t *testing.T) {
	worker, _ := wakeWorker()
	server, host, cookie := openedViewServer(t, &fakeActions{})
	server.actions = worker.handle
	response := actionPost(server, host, cookie, "http://"+host, "/api/actions/plan", `{"action":"wake","computers":["pc01","pc03"]}`)
	var review actionReview
	if err := json.Unmarshal(response.Body.Bytes(), &review); err != nil || review.Ready || review.ID != "" || !strings.HasPrefix(review.Message, "No packet will be sent") {
		t.Fatalf("review = %s", response.Body.String())
	}
}

func TestPageOffersSwitchOnInThePowerMenuAndTheComputerMenu(t *testing.T) {
	page, err := viewAssets.ReadFile("view/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, menu := range []string{`class="actions-menu power-menu"`, `class="actions-menu viewer-actions"`} {
		_, rest, found := strings.Cut(string(page), menu)
		block, _, closed := strings.Cut(rest, "</details>")
		if !found || !closed || !strings.Contains(block, `data-action="wake"`) || !strings.Contains(block, ">Switch on</button>") {
			t.Fatalf("%s has no Switch on button", menu)
		}
	}
	script, err := viewAssets.ReadFile("view/app.js")
	if err != nil {
		t.Fatal(err)
	}
	// The notice counts packets: "Switch on: 3 sent · 2 already on · 1 address unknown".
	for _, words := range []string{"sent: 'sent'", "on: 'already on'", "unknown: 'address unknown'"} {
		if !strings.Contains(string(script), words) {
			t.Fatalf("app.js lacks %s", words)
		}
	}
}

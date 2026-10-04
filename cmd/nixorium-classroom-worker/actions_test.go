package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// fakeActions answers like the worker: one busy computer and one switched off.
type fakeActions struct {
	requests []domain.ClassroomRequest
}

func (fake *fakeActions) handle(_ context.Context, request domain.ClassroomRequest) domain.ClassroomResponse {
	fake.requests = append(fake.requests, request)
	expires := time.Now().Add(time.Minute)
	switch request.Operation {
	case domain.ClassroomInternetPlanOperation:
		return domain.ClassroomResponse{InternetPlan: &domain.InternetPlan{State: "ready", Action: request.InternetAction, ExpiresAt: expires, ReviewToken: "sha256:internet", Targets: []domain.InternetTarget{
			{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true},
			{HostMeta: domain.HostMeta{Name: "pc02"}},
		}}}
	case domain.ClassroomInternetApplyOperation:
		return domain.ClassroomResponse{InternetReport: &domain.InternetReport{State: "partial", Targets: []domain.InternetOutcome{
			{Name: "pc01", State: "verified"}, {Name: "pc02", State: "not-sent", Detail: "Unavailable in the reviewed plan; no request queued."},
		}}}
	case domain.ClassroomPowerPlanOperation:
		return domain.ClassroomResponse{PowerPlan: &domain.ShutdownPlanReport{State: "ready", Action: request.PowerAction, Eligible: 1, ExpiresAt: expires, ReviewToken: "sha256:power", Targets: []domain.ShutdownTargetPlan{
			{Name: "pc01", Eligible: true, Session: domain.ShutdownSessionActive},
		}}}
	case domain.ClassroomPowerApplyOperation:
		return domain.ClassroomResponse{PowerReport: &domain.ShutdownApplyReport{State: "completed", Targets: []domain.ShutdownTargetOutcome{{Name: "pc01", State: "accepted"}}}}
	}
	return domain.ClassroomResponse{State: "failed", Message: "unexpected"}
}

func actionPost(server *viewServer, host, cookie, origin, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Host = host
	request.Header.Set("Origin", origin)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: viewCookie, Value: cookie})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func openedViewServer(t *testing.T, fake *fakeActions) (*viewServer, string, string) {
	t.Helper()
	server := newViewServer(fakeViewSource{})
	server.actions = fake.handle
	address, err := server.Open()
	if err != nil {
		t.Fatal(err)
	}
	host := strings.TrimPrefix(strings.SplitN(address, "/open", 2)[0], "http://")
	response := viewRequest(server, "/open"+strings.SplitN(address, "/open", 2)[1], host, "")
	return server, host, response.Result().Cookies()[0].Value
}

func TestPageActionsReviewThenApplyOnce(t *testing.T) {
	fake := &fakeActions{}
	server, host, cookie := openedViewServer(t, fake)
	origin := "http://" + host

	plan := actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"internet-block","computers":["pc01","pc02"]}`)
	var review actionReview
	if err := json.Unmarshal(plan.Body.Bytes(), &review); err != nil || !review.Ready || review.ID == "" || review.Confirm != "Block Internet on 1 computer" {
		t.Fatalf("review = %d %s", plan.Code, plan.Body.String())
	}
	if strings.Contains(plan.Body.String(), "sha256:") {
		t.Fatal("the review token reached the page")
	}
	if request := fake.requests[0]; request.Requested != "pc01,pc02" || request.InternetAction != domain.InternetBlock {
		t.Fatalf("plan request = %+v", request)
	}
	apply := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`"}`)
	var result actionResult
	if err := json.Unmarshal(apply.Body.Bytes(), &result); err != nil || result.State != "partial" || len(result.Rows) != 2 || result.Rows[0].Note != "Done." || !result.Rows[1].Skip {
		t.Fatalf("result = %s", apply.Body.String())
	}
	if internet, _ := server.labelsFor("pc01"); internet != "blocked" {
		t.Fatalf("pc01 label = %q", internet)
	}
	if internet, _ := server.labelsFor("pc02"); internet != "" {
		t.Fatalf("pc02 label = %q", internet)
	}
	// The same review cannot be applied twice.
	again := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`"}`)
	if !strings.Contains(again.Body.String(), `"expired"`) || len(fake.requests) != 2 {
		t.Fatalf("second apply = %s, %d requests", again.Body.String(), len(fake.requests))
	}
	list := viewRequest(server, "/api/computers", host, cookie)
	if !strings.Contains(list.Body.String(), `"internet":"blocked"`) {
		t.Fatalf("computers = %s", list.Body.String())
	}
}

func TestPagePowerActionNeedsTheWordWhenSomeoneWorks(t *testing.T) {
	fake := &fakeActions{}
	server, host, cookie := openedViewServer(t, fake)
	origin := "http://" + host
	plan := func() actionReview {
		var review actionReview
		response := actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"restart","computers":["pc01"]}`)
		if err := json.Unmarshal(response.Body.Bytes(), &review); err != nil {
			t.Fatal(err)
		}
		return review
	}
	review := plan()
	if !review.Ready || review.Word != "RESTART" || review.Rows[0].Note == "" || fake.requests[0].SessionPolicy != domain.ShutdownAcknowledgeUnknown {
		t.Fatalf("review = %+v", review)
	}
	refused := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`","word":"restart"}`)
	if !strings.Contains(refused.Body.String(), `"refused"`) || len(fake.requests) != 1 {
		t.Fatalf("wrong word = %s", refused.Body.String())
	}
	review = plan()
	applied := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`","word":"RESTART"}`)
	if !strings.Contains(applied.Body.String(), `"completed"`) {
		t.Fatalf("apply = %s", applied.Body.String())
	}
	if _, power := server.labelsFor("pc01"); power != "Restarting…" {
		t.Fatalf("power label = %q", power)
	}
	server.now = func() time.Time { return time.Now().Add(powerLabelTime + time.Minute) }
	if _, power := server.labelsFor("pc01"); power != "" {
		t.Fatalf("expired power label = %q", power)
	}
}

func TestPageActionsRefuseForeignOriginsAndBadTargets(t *testing.T) {
	fake := &fakeActions{}
	server, host, cookie := openedViewServer(t, fake)
	origin := "http://" + host
	for name, response := range map[string]*httptest.ResponseRecorder{
		"foreign origin": actionPost(server, host, cookie, "http://evil.example", "/api/actions/plan", `{"action":"shutdown","computers":["pc01"]}`),
		"no computers":   actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"shutdown","computers":[]}`),
		"bad name":       actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"shutdown","computers":["../pc01"]}`),
		"duplicate":      actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"shutdown","computers":["pc01","pc01"]}`),
		"unknown action": actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"format","computers":["pc01"]}`),
		"unknown field":  actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"shutdown","computers":["pc01"],"repository":"/tmp"}`),
		"no session":     actionPost(server, host, "", origin, "/api/actions/plan", `{"action":"shutdown","computers":["pc01"]}`),
	} {
		if response.Code == http.StatusOK {
			t.Fatalf("%s was accepted: %s", name, response.Body.String())
		}
	}
	if len(fake.requests) != 0 {
		t.Fatalf("refused requests reached the worker: %+v", fake.requests)
	}
}

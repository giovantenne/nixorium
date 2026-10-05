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
	case domain.ClassroomLockPlanOperation:
		return domain.ClassroomResponse{LockPlan: &domain.LockPlan{State: "ready", Action: request.LockAction, ExpiresAt: expires, ReviewToken: "sha256:lock", Targets: []domain.LockTarget{
			{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true, Reachable: true},
			{HostMeta: domain.HostMeta{Name: "pc02"}, Detail: "Nobody is signed in."},
		}}}
	case domain.ClassroomLockApplyOperation:
		return domain.ClassroomResponse{LockReport: &domain.LockReport{State: "partial", Targets: []domain.LockOutcome{
			{Name: "pc01", State: "verified", Detail: "Locked."}, {Name: "pc02", State: "not-sent", Detail: "Unavailable in the reviewed plan; nothing was sent."},
		}}}
	case domain.ClassroomShareBeginOperation:
		return domain.ClassroomResponse{ShareTransfer: "t1"}
	case domain.ClassroomShareChunkOperation:
		if request.ShareTransfer != "t1" {
			return domain.ClassroomResponse{State: "failed", Message: "unknown transfer"}
		}
		return domain.ClassroomResponse{State: "completed"}
	case domain.ClassroomSharePlanOperation:
		return domain.ClassroomResponse{SharePlan: &domain.SharePlan{State: "ready", Transfer: request.ShareTransfer, ExpiresAt: expires, ReviewToken: "sha256:share", Message: "1 item (5 bytes) go to the desktop.", Targets: []domain.ShareTarget{
			{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true},
		}}}
	case domain.ClassroomShareApplyOperation:
		return domain.ClassroomResponse{ShareReport: &domain.ShareReport{State: "completed", Targets: []domain.ShareOutcome{{Name: "pc01", State: "delivered", Detail: "On the desktop: notes.txt."}}}}
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
	address, err := server.Open(1000)
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

func TestPagePowerActionWarnsAboutWorkInTheReview(t *testing.T) {
	fake := &fakeActions{}
	server, host, cookie := openedViewServer(t, fake)
	origin := "http://" + host
	var review actionReview
	response := actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"restart","computers":["pc01"]}`)
	if err := json.Unmarshal(response.Body.Bytes(), &review); err != nil {
		t.Fatal(err)
	}
	// The review names the computer someone is using; the explicit button
	// confirms, with no word to type.
	if !review.Ready || review.Confirm != "Restart 1 computer" || !strings.Contains(review.Rows[0].Note, "unsaved work") || fake.requests[0].SessionPolicy != domain.ShutdownAcknowledgeUnknown {
		t.Fatalf("review = %+v", review)
	}
	if typed := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`","word":"RESTART"}`); typed.Code == http.StatusOK {
		t.Fatal("a typed word is still accepted as a field")
	}
	applied := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`"}`)
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

func TestPageLocksSelectedComputers(t *testing.T) {
	fake := &fakeActions{}
	server, host, cookie := openedViewServer(t, fake)
	origin := "http://" + host
	plan := actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"lock","computers":["pc01","pc02"]}`)
	var review actionReview
	if err := json.Unmarshal(plan.Body.Bytes(), &review); err != nil || !review.Ready || review.Confirm != "Lock 1 computer" || !review.Rows[1].Skip {
		t.Fatalf("review = %s", plan.Body.String())
	}
	if request := fake.requests[0]; request.Operation != domain.ClassroomLockPlanOperation || request.LockAction != domain.LockOn {
		t.Fatalf("plan request = %+v", request)
	}
	apply := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`"}`)
	var result actionResult
	if err := json.Unmarshal(apply.Body.Bytes(), &result); err != nil || result.State != "partial" || result.Rows[0].Note != "Locked." || result.Rows[0].Skip || !result.Rows[1].Skip {
		t.Fatalf("result = %s", apply.Body.String())
	}
	if request := fake.requests[1]; request.Operation != domain.ClassroomLockApplyOperation || request.LockPlan == nil || request.LockPlan.ReviewToken != "sha256:lock" {
		t.Fatalf("apply request = %+v", request)
	}
}

func TestPageSendsFilesChosenByItsOwnUser(t *testing.T) {
	fake := &fakeActions{}
	server, host, cookie := openedViewServer(t, fake)
	server.desktops = newDesktopBroker()
	origin := "http://" + host
	// Only a file or a folder can be asked for.
	if response := actionPost(server, host, cookie, origin, "/api/share/choose", `{"kind":"home"}`); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown kind = %d", response.Code)
	}
	// Without a helper the page is told to open the view again.
	if response := actionPost(server, host, cookie, origin, "/api/share/choose", `{"kind":"file"}`); !strings.Contains(response.Body.String(), "open it again from Nixorium") {
		t.Fatalf("no helper = %s", response.Body.String())
	}
	// Another user's helper is never asked; the page's own user (1000) is,
	// with the kind the page asked for. A closed chooser sends nothing.
	answer := func(transfer string, cancelled bool) chan int {
		asked := make(chan int, 2)
		for _, uid := range []int{1001, 1000} {
			go func() {
				if job := server.desktops.Wait(context.Background(), uid); job.id != "" {
					if job.kind != domain.DesktopChooseFolder {
						t.Errorf("helper asked for %q", job.kind)
					}
					asked <- uid
					if server.desktops.Ready(1001, job.id, "stolen", "", false) {
						t.Error("another user answered the job")
					}
					server.desktops.Ready(uid, job.id, transfer, "", cancelled)
				}
			}()
		}
		time.Sleep(50 * time.Millisecond)
		return asked
	}
	asked := answer("", true)
	if response := actionPost(server, host, cookie, origin, "/api/share/choose", `{"kind":"folder"}`); !strings.Contains(response.Body.String(), `"cancelled":true`) || <-asked != 1000 {
		t.Fatalf("cancelled = %s", response.Body.String())
	}
	asked = answer("t1", false)
	response := actionPost(server, host, cookie, origin, "/api/share/choose", `{"kind":"folder"}`)
	if !strings.Contains(response.Body.String(), `"transfer":"t1"`) || <-asked != 1000 {
		t.Fatalf("chosen = %s", response.Body.String())
	}
	plan := actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"send-files","computers":["pc01"],"transfer":"t1"}`)
	var review actionReview
	if err := json.Unmarshal(plan.Body.Bytes(), &review); err != nil || !review.Ready || review.Confirm != "Send to 1 computer" {
		t.Fatalf("review = %s", plan.Body.String())
	}
	apply := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`"}`)
	if !strings.Contains(apply.Body.String(), "On the desktop: notes.txt.") {
		t.Fatalf("apply = %s", apply.Body.String())
	}
	for _, removed := range []string{"/api/share/begin", "/api/share/chunk"} {
		if response := actionPost(server, host, cookie, origin, removed, `{}`); response.Code == http.StatusOK || response.Code == http.StatusNoContent {
			t.Fatalf("%s still answers", removed)
		}
	}
}

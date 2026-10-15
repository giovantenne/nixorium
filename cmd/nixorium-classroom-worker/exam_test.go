package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

// examActions answers like the worker for the three steps of exam mode and
// for a restart: pc01 takes every action, pc02 is switched off.
type examActions struct {
	// unconfirmed names the actions pc01 does not confirm.
	unconfirmed map[domain.ClassroomOperation]bool
}

func (fake *examActions) handle(_ context.Context, request domain.ClassroomRequest) domain.ClassroomResponse {
	expires := time.Now().Add(time.Minute)
	off := domain.HostMeta{Name: "pc02"}
	switch request.Operation {
	case domain.ClassroomInternetPlanOperation:
		return domain.ClassroomResponse{InternetPlan: &domain.InternetPlan{State: "ready", Action: request.InternetAction, ExpiresAt: expires, ReviewToken: "sha256:internet", Targets: []domain.InternetTarget{{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true}, {HostMeta: off}}}}
	case domain.ClassroomInternetApplyOperation:
		return domain.ClassroomResponse{InternetReport: &domain.InternetReport{State: "partial", Targets: []domain.InternetOutcome{fake.internet(), {Name: "pc02", State: "not-sent"}}}}
	case domain.ClassroomLockPlanOperation:
		return domain.ClassroomResponse{LockPlan: &domain.LockPlan{State: "ready", Action: request.LockAction, ExpiresAt: expires, ReviewToken: "sha256:lock", Targets: []domain.LockTarget{{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true}, {HostMeta: off, Detail: app.ClassroomOffDetail}}}}
	case domain.ClassroomLockApplyOperation:
		state := "verified"
		if fake.unconfirmed[request.Operation] {
			state = "unconfirmed"
		}
		return domain.ClassroomResponse{LockReport: &domain.LockReport{State: "partial", Targets: []domain.LockOutcome{{Name: "pc01", State: state}, {Name: "pc02", State: "not-sent"}}}}
	case domain.ClassroomMessagePlanOperation:
		content := domain.MessageContent{}
		if request.MessageContent != nil {
			content = *request.MessageContent
		}
		return domain.ClassroomResponse{MessagePlan: &domain.MessagePlan{State: "ready", Action: request.MessageAction, Content: content, ExpiresAt: expires, ReviewToken: "sha256:message", Targets: []domain.LockTarget{{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true}, {HostMeta: off, Detail: app.ClassroomOffDetail}}}}
	case domain.ClassroomMessageApplyOperation:
		return domain.ClassroomResponse{MessageReport: &domain.MessageReport{State: "partial", Targets: []domain.MessageOutcome{{Name: "pc01", State: domain.MessageDone}, {Name: "pc02", State: domain.MessageNotSent}}}}
	case domain.ClassroomPowerPlanOperation:
		return domain.ClassroomResponse{PowerPlan: &domain.ShutdownPlanReport{State: "ready", Action: request.PowerAction, Eligible: 1, ExpiresAt: expires, ReviewToken: "sha256:power", Targets: []domain.ShutdownTargetPlan{{Name: "pc01", Eligible: true}}}}
	case domain.ClassroomPowerApplyOperation:
		return domain.ClassroomResponse{PowerReport: &domain.ShutdownApplyReport{State: "completed", Targets: []domain.ShutdownTargetOutcome{{Name: "pc01", State: "accepted"}}}}
	}
	return domain.ClassroomResponse{State: "failed", Message: "unexpected"}
}

func (fake *examActions) internet() domain.InternetOutcome {
	if fake.unconfirmed[domain.ClassroomInternetApplyOperation] {
		return domain.InternetOutcome{Name: "pc01", State: "unconfirmed"}
	}
	return domain.InternetOutcome{Name: "pc01", State: "verified"}
}

// examViewSource lists pc01 in the state the test sets, and pc02 off.
type examViewSource struct {
	fakeViewSource
	state *string
}

func (source examViewSource) Computers(context.Context) ([]app.ClassroomComputer, error) {
	return []app.ClassroomComputer{{Name: "pc01", State: *source.state, HasImage: true}, {Name: "pc02", State: app.ClassroomUnreachable}}, nil
}

// examPage is one user's page of the same classroom view service.
type examPage struct {
	t            *testing.T
	server       *viewServer
	host, cookie string
}

func openPage(t *testing.T, server *viewServer, uid int) examPage {
	t.Helper()
	address, err := server.Open(uid)
	if err != nil {
		t.Fatal(err)
	}
	host := strings.TrimPrefix(strings.SplitN(address, "/open", 2)[0], "http://")
	cookie := viewRequest(server, "/open"+strings.SplitN(address, "/open", 2)[1], host, "").Result().Cookies()[0].Value
	return examPage{t, server, host, cookie}
}

// do plans an action, optionally tagged for exam mode, and applies it.
func (page examPage) do(action, exam string, extra string) actionResult {
	page.t.Helper()
	body := `{"action":"` + action + `","computers":["pc01","pc02"]` + extra
	if exam != "" {
		body += `,"exam":"` + exam + `"`
	}
	review := planReview(page.t, page.server, page.host, page.cookie, body+"}")
	if !review.Ready {
		page.t.Fatalf("%s %s review = %+v", exam, action, review)
	}
	return applyResult(page.t, page.server, page.host, page.cookie, review.ID)
}

// exams is what the page reads: each computer's exam mode, by name.
func (page examPage) exams() map[string]examView {
	page.t.Helper()
	response := viewRequest(page.server, "/api/computers", page.host, page.cookie)
	var list struct {
		Computers []struct {
			Name string    `json:"name"`
			Exam *examView `json:"exam"`
		} `json:"computers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		page.t.Fatalf("computers = %d %s", response.Code, response.Body.String())
	}
	found := map[string]examView{}
	for _, computer := range list.Computers {
		if computer.Exam != nil {
			found[computer.Name] = *computer.Exam
		}
	}
	return found
}

func examServer(t *testing.T, fake *examActions) (*viewServer, *string) {
	t.Helper()
	state := app.ClassroomViewing
	server := newViewServer(examViewSource{state: &state})
	server.actions = fake.handle
	return server, &state
}

const examMessageBody = `,"message":{"text":"Exam in progress. Wait for the teacher.","untilUnlock":true}`

func startExam(page examPage) {
	page.t.Helper()
	page.do(actionInternetBlock, examStart, "")
	page.do(actionLock, examStart, "")
	page.do(actionMessage, examStart, examMessageBody)
}

func TestWorkerRemembersWhichStepsExamModeStartedAndEndsThemOneByOne(t *testing.T) {
	server, _ := examServer(t, &examActions{})
	page := openPage(t, server, 1000)
	if exams := page.exams(); len(exams) != 0 {
		t.Fatalf("exam mode before anyone started it: %+v", exams)
	}
	before := time.Now().UnixMilli()
	startExam(page)
	exam, found := page.exams()["pc01"]
	if !found || strings.Join(exam.Steps, ",") != "internet,lock,message" || exam.Since < before-1000 || exam.Since > time.Now().UnixMilli() {
		t.Fatalf("exam mode of pc01 = %+v (%v)", exam, found)
	}
	// The computer that was switched off took no step.
	if _, found := page.exams()["pc02"]; found {
		t.Fatal("a computer that took no step is in exam mode")
	}
	// Ending removes the steps as they are done, in any order.
	for _, test := range []struct{ action, steps string }{
		{actionMessageHide, "internet,lock"},
		{actionUnlock, "internet"},
	} {
		page.do(test.action, examEnd, "")
		if got := page.exams()["pc01"]; strings.Join(got.Steps, ",") != test.steps {
			t.Fatalf("after %s the steps are %v, want %s", test.action, got.Steps, test.steps)
		}
	}
	page.do(actionInternetAllow, examEnd, "")
	if exams := page.exams(); len(exams) != 0 {
		t.Fatalf("exam mode after every step ended: %+v", exams)
	}
}

func TestWorkerRemembersOnlyWhatItAppliedForExamMode(t *testing.T) {
	// The same actions without the exam tag are the teacher's own.
	server, _ := examServer(t, &examActions{})
	page := openPage(t, server, 1000)
	page.do(actionInternetBlock, "", "")
	page.do(actionLock, "", "")
	page.do(actionMessage, "", examMessageBody)
	if exams := page.exams(); len(exams) != 0 {
		t.Fatalf("separate actions count as exam mode: %+v", exams)
	}
	// A step the computer did not confirm is not in effect.
	server, _ = examServer(t, &examActions{unconfirmed: map[domain.ClassroomOperation]bool{domain.ClassroomLockApplyOperation: true}})
	page = openPage(t, server, 1000)
	startExam(page)
	if got := page.exams()["pc01"]; strings.Join(got.Steps, ",") != "internet,message" {
		t.Fatalf("steps = %v", got.Steps)
	}
	// Ending a separate lock does not end an exam step of another kind: the
	// exam's own steps stay until exam mode ends them.
	page.do(actionUnlock, "", "")
	if got := page.exams()["pc01"]; strings.Join(got.Steps, ",") != "internet,message" {
		t.Fatalf("steps after a separate unlock = %v", got.Steps)
	}
}

func TestExamTagsMustBeAStepOfExamMode(t *testing.T) {
	server, _ := examServer(t, &examActions{})
	page := openPage(t, server, 1000)
	for _, test := range []struct{ action, exam string }{
		{actionRestart, examStart}, {actionShutdown, examEnd}, {actionCollect, examStart},
		{actionUnlock, examStart}, {actionInternetAllow, examStart}, {actionMessageHide, examStart},
		{actionLock, examEnd}, {actionInternetBlock, examEnd}, {actionMessage, examEnd},
		{actionLock, "pause"}, {actionLock, "Start"},
	} {
		body := `{"action":"` + test.action + `","computers":["pc01"],"exam":"` + test.exam + `"}`
		if response := actionPost(server, page.host, page.cookie, "http://"+page.host, "/api/actions/plan", body); response.Code != http.StatusBadRequest {
			t.Errorf("%s tagged %q = %d", test.action, test.exam, response.Code)
		}
	}
}

func TestWorkerEndsExamStepsItSeesEnd(t *testing.T) {
	server, state := examServer(t, &examActions{})
	page := openPage(t, server, 1000)
	steps := func() string { return strings.Join(page.exams()["pc01"].Steps, ",") }
	startExam(page)
	// A computer that stops answering may be off or just out of reach: the
	// memory stays.
	*state = app.ClassroomUnreachable
	if got := steps(); got != "internet,lock,message" {
		t.Fatalf("an unreachable computer = %q", got)
	}
	// The student's own lock screen keeps the session, and so the steps.
	*state = app.ClassroomLocked
	if got := steps(); got != "internet,lock,message" {
		t.Fatalf("a computer behind its own lock screen = %q", got)
	}
	// A computer whose monitoring stopped still has its session, and so its
	// steps; the teacher can end them when it answers again.
	*state = app.ClassroomNoAgent
	if got := steps(); got != "internet,lock,message" {
		t.Fatalf("monitoring stopped = %q", got)
	}
	// Nobody signed in ends the lock and the message; the Internet block
	// stays until the computer restarts.
	*state = app.ClassroomNoSession
	if got := steps(); got != "internet" {
		t.Fatalf("nobody signed in = %q", got)
	}
	*state = app.ClassroomViewing
	if got := steps(); got != "internet" {
		t.Fatalf("signed in again = %q", got)
	}
	// An Internet state that reads as allowed again ends that step too.
	server.setLabel("pc01", func(labels *computerLabels) { labels.internet = "enabled" })
	if exams := page.exams(); len(exams) != 0 {
		t.Fatalf("Internet allowed again = %+v", exams)
	}
	// A restart or a shutdown ends everything.
	startExam(page)
	page.do(actionRestart, "", "")
	if exams := page.exams(); len(exams) != 0 {
		t.Fatalf("after a restart = %+v", exams)
	}
}

func TestReopenedPageAndAnotherUsersPageSeeTheSameExamMode(t *testing.T) {
	server, _ := examServer(t, &examActions{})
	teacher := openPage(t, server, 1000)
	startExam(teacher)
	want := teacher.exams()
	if len(want) != 1 {
		t.Fatalf("exam mode = %+v", want)
	}
	// The teacher closes the page and opens it again; the administrator has
	// the classroom view open too. Neither learns it from the page.
	for name, page := range map[string]examPage{"reopened": openPage(t, server, 1000), "administrator": openPage(t, server, 1001)} {
		got := page.exams()
		if len(got) != 1 || got["pc01"].Since != want["pc01"].Since || strings.Join(got["pc01"].Steps, ",") != "internet,lock,message" {
			t.Fatalf("%s page reads %+v, want %+v", name, got, want)
		}
	}
	// And either can end it for both.
	openPage(t, server, 1001).do(actionUnlock, examEnd, "")
	if got := teacher.exams()["pc01"]; strings.Join(got.Steps, ",") != "internet,message" {
		t.Fatalf("the teacher's page after the administrator unlocked: %v", got.Steps)
	}
	// A restart of the worker forgets exam mode: a new service has none.
	fresh, _ := examServer(t, &examActions{})
	if exams := openPage(t, fresh, 1000).exams(); len(exams) != 0 {
		t.Fatalf("a new service remembers %+v", exams)
	}
}

// The page's own script, run against stubs of the document and of app.js's
// helpers (testdata/message_page.js), when node is available.
func TestMessagePageScript(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	output, err := exec.Command(node, "testdata/message_page.js", "view/message.js").CombinedOutput()
	if err != nil || !strings.Contains(string(output), "message.js scenarios passed") {
		t.Fatalf("%v\n%s", err, output)
	}
}

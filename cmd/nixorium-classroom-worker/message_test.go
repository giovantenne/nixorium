package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// noteSession is a student's session that shows messages.
type noteSession struct {
	screenSession
	lab *noteLab
}

func (session noteSession) ShowMessage(text string, seconds int, untilUnlock bool) error {
	session.lab.mutex.Lock()
	defer session.lab.mutex.Unlock()
	session.lab.calls = append(session.lab.calls, "show "+text)
	return nil
}

func (session noteSession) HideMessage() error {
	session.lab.mutex.Lock()
	defer session.lab.mutex.Unlock()
	session.lab.calls = append(session.lab.calls, "hide")
	return nil
}

// noteLab has pc01 that shows messages, pc02 switched off, pc03 with an
// agent that predates them and pc04 whose agent stopped.
type noteLab struct {
	mutex    sync.Mutex
	calls    []string
	connects int
}

func (*noteLab) LabMeta(ctx context.Context, repository string) (domain.LabMeta, error) {
	meta, err := deskLab{}.LabMeta(ctx, repository)
	meta.Clients.Hosts = append(meta.Clients.Hosts, domain.HostMeta{Name: "pc04", IP: "10.0.0.4"})
	return meta, err
}

func (lab *noteLab) Connect(_ context.Context, host domain.HostMeta) (classroomview.Session, error) {
	lab.mutex.Lock()
	lab.connects++
	lab.mutex.Unlock()
	switch host.Name {
	case "pc01":
		return noteSession{lab: lab}, nil
	case "pc03":
		return screenSession{}, nil
	case "pc04":
		return nil, classroomview.AgentError{Code: classroomview.CodeNoAgent}
	}
	return nil, classroomview.ErrUnreachable
}

func noteServer(t *testing.T) (*viewServer, *noteLab, string, string) {
	t.Helper()
	lab := &noteLab{}
	worker := classroomWorker{
		repository: "/srv/lab",
		view:       newViewServer(fakeViewSource{}),
		viewOn:     func() bool { return true },
		message:    app.NewMessageManager(lab),
	}
	worker.view.actions = worker.handle
	address, err := worker.view.Open(1000)
	if err != nil {
		t.Fatal(err)
	}
	host := strings.TrimPrefix(strings.SplitN(address, "/open", 2)[0], "http://")
	cookie := viewRequest(worker.view, "/open"+strings.SplitN(address, "/open", 2)[1], host, "").Result().Cookies()[0].Value
	return worker.view, lab, host, cookie
}

func planReview(t *testing.T, server *viewServer, host, cookie, body string) actionReview {
	t.Helper()
	response := actionPost(server, host, cookie, "http://"+host, "/api/actions/plan", body)
	if strings.Contains(response.Body.String(), "sha256:") {
		t.Fatal("the review token reached the page")
	}
	var review actionReview
	if err := json.Unmarshal(response.Body.Bytes(), &review); err != nil {
		t.Fatalf("review = %d %s", response.Code, response.Body.String())
	}
	return review
}

func applyResult(t *testing.T, server *viewServer, host, cookie, id string) actionResult {
	t.Helper()
	response := actionPost(server, host, cookie, "http://"+host, "/api/actions/apply", `{"id":"`+id+`"}`)
	var result actionResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("result = %d %s", response.Code, response.Body.String())
	}
	return result
}

func TestPageShowsAMessageAfterAReviewThatQuotesIt(t *testing.T) {
	server, lab, host, cookie := noteServer(t)
	// The words of the text never decide what the controller says: this one
	// quotes an administrative blocker, and markup is only text.
	text := "USB installation <b>in progress</b>\nWait for the teacher."
	body, _ := json.Marshal(map[string]any{"action": "message", "computers": []string{"pc01", "pc02", "pc03"}, "message": map[string]any{"text": "  " + text + "\n", "seconds": 30}})
	review := planReview(t, server, host, cookie, string(body))
	if !review.Ready || review.ID == "" || review.Title != "Message" || review.Confirm != "Send the message to 2 computers" {
		t.Fatalf("review = %+v", review)
	}
	if !strings.HasPrefix(review.Message, "“"+text+"”\n\nThis is shown on 2 of 3 computers, above everything, a lock included. It goes away by itself after 30 seconds.") {
		t.Fatalf("review message = %q", review.Message)
	}
	if want := time.Now().UnixMilli(); review.Expires <= want || review.Expires > want+int64(6*time.Minute/time.Millisecond) {
		t.Fatalf("the review ends at %d, now is %d", review.Expires, want)
	}
	if len(review.Rows) != 3 || review.Rows[0].Skip || !review.Rows[1].Skip || review.Rows[1].Note != app.ClassroomOffDetail || review.Rows[2].Skip {
		t.Fatalf("rows = %+v", review.Rows)
	}
	result := applyResult(t, server, host, cookie, review.ID)
	if result.State != "partial" || result.DoneWord != "shown" || len(result.Rows) != 3 {
		t.Fatalf("result = %+v", result)
	}
	if row := result.Rows[0]; row.Outcome != outcomeDone || row.Note != "Message shown." || row.Skip {
		t.Fatalf("pc01 = %+v", row)
	}
	if row := result.Rows[1]; row.Outcome != outcomeOff || row.Note != app.ClassroomOffDetail || !row.Skip {
		t.Fatalf("pc02 = %+v", row)
	}
	if row := result.Rows[2]; row.Outcome != outcomeFailed || !strings.Contains(row.Note, "cannot show messages yet") {
		t.Fatalf("pc03 = %+v", row)
	}
	if len(lab.calls) != 1 || lab.calls[0] != "show "+text {
		t.Fatalf("the computer was asked for %q", lab.calls)
	}
	// One review, one use.
	if again := applyResult(t, server, host, cookie, review.ID); again.State != "expired" || len(lab.calls) != 1 {
		t.Fatalf("second apply = %+v", again)
	}
}

func TestPageCountsAStoppedAgentLikeTheOtherUnavailableComputers(t *testing.T) {
	server, lab, host, cookie := noteServer(t)
	review := planReview(t, server, host, cookie, `{"action":"message","computers":["pc01","pc04"],"message":{"text":"Wait"}}`)
	if !review.Ready || review.Confirm != "Send the message to 1 computer" || len(review.Rows) != 2 {
		t.Fatalf("review = %+v", review)
	}
	if row := review.Rows[1]; !row.Skip || row.Name != "pc04" || row.Note != app.ClassroomNoAgentDetail {
		t.Fatalf("pc04 = %+v", row)
	}
	result := applyResult(t, server, host, cookie, review.ID)
	if row := result.Rows[1]; row.Outcome != outcomeStopped || row.Note != app.ClassroomNoAgentDetail || !row.Skip {
		t.Fatalf("pc04 result = %+v", row)
	}
	if len(lab.calls) != 1 {
		t.Fatalf("calls = %q", lab.calls)
	}
	// Alone, it leaves nothing to send.
	alone := planReview(t, server, host, cookie, `{"action":"message","computers":["pc04"],"message":{"text":"Wait"}}`)
	if alone.Ready || alone.ID != "" {
		t.Fatalf("a stopped agent alone = %+v", alone)
	}
}

func TestPageRemovesAMessage(t *testing.T) {
	server, lab, host, cookie := noteServer(t)
	review := planReview(t, server, host, cookie, `{"action":"message-hide","computers":["pc01","pc03"]}`)
	if !review.Ready || review.Title != "Remove message" || review.Confirm != "Remove the message from 2 computers" || !strings.Contains(review.Message, "removed from 2 of 2 computers") {
		t.Fatalf("review = %+v", review)
	}
	result := applyResult(t, server, host, cookie, review.ID)
	if result.DoneWord != "removed" || len(result.Rows) != 2 || result.Rows[0].Note != "Message removed." || result.Rows[0].Outcome != outcomeDone || len(lab.calls) != 1 || lab.calls[0] != "hide" {
		t.Fatalf("result = %+v, calls = %q", result, lab.calls)
	}
}

func TestPageRefusesAMessageItCannotShowBeforeContactingAComputer(t *testing.T) {
	server, lab, host, cookie := noteServer(t)
	long, _ := json.Marshal(map[string]any{"action": "message", "computers": []string{"pc01"}, "message": map[string]any{"text": strings.Repeat("x", 301)}})
	tab, _ := json.Marshal(map[string]any{"action": "message", "computers": []string{"pc01"}, "message": map[string]any{"text": "a\tb"}})
	for name, test := range map[string]struct{ body, message string }{
		"no message":    {`{"action":"message","computers":["pc01"]}`, "Write the message first."},
		"blank":         {`{"action":"message","computers":["pc01"],"message":{"text":"  \n "}}`, "Write the message first."},
		"too long":      {string(long), "300 characters"},
		"control":       {string(tab), "control characters"},
		"two durations": {`{"action":"message","computers":["pc01"],"message":{"text":"x","seconds":5,"untilUnlock":true}}`, "one duration"},
		"a long time":   {`{"action":"message","computers":["pc01"],"message":{"text":"x","seconds":601}}`, "out of range"},
	} {
		review := planReview(t, server, host, cookie, test.body)
		if review.Ready || review.ID != "" || !strings.Contains(review.Message, test.message) {
			t.Errorf("%s: review = %+v", name, review)
		}
	}
	if lab.connects != 0 {
		t.Fatalf("%d computers were contacted for refused messages", lab.connects)
	}
	// The page cannot add fields of its own to the message.
	response := actionPost(server, host, cookie, "http://"+host, "/api/actions/plan", `{"action":"message","computers":["pc01"],"message":{"text":"x","command":"rm"}}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("an unknown field in the message = %d", response.Code)
	}
}

func TestMessageOperationsNeedTheClassroomViewAndTheFixedDeployment(t *testing.T) {
	lab := &noteLab{}
	worker := classroomWorker{repository: "/srv/lab", viewOn: func() bool { return false }, message: app.NewMessageManager(lab)}
	plan := domain.ClassroomRequest{Operation: domain.ClassroomMessagePlanOperation, Requested: "pc01", MessageAction: domain.MessageShow, MessageContent: &domain.MessageContent{Text: "Wait"}}
	if response := worker.handle(context.Background(), plan); response.State != "failed" || !strings.Contains(response.Message, "CLASSROOM-VIEW-OFF") || lab.connects != 0 {
		t.Fatalf("view off = %+v", response)
	}
	worker.viewOn = func() bool { return true }
	response := worker.handle(context.Background(), plan)
	if response.MessagePlan == nil || response.MessagePlan.HasErrors() {
		t.Fatalf("plan = %+v", response)
	}
	foreign := *response.MessagePlan
	foreign.Repository = "/srv/other"
	apply := domain.ClassroomRequest{Operation: domain.ClassroomMessageApplyOperation, MessagePlan: &foreign}
	if refused := worker.handle(context.Background(), apply); refused.State != "failed" || refused.MessageReport != nil || len(lab.calls) != 0 {
		t.Fatalf("a review of another deployment = %+v", refused)
	}
	if refused := worker.handle(context.Background(), domain.ClassroomRequest{Operation: domain.ClassroomMessageApplyOperation}); refused.State != "failed" {
		t.Fatalf("apply without a review = %+v", refused)
	}
	worker.message = nil
	if refused := worker.handle(context.Background(), plan); refused.State != "failed" || !strings.Contains(refused.Message, "not available") {
		t.Fatalf("no manager = %+v", refused)
	}
}

func TestPageOffersMessage(t *testing.T) {
	page, err := viewAssets.ReadFile("view/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	for _, want := range []string{
		`<script src="/message.js" defer></script>`,
		`data-action="message"`, `data-action="message-hide"`,
		`id="message-dialog"`, `maxlength="300"`, `>Send a message…</button>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	// A computer's Actions menu offers the message too.
	_, rest, found := strings.Cut(html, `class="actions-menu viewer-actions"`)
	block, _, closed := strings.Cut(rest, "</details>")
	if !found || !closed || !strings.Contains(block, `data-action="message"`) || !strings.Contains(block, `data-action="message-hide"`) {
		t.Fatal("a computer's Actions menu has no message buttons")
	}
	server, _, host, cookie := noteServer(t)
	served := viewRequest(server, "/message.js", host, cookie)
	if served.Code != http.StatusOK || !strings.HasPrefix(served.Header().Get("Content-Type"), "text/javascript") || !strings.Contains(served.Body.String(), "classroomTools") {
		t.Fatalf("/message.js = %d %q", served.Code, served.Header())
	}
}

func TestToolbarSendCollectMenu(t *testing.T) {
	page, err := viewAssets.ReadFile("view/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	_, rest, found := strings.Cut(html, "<summary>Share files</summary>")
	menu, _, closed := strings.Cut(rest, "</details>")
	if !found || !closed {
		t.Fatal("the toolbar has no Share files menu")
	}
	last := -1
	for _, want := range []string{`>Send a folder…</button>`, `>Send a file…</button>`, `data-action="collect"`, `>Collect from students' desktops</button>`} {
		at := strings.Index(menu, want)
		if at <= last {
			t.Fatalf("Share files menu lacks %s in order: %s", want, menu)
		}
		last = at
	}
	toolbar, _, _ := strings.Cut(html, "</header>")
	for _, gone := range []string{">Send files<", `id="refresh"`, ">Refresh<", "Exam mode", `data-action="exam"`} {
		if strings.Contains(toolbar, gone) {
			t.Errorf("the toolbar still has %s", gone)
		}
	}
	if strings.Count(toolbar, `data-action="collect"`) != 1 {
		t.Error("Collect from students' desktops is expected only in the Share files menu")
	}
	// A computer's Actions menu keeps the three together, in the same order.
	_, rest, found = strings.Cut(html, `class="actions-menu viewer-actions"`)
	block, _, _ := strings.Cut(rest, "</details>")
	folder, file, collect := strings.Index(block, "Send a folder…"), strings.Index(block, "Send a file…"), strings.Index(block, "Collect from students' desktops")
	if !found || folder < 0 || file < folder || collect < file || strings.Contains(block[folder:collect], "data-action=\"wake\"") {
		t.Error("a computer's Actions menu lacks Send a folder…, Send a file…, Collect from students' desktops in order")
	}
}

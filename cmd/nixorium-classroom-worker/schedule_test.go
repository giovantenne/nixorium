package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

// memorySchedule keeps the schedule record in memory.
type memorySchedule struct {
	record  domain.ScheduleRecord
	failure error
}

func (store *memorySchedule) ReadSchedule() domain.ScheduleRecord {
	if store.record.Days == nil {
		store.record.Days = []string{}
	}
	return store.record
}

func (store *memorySchedule) UpdateSchedule(change func(domain.ScheduleRecord) (domain.ScheduleRecord, bool)) error {
	if store.failure != nil {
		return store.failure
	}
	next, save := change(store.ReadSchedule())
	if save {
		if next.Empty() {
			next = domain.ScheduleRecord{}
		}
		store.record = next
	}
	return nil
}

func scheduleWorker() (classroomWorker, *memorySchedule) {
	store := &memorySchedule{}
	return classroomWorker{repository: "/home/admin/nixorium-deployment", schedule: app.NewScheduleManager(store)}, store
}

func scheduleRequest(operation domain.ClassroomOperation, schedule *domain.Schedule) domain.ClassroomRequest {
	return domain.ClassroomRequest{Operation: operation, Schedule: schedule}
}

// The teacher reaches the schedule only through the classroom socket, whose
// requests the worker validates like every classroom operation.
func TestTeacherSetsAndReadsTheScheduleThroughTheClassroomWorker(t *testing.T) {
	worker, store := scheduleWorker()
	ctx := context.Background()
	if shown := worker.handle(ctx, scheduleRequest(domain.ClassroomScheduleGetOperation, nil)); shown.State != "completed" || shown.Schedule == nil || shown.Schedule.Message != "No schedule." {
		t.Fatalf("empty schedule = %+v", shown)
	}
	saved := worker.handle(ctx, scheduleRequest(domain.ClassroomScheduleSetOperation, &domain.Schedule{On: "07:50", Off: "18:00", Days: []string{"mon", "tue", "wed", "thu", "fri"}}))
	if saved.State != "completed" || saved.Schedule == nil || saved.Schedule.HasErrors() || store.record.On != "07:50" || store.record.Off != "18:00" {
		t.Fatalf("saved = %+v, record %+v", saved, store.record)
	}
	shown := worker.handle(ctx, scheduleRequest(domain.ClassroomScheduleGetOperation, nil))
	if shown.Schedule == nil || shown.Schedule.Summary != "Switch on at 07:50 and shut down at 18:00, Monday to Friday." {
		t.Fatalf("shown = %+v", shown)
	}
	refused := worker.handle(ctx, scheduleRequest(domain.ClassroomScheduleSetOperation, &domain.Schedule{On: "07:50"}))
	if refused.Schedule == nil || !refused.Schedule.HasErrors() || store.record.Off != "18:00" {
		t.Fatalf("a schedule without days was accepted: %+v, record %+v", refused, store.record)
	}
	if missing := worker.handle(ctx, scheduleRequest(domain.ClassroomScheduleSetOperation, nil)); missing.State != "failed" || missing.Schedule != nil {
		t.Fatalf("missing schedule = %+v", missing)
	}
	cleared := worker.handle(ctx, scheduleRequest(domain.ClassroomScheduleSetOperation, &domain.Schedule{Days: []string{}}))
	if cleared.Schedule == nil || cleared.Schedule.Message != "The schedule was cleared." || !store.record.Empty() {
		t.Fatalf("cleared = %+v", cleared)
	}
	store.failure = errors.New("the classroom state directory is not private to this account")
	if failed := worker.handle(ctx, scheduleRequest(domain.ClassroomScheduleSetOperation, &domain.Schedule{On: "07:50", Days: []string{"mon"}})); failed.Schedule == nil || !failed.Schedule.HasErrors() {
		t.Fatalf("store failure = %+v", failed)
	}
	if unavailable := (classroomWorker{}).handle(ctx, scheduleRequest(domain.ClassroomScheduleGetOperation, nil)); unavailable.State != "failed" {
		t.Fatalf("worker without schedule = %+v", unavailable)
	}
	// The classroom socket carries both operations, and not the run.
	for _, operation := range []domain.ClassroomOperation{domain.ClassroomScheduleGetOperation, domain.ClassroomScheduleSetOperation} {
		request, _ := domain.NewClassroomRequest(operation)
		encoded, _ := json.Marshal(request)
		if _, err := domain.DecodeClassroomRequest(encoded); err != nil {
			t.Fatalf("%s is not a classroom operation: %v", operation, err)
		}
	}
}

func schedulePageRequest(server *viewServer, host, cookie, method, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "/api/schedule", strings.NewReader(body))
	request.Host = host
	request.AddCookie(&http.Cookie{Name: viewCookie, Value: cookie})
	if method == http.MethodPost {
		request.Header.Set("Origin", "http://"+host)
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func TestPageShowsAndSavesTheSchedule(t *testing.T) {
	worker, store := scheduleWorker()
	server, host, cookie := openedViewServer(t, &fakeActions{})
	server.actions = worker.handle

	var report domain.ScheduleReport
	response := schedulePageRequest(server, host, cookie, http.MethodGet, "")
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil || response.Code != http.StatusOK || report.State != "completed" || !report.Schedule.Empty() {
		t.Fatalf("empty schedule = %d %s", response.Code, response.Body.String())
	}
	response = schedulePageRequest(server, host, cookie, http.MethodPost, `{"on":"07:50","off":"18:00","days":["mon","tue","wed","thu","fri"]}`)
	report = domain.ScheduleReport{}
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil || report.State != "completed" || report.Schedule.On != "07:50" || store.record.Off != "18:00" ||
		report.Summary != "Switch on at 07:50 and shut down at 18:00, Monday to Friday." {
		t.Fatalf("saved = %d %s", response.Code, response.Body.String())
	}
	// A refused schedule comes back as a message for the dialog, and the
	// stored schedule stays.
	response = schedulePageRequest(server, host, cookie, http.MethodPost, `{"on":"07:50","off":"07:50","days":["mon"]}`)
	report = domain.ScheduleReport{}
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil || report.State != "blocked" || !strings.Contains(report.Message, "more than 10 minutes apart") || store.record.On != "07:50" || len(store.record.Days) != 5 {
		t.Fatalf("refused = %d %s", response.Code, response.Body.String())
	}
	response = schedulePageRequest(server, host, cookie, http.MethodPost, `{"on":"","off":"","days":[]}`)
	report = domain.ScheduleReport{}
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil || report.State != "completed" || !store.record.Empty() {
		t.Fatalf("cleared = %d %s", response.Code, response.Body.String())
	}
}

func TestPageScheduleEndpointKeepsTheViewsRules(t *testing.T) {
	worker, store := scheduleWorker()
	server, host, cookie := openedViewServer(t, &fakeActions{})
	server.actions = worker.handle
	body := `{"on":"07:50","days":["mon"]}`
	post := func(origin, contentType, session string) int {
		request := httptest.NewRequest(http.MethodPost, "/api/schedule", strings.NewReader(body))
		request.Host = host
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", contentType)
		if session != "" {
			request.AddCookie(&http.Cookie{Name: viewCookie, Value: session})
		}
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		return recorder.Code
	}
	for name, code := range map[string]int{
		"foreign origin": post("http://evil.example", "application/json", cookie),
		"form post":      post("http://"+host, "application/x-www-form-urlencoded", cookie),
		"no session":     post("http://"+host, "application/json", ""),
	} {
		if code == http.StatusOK {
			t.Fatalf("%s was accepted", name)
		}
	}
	if !store.record.Empty() {
		t.Fatalf("a refused request saved %+v", store.record)
	}
	if unknown := schedulePageRequest(server, host, cookie, http.MethodPost, `{"on":"07:50","days":["mon"],"command":"x"}`); unknown.Code == http.StatusOK || !store.record.Empty() {
		t.Fatalf("unknown field = %d", unknown.Code)
	}
	// Reading needs the session too.
	if anonymous := viewRequest(server, "/api/schedule", host, ""); anonymous.Code != http.StatusForbidden {
		t.Fatalf("anonymous read = %d", anonymous.Code)
	}
	// Other methods are refused.
	request := httptest.NewRequest(http.MethodPut, "/api/schedule", strings.NewReader(body))
	request.Host = host
	request.AddCookie(&http.Cookie{Name: viewCookie, Value: cookie})
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusOK {
		t.Fatalf("PUT = %d", recorder.Code)
	}
}

func TestPageServesTheScheduleDialogUnderPower(t *testing.T) {
	server, host, cookie := openedViewServer(t, &fakeActions{})
	script := viewRequest(server, "/schedule.js", host, cookie)
	if script.Code != http.StatusOK || !strings.HasPrefix(script.Header().Get("Content-Type"), "text/javascript") || !strings.Contains(script.Body.String(), "/api/schedule") {
		t.Fatalf("script = %d %q", script.Code, script.Header().Get("Content-Type"))
	}
	if anonymous := viewRequest(server, "/schedule.js", host, ""); anonymous.Code != http.StatusForbidden {
		t.Fatalf("anonymous script = %d", anonymous.Code)
	}
	page, err := viewAssets.ReadFile("view/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	// Schedule… sits in the Power menu, which opens without a selection, and
	// is not an action on selected computers.
	_, rest, found := strings.Cut(html, `class="actions-menu power-menu"`)
	block, _, closed := strings.Cut(rest, "</details>")
	if !found || !closed || !strings.HasPrefix(rest, " data-always>") || !strings.Contains(block, `id="schedule-open"`) || !strings.Contains(block, ">Schedule…</button>") || strings.Contains(block, `id="schedule-open" data-action`) {
		t.Fatalf("Power menu = %q", block)
	}
	for _, want := range []string{`<script src="/schedule.js" defer></script>`, `<dialog id="schedule-dialog"`, `id="schedule-save"`, `id="schedule-clear"`, `id="schedule-cancel"`, `name="schedule-day"`,
		"The computers must allow waking from the network, and a computer that someone is using is never shut down."} {
		if !strings.Contains(html, want) {
			t.Fatalf("index.html lacks %s", want)
		}
	}
	// The page script keeps the Power menu usable without a selection.
	app, err := viewAssets.ReadFile("view/app.js")
	if err != nil || !strings.Contains(string(app), "!menu.hasAttribute('data-always')") {
		t.Fatalf("app.js does not keep data-always menus enabled: %v", err)
	}
	for _, name := range []string{"/api/schedule"} {
		if !postPaths[name] {
			t.Fatalf("%s does not accept a POST from the page", name)
		}
	}
}

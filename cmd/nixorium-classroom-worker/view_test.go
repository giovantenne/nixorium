package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeViewSource struct{}

func (fakeViewSource) Computers(context.Context) ([]app.ClassroomComputer, error) {
	return []app.ClassroomComputer{{Name: "pc01", State: app.ClassroomViewing, HasImage: true}}, nil
}

func (fakeViewSource) Thumbnail(name string) ([]byte, bool) {
	return []byte{0xff, 0xd8}, name == "pc01"
}

func (fakeViewSource) Frame(name string, since int64) ([]byte, int64, bool) {
	if name != "pc01" {
		return nil, 0, false
	}
	if since == 4 {
		return nil, 4, true
	}
	return []byte{0xff, 0xd8}, 4, true
}

var receivedInput []classroomview.InputEvent

func (fakeViewSource) Input(name string, events []classroomview.InputEvent, release bool) error {
	if name != "pc01" {
		return errors.New("not viewed")
	}
	receivedInput = events
	return nil
}

// clipboardSource records the shared clipboard calls; only pc01 is under
// control after Control("pc01", true).
var clipboardCalls []string
var clipboardControlled bool

func (fakeViewSource) Control(name string, on bool) error {
	clipboardControlled = on && name == "pc01"
	return nil
}

func (fakeViewSource) ClipboardSet(name, text string) error {
	if name != "pc01" || !clipboardControlled {
		return app.ErrClassroomNotControlled
	}
	clipboardCalls = append(clipboardCalls, "set:"+text)
	return nil
}

func (fakeViewSource) ClipboardGet(name string) (string, error) {
	if name != "pc01" || !clipboardControlled {
		return "", app.ErrClassroomNotControlled
	}
	return "copied", nil
}

func viewRequest(server *viewServer, path, host, cookie string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.Host = host
	if cookie != "" {
		request.AddCookie(&http.Cookie{Name: viewCookie, Value: cookie})
	}
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)
	return recorder
}

func TestOpenPageStopsAnsweringWhenTheViewIsTurnedOff(t *testing.T) {
	server := newViewServer(fakeViewSource{})
	on := true
	server.enabled = func() bool { return on }
	address, err := server.Open(1000)
	if err != nil {
		t.Fatal(err)
	}
	host := strings.TrimPrefix(strings.SplitN(address, "/open", 2)[0], "http://")
	entered := viewRequest(server, "/open"+strings.SplitN(address, "/open", 2)[1], host, "")
	cookie := entered.Result().Cookies()[0]
	if page := viewRequest(server, "/api/computers", host, cookie.Value); page.Code != http.StatusOK {
		t.Fatalf("on = %d", page.Code)
	}
	on = false
	page := viewRequest(server, "/api/computers", host, cookie.Value)
	if page.Code != http.StatusForbidden || !strings.Contains(page.Body.String(), "turned off") {
		t.Fatalf("off = %d %q", page.Code, page.Body.String())
	}
}

func TestClassroomViewNeedsTheOneTimeTokenAndTheLoopbackHost(t *testing.T) {
	server := newViewServer(fakeViewSource{})
	address, err := server.Open(1000)
	if err != nil || !strings.HasPrefix(address, "http://127.0.0.1:") || !strings.Contains(address, "/open?token=") {
		t.Fatalf("address = %q, %v", address, err)
	}
	host := strings.TrimPrefix(strings.SplitN(address, "/open", 2)[0], "http://")
	path := "/open" + strings.SplitN(address, "/open", 2)[1]

	if response := viewRequest(server, "/api/computers", host, ""); response.Code != http.StatusForbidden {
		t.Fatalf("anonymous request = %d", response.Code)
	}
	if response := viewRequest(server, path, "evil.example:80", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("foreign host = %d", response.Code)
	}
	response := viewRequest(server, path, host, "")
	if response.Code != http.StatusSeeOther || len(response.Result().Cookies()) != 1 {
		t.Fatalf("enter = %d %v", response.Code, response.Result().Cookies())
	}
	cookie := response.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie = %+v", cookie)
	}
	if again := viewRequest(server, path, host, ""); again.Code != http.StatusForbidden {
		t.Fatalf("token reused = %d", again.Code)
	}
	page := viewRequest(server, "/", host, cookie.Value)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Classroom view") || !strings.Contains(page.Header().Get("Content-Security-Policy"), "default-src 'none'") ||
		// The enlarged view shows each frame through a blob: URL.
		!strings.Contains(page.Header().Get("Content-Security-Policy"), "img-src 'self' blob:;") {
		t.Fatalf("page = %d %q", page.Code, page.Header())
	}
	if list := viewRequest(server, "/api/computers", host, cookie.Value); list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"pc01"`) {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	if image := viewRequest(server, "/api/thumbnail?name=pc01", host, cookie.Value); image.Code != http.StatusOK || image.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("image = %d", image.Code)
	}
	for _, name := range []string{"../etc", "pc09"} {
		if image := viewRequest(server, "/api/thumbnail?name="+name, host, cookie.Value); image.Code == http.StatusOK {
			t.Fatalf("thumbnail %q served", name)
		}
	}
	// The enlarged view: a frame, then nothing until it changes.
	if frame := viewRequest(server, "/api/frame?name=pc01&since=0", host, cookie.Value); frame.Code != http.StatusOK || frame.Header().Get("X-Frame") != "4" {
		t.Fatalf("frame = %d %v", frame.Code, frame.Header())
	}
	if same := viewRequest(server, "/api/frame?name=pc01&since=4", host, cookie.Value); same.Code != http.StatusNoContent {
		t.Fatalf("unchanged frame = %d", same.Code)
	}
	// Input only from this page, as JSON, with the session.
	input := func(origin, contentType, body, session string) int {
		request := httptest.NewRequest(http.MethodPost, "/api/input?name=pc01", strings.NewReader(body))
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
	events := `{"events":[{"kind":"key","keysym":97,"pressed":true}],"release":false}`
	if code := input("http://"+host, "application/json", events, cookie.Value); code != http.StatusNoContent || len(receivedInput) != 1 || receivedInput[0].Keysym != 97 {
		t.Fatalf("input = %d %+v", code, receivedInput)
	}
	for name, code := range map[string]int{
		"foreign origin": input("http://evil.example", "application/json", events, cookie.Value),
		"form post":      input("http://"+host, "application/x-www-form-urlencoded", events, cookie.Value),
		"no session":     input("http://"+host, "application/json", events, ""),
		"unknown field":  input("http://"+host, "application/json", `{"command":"x"}`, cookie.Value),
	} {
		if code == http.StatusNoContent {
			t.Fatalf("%s was accepted", name)
		}
	}
	// Expired tokens and sessions are refused.
	server.now = func() time.Time { return time.Now().Add(13 * time.Hour) }
	if expired := viewRequest(server, "/api/computers", host, cookie.Value); expired.Code != http.StatusForbidden {
		t.Fatalf("expired session = %d", expired.Code)
	}
}

func TestClassroomViewOpensOnlyWhenTurnedOn(t *testing.T) {
	worker := classroomWorker{view: newViewServer(fakeViewSource{}), viewOn: func() bool { return false }}
	ctx := adapters.WithClassroomPeerUID(context.Background(), 1000)
	response := worker.handle(ctx, domain.ClassroomRequest{Operation: domain.ClassroomViewOpenOperation})
	if response.State != "failed" || !strings.Contains(response.Message, "CLASSROOM-VIEW-OFF") || response.ViewURL != "" {
		t.Fatalf("response = %+v", response)
	}
	worker.viewOn = func() bool { return true }
	response = worker.handle(ctx, domain.ClassroomRequest{Operation: domain.ClassroomViewOpenOperation})
	if response.State != "completed" || !strings.HasPrefix(response.ViewURL, "http://127.0.0.1:") {
		t.Fatalf("response = %+v", response)
	}
}

func TestPageNamesAComputerWhoseMonitoringStopped(t *testing.T) {
	script, err := viewAssets.ReadFile("view/app.js")
	if err != nil {
		t.Fatal(err)
	}
	page, err := viewAssets.ReadFile("view/index.html")
	if err != nil {
		t.Fatal(err)
	}
	// The card says it with the words the reviews and commands use, in its
	// own cover, and the result notice counts it apart from the computers
	// that are off.
	for _, words := range []string{
		"'no-agent': '" + app.ClassroomNoAgentLabel + "'",
		"caption: '" + app.ClassroomNoAgentLabel + "'",
		"if (computer.state === 'no-agent') return 'stopped';",
		"stopped: '" + strings.ToLower(app.ClassroomNoAgentLabel[:1]) + app.ClassroomNoAgentLabel[1:] + "'",
		// The other states keep their words.
		"locked: 'Idle'",
		"unreachable: 'Off or not reachable'",
		"off: 'switched off or not reachable'",
	} {
		if !strings.Contains(string(script), words) {
			t.Fatalf("app.js lacks %s", words)
		}
	}
	if !strings.Contains(string(page), `id="icon-alert"`) {
		t.Fatal("index.html lacks the warning icon of the stopped state")
	}
}

func TestSharedClipboardEndpoints(t *testing.T) {
	server := newViewServer(fakeViewSource{})
	address, _ := server.Open(1000)
	host := strings.TrimPrefix(strings.SplitN(address, "/open", 2)[0], "http://")
	cookie := viewRequest(server, "/open"+strings.SplitN(address, "/open", 2)[1], host, "").Result().Cookies()[0]
	post := func(path, body, session string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Host = host
		request.Header.Set("Origin", "http://"+host)
		request.Header.Set("Content-Type", "application/json")
		if session != "" {
			request.AddCookie(&http.Cookie{Name: viewCookie, Value: session})
		}
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		return recorder
	}
	clipboardCalls, clipboardControlled = nil, false
	// Without control nothing reaches the computer, and the words are plain.
	if response := post("/api/clipboard/set", `{"computer":"pc01","text":"hi"}`, cookie.Value); response.Code != http.StatusConflict || strings.Contains(response.Body.String(), "classroom") || len(clipboardCalls) != 0 {
		t.Fatalf("set without control = %d %q", response.Code, response.Body.String())
	}
	if response := post("/api/clipboard/get", `{"computer":"pc01"}`, cookie.Value); response.Code != http.StatusConflict {
		t.Fatalf("get without control = %d", response.Code)
	}
	if response := post("/api/control", `{"computer":"pc01","on":true}`, cookie.Value); response.Code != http.StatusNoContent {
		t.Fatalf("control = %d", response.Code)
	}
	if response := post("/api/clipboard/set", `{"computer":"pc01","text":"a\u0000b"}`, cookie.Value); response.Code != http.StatusNoContent || len(clipboardCalls) != 1 || clipboardCalls[0] != "set:ab" {
		t.Fatalf("set = %d %v", response.Code, clipboardCalls)
	}
	if response := post("/api/clipboard/get", `{"computer":"pc01"}`, cookie.Value); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"text":"copied"`) {
		t.Fatalf("get = %d %q", response.Code, response.Body.String())
	}
	// Another computer, an oversized text, a stranger and unknown fields.
	if response := post("/api/clipboard/set", `{"computer":"pc02","text":"hi"}`, cookie.Value); response.Code != http.StatusConflict {
		t.Fatalf("other computer = %d", response.Code)
	}
	big, _ := json.Marshal(map[string]string{"computer": "pc01", "text": strings.Repeat("x", classroomview.MaxClipboardBytes+1)})
	if response := post("/api/clipboard/set", string(big), cookie.Value); response.Code != http.StatusRequestEntityTooLarge || !strings.Contains(response.Body.String(), "64 KiB") || len(clipboardCalls) != 1 {
		t.Fatalf("large text = %d %q", response.Code, response.Body.String())
	}
	if response := post("/api/clipboard/set", `{"computer":"pc01","text":"hi"}`, ""); response.Code == http.StatusNoContent {
		t.Fatal("clipboard without a session")
	}
	if response := post("/api/clipboard/set", `{"computer":"pc01","text":"hi","x":1}`, cookie.Value); response.Code != http.StatusBadRequest {
		t.Fatalf("unknown field = %d", response.Code)
	}
	if response := post("/api/control", `{"computer":"pc01","on":false}`, cookie.Value); response.Code != http.StatusNoContent {
		t.Fatalf("control end = %d", response.Code)
	}
	if response := post("/api/clipboard/get", `{"computer":"pc01"}`, cookie.Value); response.Code != http.StatusConflict {
		t.Fatalf("get after control = %d", response.Code)
	}
}

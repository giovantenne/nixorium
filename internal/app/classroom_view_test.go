package app

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

type fakeAgentSession struct {
	mutex  *sync.Mutex
	closed *int
	inputs *int
	reply  classroomview.Message
}

func (session fakeAgentSession) Thumbnail(width int, since int64) (classroomview.Message, error) {
	return session.reply, nil
}

func (session fakeAgentSession) SetLocked(locked bool) (bool, error) { return locked, nil }

func (session fakeAgentSession) Locked() bool { return false }

func (session fakeAgentSession) SendFiles([]classroomview.FileEntry, func(int) (io.ReadCloser, error)) ([]string, error) {
	return nil, nil
}

func (session fakeAgentSession) Input(events []classroomview.InputEvent, release bool) error {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	*session.inputs += len(events)
	return nil
}

func (session fakeAgentSession) Close() error {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	*session.closed++
	return nil
}

type fakeAgentConnector struct {
	mutex  sync.Mutex
	closed int
	inputs int
	errors map[string]error
	codes  map[string]string
}

func (connector *fakeAgentConnector) Connect(_ context.Context, host domain.HostMeta) (classroomview.Session, error) {
	if err := connector.errors[host.Name]; err != nil {
		return nil, err
	}
	reply := classroomview.Message{Type: classroomview.TypeThumbnail, Width: 320, Height: 200, Image: []byte(host.Name), CapturedAt: 1, Frame: 3}
	if code := connector.codes[host.Name]; code != "" {
		reply = classroomview.Message{Type: classroomview.TypeError, Code: code}
	}
	return fakeAgentSession{mutex: &connector.mutex, closed: &connector.closed, inputs: &connector.inputs, reply: reply}, nil
}

func TestClassroomViewHubShowsEveryComputerAndStopsWhenNobodyLooks(t *testing.T) {
	connector := &fakeAgentConnector{
		errors: map[string]error{"pc03": classroomview.ErrUnreachable},
		codes:  map[string]string{"pc02": classroomview.CodeNoSession},
	}
	hosts := []domain.HostMeta{{Name: "pc03"}, {Name: "pc01"}, {Name: "pc02"}}
	hub := NewClassroomViewHub(func(context.Context) ([]domain.HostMeta, error) { return hosts, nil }, connector)
	hub.interval, hub.idle = 10*time.Millisecond, 200*time.Millisecond

	computers, err := hub.Computers(context.Background())
	if err != nil || len(computers) != 3 || computers[0].Name != "pc01" || computers[0].State != ClassroomConnecting {
		t.Fatalf("computers = %+v, %v", computers, err)
	}
	deadline := time.Now().Add(2 * time.Second)
	states := map[string]string{}
	for time.Now().Before(deadline) {
		computers, _ = hub.Computers(context.Background())
		for _, computer := range computers {
			states[computer.Name] = computer.State
		}
		if states["pc01"] == ClassroomViewing && states["pc02"] == ClassroomNoSession && states["pc03"] == ClassroomUnreachable {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if states["pc01"] != ClassroomViewing || states["pc02"] != ClassroomNoSession || states["pc03"] != ClassroomUnreachable {
		t.Fatalf("states = %v", states)
	}
	if image, found := hub.Thumbnail("pc01"); !found || string(image) != "pc01" {
		t.Fatalf("thumbnail = %q, %v", image, found)
	}
	if _, found := hub.Thumbnail("pc09"); found {
		t.Fatal("unknown computer returned an image")
	}
	// Opened in full: the latest frame, nothing again until it changes, and input.
	if image, frame, found := hub.Frame("pc01", 0); !found || frame != 3 || string(image) != "pc01" {
		t.Fatalf("frame = %q %d %v", image, frame, found)
	}
	if image, _, _ := hub.Frame("pc01", 3); image != nil {
		t.Fatal("an unchanged frame was sent again")
	}
	if err := hub.Input("pc01", []classroomview.InputEvent{{Kind: classroomview.InputKey, Keysym: 'a', Pressed: true}}, false); err != nil {
		t.Fatal(err)
	}
	if err := hub.Input("pc01", []classroomview.InputEvent{{Kind: "exec"}}, false); err == nil {
		t.Fatal("malformed input was accepted")
	}
	if err := hub.Input("pc02", nil, false); err == nil {
		t.Fatal("input reached a computer that is not open in full")
	}
	// Nobody asks any more: every channel closes and polling stops.
	time.Sleep(500 * time.Millisecond)
	hub.mutex.Lock()
	running := hub.running["pc01"] || hub.running["pc02"] || hub.running["pc03"]
	hub.mutex.Unlock()
	connector.mutex.Lock()
	closed := connector.closed
	connector.mutex.Unlock()
	if running || closed == 0 {
		t.Fatalf("polling still running=%v, closed sessions=%d", running, closed)
	}
}

func TestClassroomFailureStates(t *testing.T) {
	for code, want := range map[string]string{
		classroomview.CodeNoSession: ClassroomNoSession,
		classroomview.CodeNoAgent:   ClassroomNoAgent,
		classroomview.CodeLocked:    ClassroomLocked,
		classroomview.CodeBusy:      ClassroomConnecting,
		classroomview.CodeCapture:   ClassroomFailed,
	} {
		if state, _ := classroomFailureState(classroomview.AgentError{Code: code}); state != want {
			t.Fatalf("code %s state = %s, want %s", code, state, want)
		}
	}
}

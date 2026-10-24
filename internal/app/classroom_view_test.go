package app

import (
	"context"
	"errors"
	"io"
	"strings"
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
	// clip records the shared clipboard calls ("begin", "set:text", "end").
	clip   *[]string
	copied string
}

func (session fakeAgentSession) note(call string) {
	if session.clip != nil {
		session.mutex.Lock()
		*session.clip = append(*session.clip, call)
		session.mutex.Unlock()
	}
}
func (session fakeAgentSession) ClipboardBegin() error { session.note("begin"); return nil }
func (session fakeAgentSession) ClipboardEnd() error   { session.note("end"); return nil }
func (session fakeAgentSession) ClipboardSet(text string) error {
	session.note("set:" + text)
	return nil
}
func (session fakeAgentSession) ClipboardGet() (string, error) { return session.copied, nil }

func (session fakeAgentSession) Thumbnail(width int, since int64) (classroomview.Message, error) {
	return session.reply, nil
}

func (session fakeAgentSession) SetLocked(locked bool) (bool, error) { return locked, nil }

func (session fakeAgentSession) Locked() bool { return false }

func (session fakeAgentSession) ShowFrame([]byte) error { return nil }
func (session fakeAgentSession) StopBroadcast() error   { return nil }

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
	clip   []string
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
	return fakeAgentSession{mutex: &connector.mutex, closed: &connector.closed, inputs: &connector.inputs, reply: reply, clip: &connector.clip, copied: "from pc\x00"}, nil
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
	hub.frameWait = 20 * time.Millisecond
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

// stoppableConnector reaches a computer whose agent can be stopped and
// started again, as systemd does after a student kills it.
type stoppableConnector struct {
	mutex   sync.Mutex
	stopped bool
}

func (connector *stoppableConnector) setStopped(stopped bool) {
	connector.mutex.Lock()
	defer connector.mutex.Unlock()
	connector.stopped = stopped
}

func (connector *stoppableConnector) Connect(_ context.Context, host domain.HostMeta) (classroomview.Session, error) {
	connector.mutex.Lock()
	defer connector.mutex.Unlock()
	if connector.stopped {
		return nil, classroomview.AgentError{Code: classroomview.CodeNoAgent}
	}
	var closed, inputs int
	reply := classroomview.Message{Type: classroomview.TypeThumbnail, Width: 320, Height: 200, Image: []byte(host.Name), CapturedAt: 1, Frame: 3}
	return fakeAgentSession{mutex: &sync.Mutex{}, closed: &closed, inputs: &inputs, reply: reply}, nil
}

func TestClassroomCardSaysWhenMonitoringStoppedAndRecovers(t *testing.T) {
	connector := &stoppableConnector{stopped: true}
	hub := NewClassroomViewHub(func(context.Context) ([]domain.HostMeta, error) {
		return []domain.HostMeta{{Name: "pc01"}}, nil
	}, connector)
	hub.interval, hub.idle = 10*time.Millisecond, 2*time.Second
	waitFor := func(state string) ClassroomComputer {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			computers, err := hub.Computers(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if computers[0].State == state || time.Now().After(deadline) {
				return computers[0]
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	// Somebody is signed in and the computer answers, but not its agent: a
	// state of its own, neither "Off or not reachable" nor "Nobody is signed in".
	stopped := waitFor(ClassroomNoAgent)
	if stopped.State != ClassroomNoAgent || stopped.Detail != "Monitoring stopped on this PC." {
		t.Fatalf("card = %+v", stopped)
	}
	// systemd starts the agent again: the card returns to the live picture.
	connector.setStopped(false)
	if card := waitFor(ClassroomViewing); card.State != ClassroomViewing || card.Detail != "" {
		t.Fatalf("card after the restart = %+v", card)
	}
}

func TestClassroomFailureStates(t *testing.T) {
	// A stopped agent has its own words, apart from the other states.
	if state, detail := classroomFailureState(classroomview.AgentError{Code: classroomview.CodeNoAgent}); state != ClassroomNoAgent || detail != "Monitoring stopped on this PC." {
		t.Fatalf("no agent = %s, %q", state, detail)
	}
	if _, detail := classroomFailureState(classroomview.ErrUnreachable); detail != ClassroomOffDetail || detail == ClassroomNoAgentDetail {
		t.Fatalf("an SSH failure says %q", detail)
	}
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

func TestClassroomViewHubReportsComputersThatAnswered(t *testing.T) {
	connector := &fakeAgentConnector{
		errors: map[string]error{
			"pc02": classroomview.AgentError{Code: classroomview.CodeNoSession},
			"pc03": classroomview.ErrUnreachable,
		},
	}
	hosts := []domain.HostMeta{{Name: "pc01"}, {Name: "pc02"}, {Name: "pc03"}}
	hub := NewClassroomViewHub(func(context.Context) ([]domain.HostMeta, error) { return hosts, nil }, connector)
	hub.interval, hub.idle = 10*time.Millisecond, 100*time.Millisecond
	var mutex sync.Mutex
	answered := map[string]bool{}
	hub.Answered = func(host domain.HostMeta) {
		mutex.Lock()
		defer mutex.Unlock()
		answered[host.Name] = true
	}
	if _, err := hub.Computers(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		mutex.Lock()
		done := answered["pc01"] && answered["pc02"]
		mutex.Unlock()
		if done || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mutex.Lock()
	defer mutex.Unlock()
	// Nobody signed in still answers over the network; a computer that is
	// off does not.
	if !answered["pc01"] || !answered["pc02"] || answered["pc03"] {
		t.Fatalf("answered = %v", answered)
	}
}

func TestClassroomClipboardWorksOnlyUnderControl(t *testing.T) {
	connector := &fakeAgentConnector{}
	hub := NewClassroomViewHub(func(context.Context) ([]domain.HostMeta, error) {
		return []domain.HostMeta{{Name: "pc01"}}, nil
	}, connector)
	hub.interval, hub.idle = 10*time.Millisecond, 5*time.Second
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		computers, _ := hub.Computers(context.Background())
		if computers[0].State == ClassroomViewing {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	hub.Frame("pc01", 0)
	if err := hub.ClipboardSet("pc01", "hello"); !errors.Is(err, ErrClassroomNotControlled) {
		t.Fatalf("set without control: %v", err)
	}
	if _, err := hub.ClipboardGet("pc01"); !errors.Is(err, ErrClassroomNotControlled) {
		t.Fatalf("get without control: %v", err)
	}
	if err := hub.Control("pc09", true); err == nil {
		t.Fatal("control of an unknown computer")
	}
	if err := hub.Control("pc01", true); err != nil {
		t.Fatal(err)
	}
	if err := hub.ClipboardSet("pc01", strings.Repeat("x", classroomview.MaxClipboardBytes+1)); !errors.Is(err, classroomview.ErrClipboardTooLarge) {
		t.Fatalf("large text: %v", err)
	}
	if err := hub.ClipboardSet("pc01", "hello\x00"); err != nil {
		t.Fatal(err)
	}
	if text, err := hub.ClipboardGet("pc01"); err != nil || text != "from pc" {
		t.Fatalf("get = %q, %v", text, err)
	}
	if err := hub.Control("pc01", false); err != nil {
		t.Fatal(err)
	}
	if err := hub.ClipboardSet("pc01", "late"); !errors.Is(err, ErrClassroomNotControlled) {
		t.Fatalf("set after control ended: %v", err)
	}
	connector.mutex.Lock()
	got := strings.Join(connector.clip, ",")
	connector.mutex.Unlock()
	if got != "begin,set:hello,end" {
		t.Fatalf("calls = %s", got)
	}
}

func TestClassroomFrameWaitsForTheNextPicture(t *testing.T) {
	hub := NewClassroomViewHub(nil, nil)
	hub.computers["pc01"] = &ClassroomComputer{Name: "pc01", Frame: 3, imageBytes: []byte("three")}
	go func() {
		time.Sleep(50 * time.Millisecond)
		hub.update("pc01", func(computer *ClassroomComputer) { computer.Frame, computer.imageBytes = 4, []byte("four") })
		hub.frameArrived()
	}()
	started := time.Now()
	image, frame, found := hub.Frame("pc01", 3)
	if !found || frame != 4 || string(image) != "four" || time.Since(started) >= classroomFrameWait {
		t.Fatalf("frame = %q %d %v after %v", image, frame, found, time.Since(started))
	}
	hub.frameWait = 30 * time.Millisecond
	if image, frame, _ := hub.Frame("pc01", 4); image != nil || frame != 4 {
		t.Fatalf("an unchanged frame was sent: %q %d", image, frame)
	}
}

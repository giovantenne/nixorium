package main

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

type shownMessage struct {
	text        string
	seconds     int
	untilUnlock bool
}

type fakeMessenger struct {
	mutex       sync.Mutex
	shown       []shownMessage
	hides       int
	unavailable bool
}

func (notice *fakeMessenger) ShowMessage(text string, seconds int, untilUnlock bool) error {
	notice.mutex.Lock()
	defer notice.mutex.Unlock()
	if notice.unavailable {
		return errMessageUnavailable
	}
	notice.shown = append(notice.shown, shownMessage{text, seconds, untilUnlock})
	return nil
}

func (notice *fakeMessenger) HideMessage() error {
	notice.mutex.Lock()
	defer notice.mutex.Unlock()
	if notice.unavailable {
		return errMessageUnavailable
	}
	notice.hides++
	return nil
}

func messageChannel(t *testing.T, agent agentContext) func(classroomview.Message) classroomview.Message {
	t.Helper()
	server, client := net.Pipe()
	go handle(server, agent)
	t.Cleanup(func() { _ = client.Close() })
	return func(message classroomview.Message) classroomview.Message {
		t.Helper()
		if err := classroomview.Write(client, message); err != nil {
			t.Fatal(err)
		}
		reply, err := classroomview.Read(client)
		if err != nil {
			t.Fatal(err)
		}
		return reply
	}
}

func TestAgentShowsAndHidesTheMessageThroughTheExtension(t *testing.T) {
	notice := &fakeMessenger{}
	exchange := messageChannel(t, agentContext{userName: "student", capture: fakeCapture{}, lock: &fakeLocker{}, notice: notice})
	if reply := exchange(classroomview.Message{Type: classroomview.TypeMessageShow, Text: "Exam in progress.\nWait for the teacher.", Seconds: 30}); reply.Type != classroomview.TypeMessageShown {
		t.Fatalf("show reply = %+v", reply)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeMessageShow, Text: "Wait", UntilUnlock: true}); reply.Type != classroomview.TypeMessageShown {
		t.Fatalf("until-unlock reply = %+v", reply)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeMessageHide}); reply.Type != classroomview.TypeMessageHidden {
		t.Fatalf("hide reply = %+v", reply)
	}
	want := []shownMessage{{"Exam in progress.\nWait for the teacher.", 30, false}, {"Wait", 0, true}}
	if len(notice.shown) != 2 || notice.shown[0] != want[0] || notice.shown[1] != want[1] || notice.hides != 1 {
		t.Fatalf("the extension was asked for %+v and %d hides", notice.shown, notice.hides)
	}
}

func TestAgentRefusesMessagesBeyondTheLimitsWithoutAskingTheExtension(t *testing.T) {
	notice := &fakeMessenger{}
	exchange := messageChannel(t, agentContext{userName: "student", capture: fakeCapture{}, lock: &fakeLocker{}, notice: notice})
	for name, message := range map[string]classroomview.Message{
		"empty":            {Type: classroomview.TypeMessageShow},
		"too long":         {Type: classroomview.TypeMessageShow, Text: strings.Repeat("x", classroomview.MaxMessageRunes+1)},
		"control":          {Type: classroomview.TypeMessageShow, Text: "red\x1b[31m"},
		"too many lines":   {Type: classroomview.TypeMessageShow, Text: strings.Repeat("x\n", classroomview.MaxMessageLines) + "x"},
		"negative time":    {Type: classroomview.TypeMessageShow, Text: "Wait", Seconds: -1},
		"two durations":    {Type: classroomview.TypeMessageShow, Text: "Wait", Seconds: 5, UntilUnlock: true},
		"beyond the range": {Type: classroomview.TypeMessageShow, Text: "Wait", Seconds: classroomview.MaxMessageSeconds + 1},
	} {
		if reply := exchange(message); reply.Type != classroomview.TypeError || reply.Code != classroomview.CodeBadRequest || reply.Detail == "" {
			t.Errorf("%s: reply = %+v", name, reply)
		}
	}
	if len(notice.shown) != 0 {
		t.Fatalf("the extension was asked for %+v", notice.shown)
	}
}

func TestAgentSaysWhenTheExtensionCannotShowMessages(t *testing.T) {
	for name, agent := range map[string]agentContext{
		"extension did not answer": {userName: "student", capture: fakeCapture{}, lock: &fakeLocker{}, notice: &fakeMessenger{unavailable: true}},
		"no extension at all":      {userName: "student", capture: fakeCapture{}, lock: &fakeLocker{}},
	} {
		exchange := messageChannel(t, agent)
		for _, message := range []classroomview.Message{
			{Type: classroomview.TypeMessageShow, Text: "Wait"},
			{Type: classroomview.TypeMessageHide},
		} {
			if reply := exchange(message); reply.Type != classroomview.TypeError || reply.Code != classroomview.CodeMessageUnavailable {
				t.Errorf("%s: %s reply = %+v", name, message.Type, reply)
			}
		}
	}
	// The lock keeps working beside it.
	lock := &fakeLocker{}
	exchange := messageChannel(t, agentContext{userName: "student", capture: fakeCapture{}, lock: lock})
	if reply := exchange(classroomview.Message{Type: classroomview.TypeLock, Locked: true}); reply.Type != classroomview.TypeLockState || !lock.locked {
		t.Fatalf("lock beside a missing message extension = %+v", reply)
	}
}

func TestAgentMessageDoesNotEndWithTheConnection(t *testing.T) {
	notice := &fakeMessenger{}
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		handle(server, agentContext{userName: "student", capture: fakeCapture{}, lock: &fakeLocker{}, notice: notice})
		close(done)
	}()
	_ = classroomview.Write(client, classroomview.Message{Type: classroomview.TypeMessageShow, Text: "Back soon"})
	if reply, err := classroomview.Read(client); err != nil || reply.Type != classroomview.TypeMessageShown {
		t.Fatalf("show reply = %+v, %v", reply, err)
	}
	_ = client.Close()
	<-done
	// Unlike the teacher's screen, the message is the session's own: closing
	// the controller's connection removes nothing.
	if notice.hides != 0 || len(notice.shown) != 1 {
		t.Fatalf("after the connection ended: %+v and %d hides", notice.shown, notice.hides)
	}
}

func TestProbeMessageRefusesAnUnknownDuration(t *testing.T) {
	if err := probeMessage(context.Background(), []string{"true"}, true, "Wait", "soon", &strings.Builder{}); err == nil {
		t.Fatal("a duration that is not a number of seconds was accepted")
	}
	if err := probeMessage(context.Background(), nil, true, "Wait", "0", &strings.Builder{}); err == nil || errors.Is(err, errMessageUnavailable) {
		t.Fatalf("a probe without a command = %v", err)
	}
}

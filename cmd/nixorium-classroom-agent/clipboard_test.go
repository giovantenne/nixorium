package main

import (
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

func (capture fakeCapture) ClipboardBegin() error { return capture.err }
func (capture fakeCapture) ClipboardEnd() error   { return nil }
func (capture fakeCapture) ClipboardRelease()     {}
func (capture fakeCapture) ClipboardSet(text string) error {
	if _, err := classroomview.ValidateClipboardText(text); err != nil {
		return err
	}
	return capture.err
}
func (capture fakeCapture) ClipboardGet() (string, error) {
	if capture.err != nil {
		return "", capture.err
	}
	return string(capture.image), nil
}

func TestClipboardRepliesUseShortCodes(t *testing.T) {
	set := func(capture fakeCapture, text string) classroomview.Message {
		return clipboardReply(capture, classroomview.Message{Type: classroomview.TypeClipboardSet, Text: text})
	}
	if reply := set(fakeCapture{}, "hello"); reply.Type != classroomview.TypeClipboardDone {
		t.Fatalf("set = %+v", reply)
	}
	if reply := set(fakeCapture{}, strings.Repeat("x", classroomview.MaxClipboardBytes+1)); reply.Code != classroomview.CodeClipboardTooLarge {
		t.Fatalf("large = %+v", reply)
	}
	if reply := set(fakeCapture{err: errLocked}, "hello"); reply.Code != classroomview.CodeLocked {
		t.Fatalf("locked = %+v", reply)
	}
	if reply := set(fakeCapture{err: errNotReady}, "hello"); reply.Code != classroomview.CodeNotReady {
		t.Fatalf("not viewed = %+v", reply)
	}
	// A raw error, which might quote the text, never reaches the teacher.
	if reply := set(fakeCapture{err: errors.New("secret text")}, "hello"); reply.Code != classroomview.CodeClipboard || reply.Detail != "" {
		t.Fatalf("failure = %+v", reply)
	}
	if reply := clipboardReply(fakeCapture{image: []byte("copied")}, classroomview.Message{Type: classroomview.TypeClipboardGet}); reply.Type != classroomview.TypeClipboardDone || reply.Text != "copied" {
		t.Fatalf("get = %+v", reply)
	}
}

func TestSelectionPipes(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = writeSelection(write, []byte("hello"), time.Second) }()
	if data, err := readSelection(read, 64, time.Second); err != nil || string(data) != "hello" {
		t.Fatalf("read %q, %v", data, err)
	}
	// More than the limit is refused, not cut.
	read, write, _ = os.Pipe()
	go func() { _ = writeSelection(write, []byte(strings.Repeat("x", 65)), time.Second) }()
	if _, err := readSelection(read, 64, time.Second); !errors.Is(err, classroomview.ErrClipboardTooLarge) {
		t.Fatalf("over the limit: %v", err)
	}
	// A writer that never finishes is given up after the timeout.
	read, write, _ = os.Pipe()
	defer write.Close()
	started := time.Now()
	if _, err := readSelection(read, 64, 100*time.Millisecond); err == nil || time.Since(started) > 2*time.Second {
		t.Fatalf("hanging writer: %v after %v", err, time.Since(started))
	}
}

func TestTextFormatsOnly(t *testing.T) {
	if pickTextMime([]string{"image/png", "text/plain", "text/plain;charset=utf-8"}) != "text/plain;charset=utf-8" {
		t.Fatal("the best text format was not chosen")
	}
	if pickTextMime([]string{"image/png", "text/html"}) != "" || isTextMime("image/png") {
		t.Fatal("a non-text format was accepted")
	}
}

type endCountingCapture struct {
	fakeCapture
	ended chan struct{}
}

func (capture endCountingCapture) ClipboardEnd() error {
	capture.ended <- struct{}{}
	return nil
}

func (capture endCountingCapture) ClipboardRelease() { capture.ended <- struct{}{} }

func TestOnlyTheConnectionThatBeganTheClipboardEndsIt(t *testing.T) {
	capture := endCountingCapture{ended: make(chan struct{}, 4)}
	agent := agentContext{userName: "student", capture: capture, lock: &fakeLocker{}}
	open := func() (net.Conn, <-chan struct{}) {
		server, client := net.Pipe()
		finished := make(chan struct{})
		go func() {
			handle(server, agent)
			close(finished)
		}()
		return client, finished
	}
	exchange := func(client net.Conn, message classroomview.Message) classroomview.Message {
		t.Helper()
		_ = client.SetDeadline(time.Now().Add(2 * time.Second))
		if err := classroomview.Write(client, message); err != nil {
			t.Fatal(err)
		}
		reply, err := classroomview.Read(client)
		if err != nil {
			t.Fatal(err)
		}
		return reply
	}
	control, controlDone := open()
	if reply := exchange(control, classroomview.Message{Type: classroomview.TypeClipboardBegin}); reply.Type != classroomview.TypeClipboardDone {
		t.Fatalf("begin = %+v", reply)
	}
	// Another channel does its work and closes: the control goes on.
	other, otherDone := open()
	if reply := exchange(other, classroomview.Message{Type: classroomview.TypeLock, Locked: true}); reply.Type != classroomview.TypeLockState {
		t.Fatalf("lock = %+v", reply)
	}
	other.Close()
	<-otherDone
	select {
	case <-capture.ended:
		t.Fatal("another channel closing ended the shared clipboard")
	case <-time.After(100 * time.Millisecond):
	}
	// The control's own channel closing ends it.
	control.Close()
	<-controlDone
	select {
	case <-capture.ended:
	case <-time.After(2 * time.Second):
		t.Fatal("the control closing left the shared clipboard on")
	}
}

func TestASecondControlKeepsTheClipboardWhenTheFirstCloses(t *testing.T) {
	capture := &mutterCapture{running: true}
	capture.clipboard = clipboardState{on: true, holders: 1}
	// A reconnect begins the clipboard again while the old channel drains.
	capture.clipboard.holders++
	capture.ClipboardRelease()
	if !capture.clipboard.on || capture.clipboard.holders != 1 {
		t.Fatalf("the old channel closing ended the clipboard: %+v", capture.clipboard)
	}
}

package main

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

func (capture fakeCapture) ClipboardBegin() error { return capture.err }
func (capture fakeCapture) ClipboardEnd() error   { return nil }
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

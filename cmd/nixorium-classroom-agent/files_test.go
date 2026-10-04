package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

func studentHome(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "user-dirs.dirs"), []byte("XDG_DESKTOP_DIR=\"$HOME/Scrivania\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return home, filepath.Join(home, "Scrivania")
}

var lessonEntries = []classroomview.FileEntry{
	{Path: "Lesson", Dir: true},
	{Path: "Lesson/notes.txt", Size: 11},
	{Path: "Lesson/empty.txt"},
	{Path: "run.sh", Size: 4},
}

func sendLesson(t *testing.T, home string) []string {
	t.Helper()
	incoming, err := newReceiver(home, lessonEntries)
	if err != nil {
		t.Fatal(err)
	}
	for _, piece := range []struct {
		index int
		data  string
	}{{1, "hello "}, {1, "world"}, {3, "ls\n\n"}} {
		if err := incoming.chunk(piece.index, []byte(piece.data)); err != nil {
			t.Fatal(err)
		}
	}
	placed, err := incoming.finish()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(incoming.staging); !os.IsNotExist(err) {
		t.Fatal("the staging folder was left behind")
	}
	return placed
}

func TestReceiverPlacesFilesOnTheDesktop(t *testing.T) {
	home, desktop := studentHome(t)
	if placed := sendLesson(t, home); strings.Join(placed, ",") != "Lesson,run.sh" {
		t.Fatalf("placed = %v", placed)
	}
	content, err := os.ReadFile(filepath.Join(desktop, "Lesson", "notes.txt"))
	if err != nil || string(content) != "hello world" {
		t.Fatalf("notes = %q, %v", content, err)
	}
	for name, mode := range map[string]os.FileMode{"Lesson": 0o755 | os.ModeDir, "Lesson/notes.txt": 0o644, "Lesson/empty.txt": 0o644, "run.sh": 0o644} {
		info, err := os.Lstat(filepath.Join(desktop, name))
		if err != nil || info.Mode() != mode {
			t.Fatalf("%s mode = %v, %v", name, info.Mode(), err)
		}
	}
	// Nothing on the desktop is replaced: a second sending gets new names.
	if placed := sendLesson(t, home); strings.Join(placed, ",") != "Lesson (2),run (2).sh" {
		t.Fatalf("second placed = %v", placed)
	}
	if content, _ := os.ReadFile(filepath.Join(desktop, "run.sh")); string(content) != "ls\n\n" {
		t.Fatalf("the first file changed: %q", content)
	}
}

func TestReceiverRefusesBrokenSendings(t *testing.T) {
	home, desktop := studentHome(t)
	for name, step := range map[string]func(*receiver) error{
		"out of order": func(incoming *receiver) error { return incoming.chunk(3, []byte("ls")) },
		"too large":    func(incoming *receiver) error { return incoming.chunk(1, []byte("hello world!")) },
		"a folder":     func(incoming *receiver) error { return incoming.chunk(0, []byte("x")) },
		"incomplete": func(incoming *receiver) error {
			if err := incoming.chunk(1, []byte("hello")); err != nil {
				return nil
			}
			_, err := incoming.finish()
			return err
		},
	} {
		incoming, err := newReceiver(home, lessonEntries)
		if err != nil {
			t.Fatal(err)
		}
		if err := step(incoming); err == nil {
			t.Fatalf("%s was accepted", name)
		}
		incoming.abort()
		if _, err := os.Stat(incoming.staging); !os.IsNotExist(err) {
			t.Fatalf("%s left the staging folder", name)
		}
	}
	if _, err := newReceiver(home, []classroomview.FileEntry{{Path: "../escape", Size: 1}}); err == nil {
		t.Fatal("a path outside the sending was accepted")
	}
	if entries, _ := os.ReadDir(desktop); len(entries) != 0 {
		t.Fatalf("refused sendings reached the desktop: %v", entries)
	}
}

func TestDesktopDirectoryStaysInTheHome(t *testing.T) {
	home := t.TempDir()
	if got, _ := desktopDirectory(home); got != filepath.Join(home, "Desktop") {
		t.Fatalf("default = %s", got)
	}
	_ = os.MkdirAll(filepath.Join(home, ".config"), 0o700)
	for _, line := range []string{`XDG_DESKTOP_DIR="/etc"`, `XDG_DESKTOP_DIR="$HOME/../x"`, `XDG_DESKTOP_DIR="$HOME/"`} {
		_ = os.WriteFile(filepath.Join(home, ".config", "user-dirs.dirs"), []byte(line+"\n"), 0o600)
		if got, _ := desktopDirectory(home); got != filepath.Join(home, "Desktop") {
			t.Fatalf("%s gave %s", line, got)
		}
	}
}

func TestAgentReceivesFilesOverTheProtocol(t *testing.T) {
	home, desktop := studentHome(t)
	server, client := net.Pipe()
	go handle(server, agentContext{userName: "student", home: home, capture: fakeCapture{}, lock: &fakeLocker{}})
	exchange := func(message classroomview.Message) classroomview.Message {
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
	if reply := exchange(classroomview.Message{Type: classroomview.TypeFilesChunk, Index: 0, Data: []byte("x")}); reply.Code != classroomview.CodeFilesRefused {
		t.Fatalf("chunk without a sending = %+v", reply)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeFilesBegin, Entries: []classroomview.FileEntry{{Path: "a.txt", Size: 2}}}); reply.Type != classroomview.TypeFilesReady {
		t.Fatalf("begin = %+v", reply)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeFilesChunk, Index: 0, Data: []byte("hi")}); reply.Type != classroomview.TypeFilesAck {
		t.Fatalf("chunk = %+v", reply)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeFilesEnd}); reply.Type != classroomview.TypeFilesDone || reply.Detail != "a.txt" {
		t.Fatalf("end = %+v", reply)
	}
	if content, _ := os.ReadFile(filepath.Join(desktop, "a.txt")); string(content) != "hi" {
		t.Fatalf("a.txt = %q", content)
	}
	// A connection that ends mid-sending leaves nothing behind.
	if reply := exchange(classroomview.Message{Type: classroomview.TypeFilesBegin, Entries: []classroomview.FileEntry{{Path: "b.txt", Size: 2}}}); reply.Type != classroomview.TypeFilesReady {
		t.Fatalf("second begin = %+v", reply)
	}
	_ = client.Close()
	staging := filepath.Join(home, ".local", "share", "nixorium-classroom")
	for attempt := 0; attempt < 100; attempt++ {
		if entries, _ := os.ReadDir(staging); len(entries) == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("an interrupted sending left its staging folder")
}

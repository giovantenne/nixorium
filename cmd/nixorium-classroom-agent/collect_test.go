package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

// collectChannel serves the agent on one end of a pipe and exchanges
// messages through the other.
func collectChannel(t *testing.T, home string) func(classroomview.Message) classroomview.Message {
	t.Helper()
	server, client := net.Pipe()
	go handle(server, agentContext{userName: "student", home: home, capture: fakeCapture{}, lock: &fakeLocker{}})
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

func TestAgentCollectsTheDesktopWithoutChangingIt(t *testing.T) {
	home, desktop := studentHome(t)
	large := bytes.Repeat([]byte("0123456789abcdef"), (classroomview.MaxChunkBytes+classroomview.MaxChunkBytes/2)/16)
	for name, content := range map[string][]byte{"Homework/essay.odt": []byte("essay"), "large.bin": large, "empty.txt": nil, ".hidden": []byte("x")} {
		target := filepath.Join(desktop, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(desktop, "link")); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(filepath.Join(desktop, "large.bin"))
	exchange := collectChannel(t, home)
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: 0}); reply.Code != classroomview.CodeCollectRefused {
		t.Fatalf("a piece without a list = %+v", reply)
	}
	listed := exchange(classroomview.Message{Type: classroomview.TypeCollectList})
	want := []classroomview.FileEntry{{Path: "Homework", Dir: true}, {Path: "Homework/essay.odt", Size: 5}, {Path: "empty.txt"}, {Path: "large.bin", Size: int64(len(large))}}
	if listed.Type != classroomview.TypeCollectListed || fmt.Sprint(listed.Entries) != fmt.Sprint(want) {
		t.Fatalf("listed = %+v", listed)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: 1}); reply.Type != classroomview.TypeCollectData || string(reply.Data) != "essay" || reply.Index != 1 {
		t.Fatalf("essay = %+v", reply)
	}
	received := []byte{}
	for len(received) < len(large) {
		reply := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: 3})
		if reply.Type != classroomview.TypeCollectData || len(reply.Data) > classroomview.MaxChunkBytes {
			t.Fatalf("large piece = %s %s %d", reply.Type, reply.Code, len(reply.Data))
		}
		received = append(received, reply.Data...)
	}
	if !bytes.Equal(received, large) {
		t.Fatal("the large file arrived changed")
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectEnd}); reply.Type != classroomview.TypeCollectDone {
		t.Fatalf("end = %+v", reply)
	}
	after, err := os.Stat(filepath.Join(desktop, "large.bin"))
	if err != nil || !after.ModTime().Equal(before.ModTime()) || after.Mode() != before.Mode() {
		t.Fatalf("the desktop changed: %v %v", after, err)
	}
	if entries, _ := os.ReadDir(desktop); len(entries) != 5 {
		t.Fatalf("desktop entries = %d", len(entries))
	}
}

func TestAgentCollectRefusesChangedFilesAndForeignRequests(t *testing.T) {
	home, desktop := studentHome(t)
	if err := os.MkdirAll(filepath.Join(desktop, "Folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt", "Folder/c.txt"} {
		if err := os.WriteFile(filepath.Join(desktop, filepath.FromSlash(name)), []byte("abc"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	exchange := collectChannel(t, home)
	list := func() []classroomview.FileEntry {
		t.Helper()
		reply := exchange(classroomview.Message{Type: classroomview.TypeCollectList})
		if reply.Type != classroomview.TypeCollectListed || len(reply.Entries) < 3 {
			t.Fatalf("listed = %+v", reply)
		}
		return reply.Entries
	}
	for name, index := range map[string]int{"a folder": 0, "negative": -1, "past the end": 4} {
		list()
		if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: index}); reply.Code != classroomview.CodeCollectRefused {
			t.Fatalf("%s = %+v", name, reply)
		}
	}
	// Each file is handed over once and in list order.
	list()
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: 3}); reply.Type != classroomview.TypeCollectData {
		t.Fatalf("b.txt = %+v", reply)
	}
	for name, index := range map[string]int{"again": 3, "earlier": 2} {
		if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: index}); reply.Code != classroomview.CodeCollectRefused {
			t.Fatalf("%s = %+v", name, reply)
		}
		list()
		exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: 3})
	}
	// A file that grew, or became a link, after the list is refused.
	list()
	if err := os.WriteFile(filepath.Join(desktop, "a.txt"), []byte("abcd"), 0o600); err != nil {
		t.Fatal(err)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: 2}); reply.Code != classroomview.CodeCollectRefused {
		t.Fatalf("grown file = %+v", reply)
	}
	list()
	if err := os.Remove(filepath.Join(desktop, "b.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(desktop, "b.txt")); err != nil {
		t.Fatal(err)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: 3}); reply.Code != classroomview.CodeCollectRefused {
		t.Fatalf("linked file = %+v", reply)
	}
	// A folder replaced by a link is not followed either.
	list()
	if err := os.Rename(filepath.Join(desktop, "Folder"), filepath.Join(home, "Moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(home, "Moved"), filepath.Join(desktop, "Folder")); err != nil {
		t.Fatal(err)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: 1}); reply.Code != classroomview.CodeCollectRefused {
		t.Fatalf("linked folder = %+v", reply)
	}
}

func TestAgentCollectReportsTooMuchAndEmptyDesktops(t *testing.T) {
	home, desktop := studentHome(t)
	exchange := collectChannel(t, home)
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectList}); reply.Type != classroomview.TypeCollectListed || len(reply.Entries) != 0 {
		t.Fatalf("missing desktop = %+v", reply)
	}
	if err := os.MkdirAll(desktop, 0o755); err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= classroomview.MaxFileEntries; index++ {
		if err := os.WriteFile(filepath.Join(desktop, fmt.Sprintf("f%04d", index)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectList}); reply.Code != classroomview.CodeCollectTooMuch {
		t.Fatalf("too many = %s %s", reply.Type, reply.Code)
	}
	// The connection still answers after a refusal.
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectEnd}); reply.Type != classroomview.TypeCollectDone {
		t.Fatalf("end = %+v", reply)
	}
}

func TestCollectionEndsWithTheConnection(t *testing.T) {
	home, desktop := studentHome(t)
	if err := os.MkdirAll(desktop, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(desktop, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		handle(server, agentContext{userName: "student", home: home, capture: fakeCapture{}, lock: &fakeLocker{}})
		close(done)
	}()
	if err := classroomview.Write(client, classroomview.Message{Type: classroomview.TypeCollectList}); err != nil {
		t.Fatal(err)
	}
	if _, err := classroomview.Read(client); err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the connection did not end")
	}
}

func TestCollectShowsNoNoticeByItself(t *testing.T) {
	home, desktop := studentHome(t)
	if err := os.MkdirAll(desktop, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(desktop, "a.txt"), []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	notice := &fakeMessenger{}
	server, client := net.Pipe()
	go handle(server, agentContext{userName: "student", home: home, capture: fakeCapture{}, lock: &fakeLocker{}, notice: notice})
	t.Cleanup(func() { _ = client.Close() })
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
	// The controller asks for the notice once a collection succeeded; a
	// review (list, end) and a collection (list, piece, end) show nothing.
	exchange(classroomview.Message{Type: classroomview.TypeCollectList})
	exchange(classroomview.Message{Type: classroomview.TypeCollectEnd})
	exchange(classroomview.Message{Type: classroomview.TypeCollectList})
	if reply := exchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: 0}); reply.Type != classroomview.TypeCollectData {
		t.Fatalf("piece = %+v", reply)
	}
	exchange(classroomview.Message{Type: classroomview.TypeCollectEnd})
	notice.mutex.Lock()
	defer notice.mutex.Unlock()
	if len(notice.notices) != 0 {
		t.Fatalf("notices = %v", notice.notices)
	}
}

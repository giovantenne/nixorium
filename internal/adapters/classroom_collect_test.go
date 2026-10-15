package adapters

import (
	"errors"
	"io"
	"os/exec"
	"testing"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

// pipedAgentSession is a session whose agent answers each request with the
// next reply of a script.
func pipedAgentSession(t *testing.T, replies []classroomview.Message) (*sshAgentSession, chan classroomview.Message) {
	t.Helper()
	requestReader, requestWriter := io.Pipe()
	replyReader, replyWriter := io.Pipe()
	requests := make(chan classroomview.Message, len(replies)+1)
	go func() {
		for _, reply := range replies {
			request, err := classroomview.Read(requestReader)
			if err != nil {
				return
			}
			requests <- request
			if err := classroomview.Write(replyWriter, reply); err != nil {
				return
			}
		}
	}()
	session := &sshAgentSession{command: &exec.Cmd{}, input: requestWriter, output: replyReader}
	t.Cleanup(func() { _ = session.Close(); _ = replyWriter.Close() })
	return session, requests
}

func TestCollectSessionListsAndReadsInPieces(t *testing.T) {
	session, requests := pipedAgentSession(t, []classroomview.Message{
		{Type: classroomview.TypeCollectListed, Entries: []classroomview.FileEntry{{Path: "a.txt", Size: 5}}, Skipped: 1},
		{Type: classroomview.TypeCollectData, Index: 0, Data: []byte("hel")},
		{Type: classroomview.TypeCollectData, Index: 0, Data: []byte("lo")},
		{Type: classroomview.TypeCollectDone},
	})
	entries, skipped, err := session.ListDesktop()
	if err != nil || len(entries) != 1 || skipped != 1 || (<-requests).Type != classroomview.TypeCollectList {
		t.Fatalf("list = %+v %d %v", entries, skipped, err)
	}
	received := ""
	if err := session.ReadDesktopFile(0, 5, func(data []byte) error { received += string(data); return nil }); err != nil || received != "hello" {
		t.Fatalf("read = %q, %v", received, err)
	}
	for range 2 {
		if request := <-requests; request.Type != classroomview.TypeCollectChunk || request.Index != 0 {
			t.Fatalf("request = %+v", request)
		}
	}
	if err := session.EndCollect(); err != nil {
		t.Fatal(err)
	}
}

func TestCollectSessionRefusesForeignPieces(t *testing.T) {
	for name, reply := range map[string]classroomview.Message{
		"another file":    {Type: classroomview.TypeCollectData, Index: 1, Data: []byte("abc")},
		"too long":        {Type: classroomview.TypeCollectData, Index: 0, Data: []byte("abcdef")},
		"empty":           {Type: classroomview.TypeCollectData, Index: 0},
		"agent refusal":   {Type: classroomview.TypeError, Code: classroomview.CodeCollectRefused},
		"unexpected type": {Type: classroomview.TypeThumbnail, Image: []byte("x")},
	} {
		session, _ := pipedAgentSession(t, []classroomview.Message{reply})
		if err := session.ReadDesktopFile(0, 5, func([]byte) error { return nil }); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	session, _ := pipedAgentSession(t, []classroomview.Message{{Type: classroomview.TypeError, Code: classroomview.CodeUnsupported}})
	var agentError classroomview.AgentError
	if _, _, err := session.ListDesktop(); !errors.As(err, &agentError) || agentError.Code != classroomview.CodeUnsupported {
		t.Fatalf("an older agent = %v", err)
	}
}

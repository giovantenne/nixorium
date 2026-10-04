package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// TestClassroomAgentHelper plays the client side of "ssh … nixorium-classroom-connect".
func TestClassroomAgentHelper(t *testing.T) {
	mode := os.Getenv("NIXORIUM_TEST_CLASSROOM_AGENT")
	if mode == "" {
		return
	}
	if mode == "no-agent" {
		_ = classroomview.Write(os.Stdout, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNoAgent})
		os.Exit(0)
	}
	received, chunks := 0, 0
	for {
		request, err := classroomview.Read(os.Stdin)
		if err != nil {
			os.Exit(0)
		}
		switch request.Type {
		case classroomview.TypeLock:
			_ = classroomview.Write(os.Stdout, classroomview.Message{Type: classroomview.TypeLockState, Locked: request.Locked})
		case classroomview.TypeFilesBegin:
			_ = classroomview.Write(os.Stdout, classroomview.Message{Type: classroomview.TypeFilesReady})
		case classroomview.TypeFilesChunk:
			received += len(request.Data)
			chunks++
			_ = classroomview.Write(os.Stdout, classroomview.Message{Type: classroomview.TypeFilesAck, Index: request.Index})
		case classroomview.TypeFilesEnd:
			_ = classroomview.Write(os.Stdout, classroomview.Message{Type: classroomview.TypeFilesDone, Detail: fmt.Sprintf("bytes %d\npieces %d", received, chunks)})
		case classroomview.TypeHello:
			_ = classroomview.Write(os.Stdout, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion, User: "student"})
		case classroomview.TypeThumbnailRequest:
			_ = classroomview.Write(os.Stdout, classroomview.Message{Type: classroomview.TypeThumbnail, Width: request.Width, Height: 200, Image: []byte("jpeg"), Frame: 7})
		case classroomview.TypeInput:
			_ = classroomview.Write(os.Stdout, classroomview.Message{Type: classroomview.TypeInputDone})
		}
	}
}

func fakeSSH(t *testing.T, mode string) {
	t.Helper()
	directory := t.TempDir()
	log := filepath.Join(directory, "arguments")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + log + "\nexec " + os.Args[0] + " -test.run=TestClassroomAgentHelper\n"
	if err := os.WriteFile(filepath.Join(directory, "ssh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("NIXORIUM_TEST_CLASSROOM_AGENT", mode)
	t.Cleanup(func() {
		arguments, _ := os.ReadFile(log)
		for _, expected := range []string{"BatchMode=yes", "ClearAllForwardings=yes", "ForwardAgent=no", "root@192.0.2.1", "nixorium-classroom-connect"} {
			if len(arguments) > 0 && !strings.Contains(string(arguments), expected) {
				t.Errorf("ssh arguments lack %q:\n%s", expected, arguments)
			}
		}
	})
}

func TestClassroomAgentConnectorExchangesOverSSH(t *testing.T) {
	fakeSSH(t, "agent")
	session, err := ClassroomAgentConnector{}.Connect(context.Background(), domain.HostMeta{Name: "pc01", IP: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	reply, err := session.Thumbnail(320, 0)
	if err != nil || reply.Type != classroomview.TypeThumbnail || string(reply.Image) != "jpeg" || reply.Width != 320 || reply.Frame != 7 {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	if err := session.Input([]classroomview.InputEvent{{Kind: classroomview.InputKey, Keysym: 'a', Pressed: true}}, true); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Thumbnail(320, 0); err == nil {
		t.Fatal("a closed session answered")
	}
}

func TestClassroomAgentConnectorReportsAMissingAgent(t *testing.T) {
	fakeSSH(t, "no-agent")
	_, err := ClassroomAgentConnector{}.Connect(context.Background(), domain.HostMeta{Name: "pc01", IP: "192.0.2.1"})
	var agentError classroomview.AgentError
	if !errors.As(err, &agentError) || agentError.Code != classroomview.CodeNoAgent {
		t.Fatalf("err = %v", err)
	}
}

func TestClassroomAgentConnectorLocksAndSendsFiles(t *testing.T) {
	fakeSSH(t, "agent")
	session, err := ClassroomAgentConnector{}.Connect(context.Background(), domain.HostMeta{Name: "pc01", IP: "192.0.2.1"})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if locked, err := session.SetLocked(true); err != nil || !locked {
		t.Fatalf("lock = %t, %v", locked, err)
	}
	size := int64(classroomview.MaxChunkBytes*2 + classroomview.MaxChunkBytes/2)
	entries := []classroomview.FileEntry{{Path: "Lesson", Dir: true}, {Path: "Lesson/big.bin", Size: size}, {Path: "empty.txt"}}
	placed, err := session.SendFiles(entries, func(index int) (io.ReadCloser, error) {
		if index != 1 {
			t.Errorf("opened entry %d", index)
		}
		return io.NopCloser(strings.NewReader(strings.Repeat("x", int(size)))), nil
	})
	if err != nil || strings.Join(placed, ",") != fmt.Sprintf("bytes %d,pieces 3", size) {
		t.Fatalf("placed = %v, %v", placed, err)
	}
	// A file shorter than announced is an error, not a silent truncation.
	_, err = session.SendFiles(entries, func(int) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("short")), nil
	})
	if err == nil {
		t.Fatal("a short file was sent")
	}
}

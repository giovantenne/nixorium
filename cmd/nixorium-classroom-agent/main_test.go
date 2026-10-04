package main

import (
	"bytes"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

func fakeLoginctl(values map[string]string) commandRunner {
	return func(arguments ...string) (string, error) {
		key := strings.Join(arguments, " ")
		if value, found := values[key]; found {
			return value, nil
		}
		return "", errors.New("unexpected loginctl " + key)
	}
}

func graphicalSession(uid int) commandRunner {
	return fakeLoginctl(map[string]string{
		"show-seat seat0 --property=ActiveSession --value": "2",
		"show-session 2 --property=Type --value":           "wayland",
		"show-session 2 --property=User --value":           strconv.Itoa(uid),
	})
}

// startAgent serves in a temporary runtime directory laid out like /run/user.
func startAgent(t *testing.T) (string, int) {
	t.Helper()
	base := t.TempDir()
	uid := os.Getuid()
	runtime := filepath.Join(base, strconv.Itoa(uid))
	if err := os.Mkdir(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	go func() { _ = serve(runtime) }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Lstat(filepath.Join(runtime, SocketName)); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("agent socket did not appear")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return base, uid
}

func TestConnectRelaysHelloToTheSessionAgent(t *testing.T) {
	base, uid := startAgent(t)
	info, err := os.Lstat(filepath.Join(base, strconv.Itoa(uid), SocketName))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v, %v", info.Mode(), err)
	}
	request, output := &bytes.Buffer{}, &bytes.Buffer{}
	if err := classroomview.Write(request, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	if err := connect(request, output, graphicalSession(uid), base); err != nil {
		t.Fatal(err)
	}
	reply, err := classroomview.Read(output)
	if err != nil || reply.Type != classroomview.TypeHello || reply.Version != classroomview.ProtocolVersion || reply.User == "" {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
}

func TestConnectReportsMissingSessionOrAgent(t *testing.T) {
	base := t.TempDir()
	for name, run := range map[string]commandRunner{
		"no session":   fakeLoginctl(map[string]string{"show-seat seat0 --property=ActiveSession --value": ""}),
		"text session": fakeLoginctl(map[string]string{"show-seat seat0 --property=ActiveSession --value": "3", "show-session 3 --property=Type --value": "tty"}),
		"no agent":     graphicalSession(os.Getuid()),
	} {
		output := &bytes.Buffer{}
		if err := connect(strings.NewReader(""), output, run, base); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		reply, err := classroomview.Read(output)
		if err != nil || reply.Type != classroomview.TypeError || (reply.Code != classroomview.CodeNoSession && reply.Code != classroomview.CodeNoAgent) {
			t.Fatalf("%s: reply = %+v, %v", name, reply, err)
		}
	}
}

func TestAgentSocketRefusesLinksAndOtherFiles(t *testing.T) {
	base := t.TempDir()
	uid := os.Getuid()
	runtime := filepath.Join(base, strconv.Itoa(uid))
	if err := os.Mkdir(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(base, "elsewhere.sock")
	listener, err := net.Listen("unix", target)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.Symlink(target, filepath.Join(runtime, SocketName)); err != nil {
		t.Fatal(err)
	}
	if _, err := agentSocket(base, uid); err == nil {
		t.Fatal("a symbolic link was accepted as the agent socket")
	}
	if err := os.Remove(filepath.Join(runtime, SocketName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime, SocketName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := agentSocket(base, uid); err == nil {
		t.Fatal("a regular file was accepted as the agent socket")
	}
	if err := serve(runtime); err == nil {
		t.Fatal("serve replaced a file that is not its socket")
	}
}

func TestAgentRefusesOtherProtocolVersionsAndUnknownMessages(t *testing.T) {
	server, client := net.Pipe()
	go handle(server, "student", fakeCapture{})
	_ = classroomview.Write(client, classroomview.Message{Type: "screen.record"})
	if reply, err := classroomview.Read(client); err != nil || reply.Code != classroomview.CodeUnsupported {
		t.Fatalf("unknown message reply = %+v, %v", reply, err)
	}
	_ = classroomview.Write(client, classroomview.Message{Type: classroomview.TypeHello, Version: 99})
	if reply, err := classroomview.Read(client); err != nil || reply.Code != classroomview.CodeUnsupported {
		t.Fatalf("version reply = %+v, %v", reply, err)
	}
	if _, err := classroomview.Read(client); !errors.Is(err, io.EOF) && err == nil {
		t.Fatal("agent kept an incompatible connection open")
	}
}

type fakeCapture struct {
	image []byte
	err   error
}

func (capture fakeCapture) Thumbnail(width int, since int64) ([]byte, int, time.Time, int64, error) {
	if capture.err != nil {
		return nil, 0, time.Time{}, 0, capture.err
	}
	if since == 5 {
		return nil, width * 10 / 16, time.UnixMilli(1700000000000), 5, nil
	}
	return capture.image, width * 10 / 16, time.UnixMilli(1700000000000), 5, nil
}

func (capture fakeCapture) Input(events []classroomview.InputEvent, release bool) error {
	return capture.err
}

func TestThumbnailReplies(t *testing.T) {
	reply := thumbnailReply(fakeCapture{image: []byte{0xff, 0xd8, 0xff, 0xd9}}, 320, 0)
	if reply.Type != classroomview.TypeThumbnail || reply.Width != 320 || reply.Height != 200 || len(reply.Image) != 4 || reply.CapturedAt != 1700000000000 || reply.Frame != 5 {
		t.Fatalf("reply = %+v", reply)
	}
	if unchanged := thumbnailReply(fakeCapture{image: []byte{1}}, 320, 5); unchanged.Type != classroomview.TypeThumbnail || unchanged.Image != nil || unchanged.Frame != 5 {
		t.Fatalf("unchanged reply = %+v", unchanged)
	}
	if done := inputReply(fakeCapture{}, classroomview.Message{Type: classroomview.TypeInput, Events: []classroomview.InputEvent{{Kind: classroomview.InputPointerMove, X: 0.5, Y: 0.5}}}); done.Type != classroomview.TypeInputDone {
		t.Fatalf("input reply = %+v", done)
	}
	for _, event := range []classroomview.InputEvent{{Kind: classroomview.InputPointerMove, X: 2}, {Kind: classroomview.InputButton, Button: "fourth"}, {Kind: classroomview.InputScroll}, {Kind: classroomview.InputKey}, {Kind: "shell"}} {
		if refused := inputReply(fakeCapture{}, classroomview.Message{Type: classroomview.TypeInput, Events: []classroomview.InputEvent{event}}); refused.Code != classroomview.CodeBadRequest {
			t.Fatalf("event %+v reply = %+v", event, refused)
		}
	}
	if notViewed := inputReply(fakeCapture{err: errNotReady}, classroomview.Message{Type: classroomview.TypeInput}); notViewed.Code != classroomview.CodeNotReady {
		t.Fatalf("not viewed reply = %+v", notViewed)
	}
	for width, code := range map[int]string{8: classroomview.CodeBadRequest, 4000: classroomview.CodeBadRequest} {
		if reply := thumbnailReply(fakeCapture{}, width, 0); reply.Code != code {
			t.Fatalf("width %d reply = %+v", width, reply)
		}
	}
	if reply := thumbnailReply(fakeCapture{err: errNotReady}, 320, 0); reply.Code != classroomview.CodeNotReady {
		t.Fatalf("not ready reply = %+v", reply)
	}
	if reply := thumbnailReply(fakeCapture{err: errLocked}, 320, 0); reply.Code != classroomview.CodeLocked {
		t.Fatalf("locked reply = %+v", reply)
	}
	if reply := thumbnailReply(fakeCapture{err: errors.New("no Mutter")}, 320, 0); reply.Code != classroomview.CodeCapture {
		t.Fatalf("failure reply = %+v", reply)
	}
}

func TestReadFramesSplitsMultipartJPEGs(t *testing.T) {
	stream := "--nixoriumframe\r\nContent-Type: image/jpeg\r\nContent-Length: 3\r\n\r\nabc\r\n" +
		"--nixoriumframe\r\nContent-Type: image/jpeg\r\nContent-Length: 2\r\n\r\nde\r\n"
	frames := []string{}
	err := readFrames(strings.NewReader(stream), func(frame []byte) { frames = append(frames, string(frame)) })
	if !errors.Is(err, io.EOF) || len(frames) != 2 || frames[0] != "abc" || frames[1] != "de" {
		t.Fatalf("frames = %q, %v", frames, err)
	}
	bad := "--nixoriumframe\r\nContent-Length: 99999999\r\n\r\n"
	if err := readFrames(strings.NewReader(bad), func([]byte) {}); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("oversized frame accepted: %v", err)
	}
}

func TestScaledHeightKeepsAspectAndIsEven(t *testing.T) {
	for _, item := range []struct{ width, screenWidth, screenHeight, want int }{{320, 1280, 800, 200}, {320, 1920, 1080, 180}, {321, 1366, 768, 180}, {100, 1000, 9, 2}} {
		if got := scaledHeight(item.width, item.screenWidth, item.screenHeight); got != item.want {
			t.Fatalf("scaledHeight(%v) = %d", item, got)
		}
	}
}

func TestAgentAnswersWhileAnotherConnectionHangs(t *testing.T) {
	listener, err := net.Listen("unix", filepath.Join(t.TempDir(), SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() { _ = serveConnections(listener, "student", fakeCapture{}) }()
	hello := func() (classroomview.Message, error) {
		connection, err := net.Dial("unix", listener.Addr().String())
		if err != nil {
			return classroomview.Message{}, err
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
		if err := classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}); err != nil {
			return classroomview.Message{}, err
		}
		return classroomview.Read(connection)
	}
	// Silent connections, as left by a controller that vanished.
	for range maxConnections - 1 {
		silent, err := net.Dial("unix", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer silent.Close()
	}
	if reply, err := hello(); err != nil || reply.Type != classroomview.TypeHello {
		t.Fatalf("hello beside silent connections = %+v, %v", reply, err)
	}
	last, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer last.Close()
	// Every slot is taken now: a further connection is refused at once.
	deadline := time.Now().Add(2 * time.Second)
	for {
		reply, err := hello()
		if err == nil && reply.Code == classroomview.CodeBusy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("over-limit reply = %+v, %v", reply, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

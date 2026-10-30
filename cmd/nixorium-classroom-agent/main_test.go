package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/godbus/dbus/v5"
)

func fakeLoginctl(values map[string]string) commandRunner {
	return func(_ context.Context, arguments ...string) (string, error) {
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
		"show-session 2 --property=Class --value":          "user",
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
	if err := connect(context.Background(), request, output, graphicalSession(uid), base); err != nil {
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
		// The login screen is a graphical session of the display manager's user.
		"login screen": fakeLoginctl(map[string]string{
			"show-seat seat0 --property=ActiveSession --value": "c1",
			"show-session c1 --property=Type --value":          "wayland",
			"show-session c1 --property=Class --value":         "greeter",
			"show-session c1 --property=User --value":          "60578",
		}),
		"no agent": graphicalSession(os.Getuid()),
	} {
		output := &bytes.Buffer{}
		if err := connect(context.Background(), strings.NewReader(""), output, run, base); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		reply, err := classroomview.Read(output)
		want := classroomview.CodeNoSession
		if name == "no agent" {
			want = classroomview.CodeNoAgent
		}
		if err != nil || reply.Type != classroomview.TypeError || reply.Code != want {
			t.Fatalf("%s: reply = %+v, %v; want code %s", name, reply, err, want)
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
	if _, release, err := agentSocket(base, uid); err == nil {
		release()
		t.Fatal("a symbolic link was accepted as the agent socket")
	}
	if err := os.Remove(filepath.Join(runtime, SocketName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtime, SocketName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, release, err := agentSocket(base, uid); err == nil {
		release()
		t.Fatal("a regular file was accepted as the agent socket")
	}
	if err := serve(runtime); err == nil {
		t.Fatal("serve replaced a file that is not its socket")
	}
}

func TestAgentSocketRefusesALinkedRuntimeDirectory(t *testing.T) {
	base := t.TempDir()
	uid := os.Getuid()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, SocketName), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(base, strconv.Itoa(uid))); err != nil {
		t.Fatal(err)
	}
	if _, release, err := agentSocket(base, uid); err == nil {
		release()
		t.Fatal("a symbolic link was accepted as the runtime directory")
	}
	// A session user other than the directory's owner is refused too.
	other := t.TempDir()
	if err := os.Mkdir(filepath.Join(other, strconv.Itoa(uid+1)), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, release, err := agentSocket(other, uid+1); err == nil {
		release()
		t.Fatal("a runtime directory of another owner was accepted")
	}
}

func TestAgentSocketPinsTheCheckedSocket(t *testing.T) {
	// Unix socket paths are short: a name from t.TempDir would not fit.
	base, err := os.MkdirTemp("", "pin")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(base)
	uid := os.Getuid()
	runtime := filepath.Join(base, strconv.Itoa(uid))
	if err := os.Mkdir(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(runtime, SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	other, err := net.Listen("unix", filepath.Join(base, "other.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	path, release, err := agentSocket(base, uid)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	// Swapping the name for a link after the check changes nothing.
	if err := os.Remove(filepath.Join(runtime, SocketName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "other.sock"), filepath.Join(runtime, SocketName)); err != nil {
		t.Fatal(err)
	}
	accepted := make(chan string, 2)
	for name, socket := range map[string]net.Listener{"agent": listener, "other": other} {
		go func() {
			if connection, err := socket.Accept(); err == nil {
				connection.Close()
				accepted <- name
			}
		}()
	}
	connection, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	connection.Close()
	select {
	case name := <-accepted:
		if name != "agent" {
			t.Fatalf("connected to the %s socket", name)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing accepted the connection")
	}
}

func TestAgentRefusesOtherProtocolVersionsAndUnknownMessages(t *testing.T) {
	server, client := net.Pipe()
	go handle(server, agentContext{userName: "student", capture: fakeCapture{}, lock: &fakeLocker{}})
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
	go func() {
		_ = serveConnections(listener, agentContext{userName: "student", capture: fakeCapture{}, lock: &fakeLocker{}})
	}()
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
	// The last free slot: it may be refused while the previous hello's slot
	// is still being released, so retry until it is answered and kept open.
	deadline := time.Now().Add(2 * time.Second)
	for {
		last, err := net.Dial("unix", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer last.Close()
		_ = last.SetDeadline(time.Now().Add(time.Second))
		_ = classroomview.Write(last, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion})
		if reply, err := classroomview.Read(last); err == nil && reply.Type == classroomview.TypeHello {
			_ = last.SetDeadline(time.Time{})
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the last free slot was never answered")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// Every slot is taken now: a further connection is refused at once,
	// before it sends anything.
	refused, err := net.Dial("unix", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer refused.Close()
	_ = refused.SetDeadline(time.Now().Add(2 * time.Second))
	if reply, err := classroomview.Read(refused); err != nil || reply.Code != classroomview.CodeBusy {
		t.Fatalf("over-limit reply = %+v, %v", reply, err)
	}
}

type fakeLocker struct {
	locked      bool
	unavailable bool
}

func (locker *fakeLocker) SetLocked(locked bool) error {
	if locker.unavailable {
		return errLockUnavailable
	}
	locker.locked = locked
	return nil
}

func (locker *fakeLocker) Locked() bool { return locker.locked }

func TestAgentLocksThroughTheExtension(t *testing.T) {
	lock := &fakeLocker{}
	server, client := net.Pipe()
	go handle(server, agentContext{userName: "student", capture: fakeCapture{image: []byte{0xff, 0xd8}}, lock: lock})
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
	if reply := exchange(classroomview.Message{Type: classroomview.TypeLock, Locked: true}); reply.Type != classroomview.TypeLockState || !reply.Locked || !lock.locked {
		t.Fatalf("lock reply = %+v", reply)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeThumbnailRequest, Width: 320}); reply.Type != classroomview.TypeThumbnail || !reply.Locked {
		t.Fatalf("thumbnail of a locked screen = %+v", reply)
	}
	if reply := exchange(classroomview.Message{Type: classroomview.TypeLock}); reply.Type != classroomview.TypeLockState || reply.Locked || lock.locked {
		t.Fatalf("unlock reply = %+v", reply)
	}
	lock.unavailable = true
	if reply := exchange(classroomview.Message{Type: classroomview.TypeLock, Locked: true}); reply.Code != classroomview.CodeLockUnavailable {
		t.Fatalf("lock without the extension = %+v", reply)
	}
	_ = client.Close()
}

type fakeShower struct {
	mutex sync.Mutex
	shown int
	stops int
}

func (shower *fakeShower) Show([]byte) error {
	shower.mutex.Lock()
	defer shower.mutex.Unlock()
	shower.shown++
	return nil
}

func (shower *fakeShower) Stop() {
	shower.mutex.Lock()
	defer shower.mutex.Unlock()
	shower.stops++
}

func TestAgentShowsTheTeachersScreenUntilTheConnectionEnds(t *testing.T) {
	screen := &fakeShower{}
	server, client := net.Pipe()
	go handle(server, agentContext{userName: "student", capture: fakeCapture{}, lock: &fakeLocker{}, screen: screen})
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
	if reply := exchange(classroomview.Message{Type: classroomview.TypeBroadcastFrame}); reply.Code != classroomview.CodeBadRequest {
		t.Fatalf("empty picture = %+v", reply)
	}
	for range 2 {
		if reply := exchange(classroomview.Message{Type: classroomview.TypeBroadcastFrame, Image: []byte{0xff, 0xd8}}); reply.Type != classroomview.TypeBroadcastShown {
			t.Fatalf("frame = %+v", reply)
		}
	}
	_ = client.Close()
	for attempt := 0; attempt < 100; attempt++ {
		screen.mutex.Lock()
		shown, stops := screen.shown, screen.stops
		screen.mutex.Unlock()
		if shown == 2 && stops == 1 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the showing did not end with its connection")
}

// hangingCommands puts a loginctl that never answers first in PATH.
func hangingCommands(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "loginctl"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func TestStuckLoginctlEndsAndTheControllerIsTold(t *testing.T) {
	hangingCommands(t)
	old := loginctlTimeout
	loginctlTimeout = 200 * time.Millisecond
	t.Cleanup(func() { loginctlTimeout = old })
	output := &bytes.Buffer{}
	started := time.Now()
	if err := connect(context.Background(), strings.NewReader(""), output, loginctl, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > 5*time.Second {
		t.Fatalf("connect waited %v for loginctl", time.Since(started))
	}
	reply, err := classroomview.Read(output)
	if err != nil || reply.Type != classroomview.TypeError || reply.Code != classroomview.CodeNoSession {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
	// The caller's own context ends it too.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := loginctl(ctx, "show-seat", "seat0"); err == nil {
		t.Fatal("loginctl ran with a cancelled context")
	}
}

func TestProbeEndsWhenItsContextEnds(t *testing.T) {
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "notes.txt"), []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]func(context.Context) error{
		"hello": func(ctx context.Context) error { return probe(ctx, []string{"sleep", "30"}, "", false, 0, io.Discard) },
		"lock":  func(ctx context.Context) error { return probeLock(ctx, []string{"sleep", "30"}, true, io.Discard) },
		"collect": func(ctx context.Context) error {
			return probeCollect(ctx, []string{"sleep", "30"}, t.TempDir(), io.Discard)
		},
		"send": func(ctx context.Context) error {
			return probeSend(ctx, []string{"sleep", "30"}, folder, io.Discard)
		},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		done := make(chan error, 1)
		go func() { done <- run(ctx) }()
		select {
		case err := <-done:
			if err == nil {
				t.Fatalf("%s: a command that never answered was a success", name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: the probe waited for a command that never answers", name)
		}
		cancel()
	}
	// A showing waits for its time, but not past the context.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	picture := filepath.Join(t.TempDir(), "screen.jpg")
	if err := os.WriteFile(picture, []byte("\xff\xd8\xff\xd9"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := probeShow(ctx, []string{"cat"}, picture, time.Minute, io.Discard); err == nil {
		t.Fatal("a showing outlived its context")
	}
}

func TestProbeNeedsACommandAndSaneArguments(t *testing.T) {
	for name, arguments := range map[string][]string{
		"no command":        nil,
		"no command (send)": {"--send", t.TempDir()},
		"too long a show":   {"--show", "screen.jpg", "61", "cat"},
	} {
		if err := runProbe(context.Background(), arguments, io.Discard); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}

// fakeAgent listens like the session's agent, in a temporary runtime
// directory laid out like /run/user, and hands every connection to behave.
// release ends the connections that behave leaves open.
func fakeAgent(t *testing.T, behave func(connection net.Conn, release <-chan struct{})) (string, int) {
	t.Helper()
	// Short, because a socket path is limited to about 100 bytes.
	base, err := os.MkdirTemp("", "cv")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(base) })
	uid := os.Getuid()
	runtime := filepath.Join(base, strconv.Itoa(uid))
	if err := os.Mkdir(runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(runtime, SocketName))
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	t.Cleanup(func() {
		close(release)
		_ = listener.Close()
	})
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer connection.Close()
				behave(connection, release)
			}()
		}
	}()
	return base, uid
}

func shortRelayDeadlines(t *testing.T) {
	t.Helper()
	hello, grace, write := helloTimeout, relayGrace, relayWriteTimeout
	helloTimeout, relayGrace, relayWriteTimeout = 200*time.Millisecond, 200*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { helloTimeout, relayGrace, relayWriteTimeout = hello, grace, write })
}

// answersHello is an agent that answers the first hello and then does what
// afterwards says.
func answersHello(afterwards func(connection net.Conn, release <-chan struct{})) func(net.Conn, <-chan struct{}) {
	return func(connection net.Conn, release <-chan struct{}) {
		if _, err := classroomview.Read(connection); err != nil {
			return
		}
		_ = classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion, User: "student"})
		afterwards(connection, release)
	}
}

func connectWithin(t *testing.T, input io.Reader, base string, uid int) *bytes.Buffer {
	t.Helper()
	output := &bytes.Buffer{}
	done := make(chan error, 1)
	go func() { done <- connect(context.Background(), input, output, graphicalSession(uid), base) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("connect: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("connect lingered on an agent that does not answer")
	}
	return output
}

func TestConnectReportsAFrozenAgentAsNotRunning(t *testing.T) {
	shortRelayDeadlines(t)
	// The socket exists and accepts, but nothing ever answers (SIGSTOP).
	base, uid := fakeAgent(t, func(_ net.Conn, release <-chan struct{}) { <-release })
	output := connectWithin(t, strings.NewReader(""), base, uid)
	reply, err := classroomview.Read(output)
	if err != nil || reply.Type != classroomview.TypeError || reply.Code != classroomview.CodeNoAgent {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
}

func TestConnectForwardsTheRefusalOfABusyAgent(t *testing.T) {
	shortRelayDeadlines(t)
	base, uid := fakeAgent(t, func(connection net.Conn, _ <-chan struct{}) {
		_, _ = classroomview.Read(connection)
		_ = classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeBusy})
	})
	reply, err := classroomview.Read(connectWithin(t, strings.NewReader(""), base, uid))
	if err != nil || reply.Type != classroomview.TypeError || reply.Code != classroomview.CodeBusy {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
}

func TestConnectEndsAfterTheControllerLeavesAFrozenAgent(t *testing.T) {
	shortRelayDeadlines(t)
	// The agent answers its first hello, then freezes: it reads nothing more
	// and never closes. The controller says hello and goes away.
	base, uid := fakeAgent(t, answersHello(func(_ net.Conn, release <-chan struct{}) { <-release }))
	request := &bytes.Buffer{}
	if err := classroomview.Write(request, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	output := connectWithin(t, request, base, uid)
	if output.Len() != 0 {
		t.Fatalf("the frozen agent answered %q", output.Bytes())
	}
}

func TestConnectEndsOnAFrozenAgentThatStopsReading(t *testing.T) {
	shortRelayDeadlines(t)
	// More data than a socket buffers, for an agent that stopped reading:
	// the blocked write must end the relay instead of holding it.
	base, uid := fakeAgent(t, answersHello(func(_ net.Conn, release <-chan struct{}) { <-release }))
	connectWithin(t, bytes.NewReader(make([]byte, 32<<20)), base, uid)
}

func TestConnectKeepsRelayingToAHealthyAgent(t *testing.T) {
	// A healthy agent still answers the controller's own hello, and the
	// relay ends with the agent's close, not with the grace period.
	base, uid := startAgent(t)
	request := &bytes.Buffer{}
	for range 2 {
		if err := classroomview.Write(request, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}); err != nil {
			t.Fatal(err)
		}
	}
	output := connectWithin(t, request, base, uid)
	for range 2 {
		if reply, err := classroomview.Read(output); err != nil || reply.Type != classroomview.TypeHello {
			t.Fatalf("reply = %+v, %v", reply, err)
		}
	}
}

type stoppingCapture struct {
	fakeCapture
	stopped chan struct{}
}

func (capture stoppingCapture) StopCapture() { capture.stopped <- struct{}{} }

func TestLastConnectionClosingStopsCapture(t *testing.T) {
	listener, err := net.Listen("unix", filepath.Join(t.TempDir(), SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	capture := stoppingCapture{stopped: make(chan struct{}, 4)}
	go func() {
		_ = serveConnections(listener, agentContext{userName: "student", capture: capture, lock: &fakeLocker{}})
	}()
	dial := func() net.Conn {
		connection, err := net.Dial("unix", listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		_ = classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion})
		_ = connection.SetDeadline(time.Now().Add(2 * time.Second))
		if _, err := classroomview.Read(connection); err != nil {
			t.Fatal(err)
		}
		return connection
	}
	first, second := dial(), dial()
	first.Close()
	select {
	case <-capture.stopped:
		t.Fatal("capture stopped while a channel was still open")
	case <-time.After(300 * time.Millisecond):
	}
	second.Close()
	select {
	case <-capture.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("capture kept running after the last channel closed")
	}
}

func TestHandleEndsWhenThePeerStopsReading(t *testing.T) {
	previous := replyWriteTimeout
	replyWriteTimeout = 100 * time.Millisecond
	defer func() { replyWriteTimeout = previous }()
	server, client := net.Pipe()
	defer client.Close()
	finished := make(chan struct{})
	go func() {
		handle(server, agentContext{userName: "student", capture: fakeCapture{}, lock: &fakeLocker{}})
		close(finished)
	}()
	// The peer asks and never reads the answer (a pipe holds no bytes).
	if err := classroomview.Write(client, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("a peer that stopped reading pinned the handler")
	}
}

// stalledObject answers a call only when its context ends, like a
// gnome-shell that stopped responding.
type stalledObject struct{ dbus.BusObject }

func (stalledObject) CallWithContext(ctx context.Context, method string, flags dbus.Flags, args ...any) *dbus.Call {
	<-ctx.Done()
	return &dbus.Call{Err: ctx.Err()}
}

func TestBusCallsGiveUpOnAStalledCompositor(t *testing.T) {
	previous := busTimeout
	busTimeout = 50 * time.Millisecond
	defer func() { busTimeout = previous }()
	started := time.Now()
	if err := callBus(stalledObject{}, "org.example.Method").Err; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("call error = %v", err)
	}
	if _, err := busProperty(stalledObject{}, "org.example", "Property"); err == nil {
		t.Fatal("a stalled property read succeeded")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("a stalled call was not bounded")
	}
}

// slowLocker answers Locked only after release is closed.
type slowLocker struct {
	fakeLocker
	release chan struct{}
}

func (locker *slowLocker) Locked() bool {
	<-locker.release
	return true
}

func TestHelloDoesNotWaitLongerThanTheRelayAllows(t *testing.T) {
	previous := helloLockedWait
	helloLockedWait = 50 * time.Millisecond
	defer func() { helloLockedWait = previous }()
	slow := &slowLocker{release: make(chan struct{})}
	started := time.Now()
	if lockedForHello(slow) {
		t.Fatal("an extension that did not answer counted as locked")
	}
	if time.Since(started) > time.Second {
		t.Fatal("the hello waited for a slow extension")
	}
	close(slow.release)
	if !lockedForHello(&fakeLocker{locked: true}) {
		t.Fatal("a prompt answer was ignored")
	}
}

func TestHelloWaitIsShorterThanTheRelaysHelloTimeout(t *testing.T) {
	if helloLockedWait >= helloTimeout {
		t.Fatalf("hello waits %v for the lock state but the relay allows %v", helloLockedWait, helloTimeout)
	}
}

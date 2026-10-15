// Command nixorium-classroom-agent is the classroom view agent of a client.
//
//	serve    runs in the user's graphical session and listens on a private
//	         Unix socket in XDG_RUNTIME_DIR.
//	connect  runs as root through the controller's fixed SSH command and
//	         relays standard input and output to the agent of the user of the
//	         active graphical session on seat0.
//	probe    runs a command (for example the SSH connection) and exchanges
//	         one hello through it, optionally a thumbnail and input, or a
//	         lock request (--lock on|off), sends a folder's contents to the
//	         desktop (--send DIR), collects the desktop into a folder
//	         (--collect DIR), or shows a picture as the teacher's screen for
//	         some seconds (--show JPEG SECONDS); used by tests and
//	         diagnostics.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

// SocketName is the agent socket inside the user's runtime directory.
const SocketName = "nixorium-classroom.sock"

var agentVersion = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: nixorium-classroom-agent serve|connect|probe COMMAND...")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(os.Getenv("XDG_RUNTIME_DIR"))
	case "connect":
		if os.Geteuid() != 0 {
			err = errors.New("connect must run as root")
			break
		}
		// The relay lives as long as the controller's channel and ends soon
		// after it closes; loginctl, the dial and the first hello of the
		// agent have their own deadlines (see connect).
		err = connect(context.Background(), os.Stdin, os.Stdout, loginctl, "/run/user")
	case "probe":
		ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
		err = runProbe(ctx, os.Args[2:], os.Stdout)
		cancel()
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "nixorium-classroom-agent:", err)
		os.Exit(1)
	}
}

// probeTimeout ends a probe, and the command it runs, that has not finished:
// the longest legitimate one sends or collects a desktop (500 MB at most).
const probeTimeout = 10 * time.Minute

// runProbe parses the arguments of the probe command and runs the check they
// name; the command it starts ends when ctx does.
func runProbe(ctx context.Context, arguments []string, output io.Writer) error {
	thumbnail, input := "", false
	if len(arguments) >= 3 && arguments[0] == "--show" {
		seconds, err := strconv.Atoi(arguments[2])
		if err != nil || seconds < 0 || seconds > 60 {
			return errors.New("--show needs a JPEG file and 0–60 seconds")
		}
		return probeShow(ctx, arguments[3:], arguments[1], time.Duration(seconds)*time.Second, output)
	}
	if len(arguments) >= 2 && arguments[0] == "--send" {
		return probeSend(ctx, arguments[2:], arguments[1], output)
	}
	if len(arguments) >= 2 && arguments[0] == "--collect" {
		return probeCollect(ctx, arguments[2:], arguments[1], output)
	}
	if len(arguments) >= 2 && arguments[0] == "--lock" && (arguments[1] == "on" || arguments[1] == "off") {
		return probeLock(ctx, arguments[2:], arguments[1] == "on", output)
	}
	if len(arguments) >= 1 && arguments[0] == "--input" {
		input, arguments = true, arguments[1:]
	}
	if len(arguments) >= 2 && arguments[0] == "--thumbnail" {
		thumbnail, arguments = arguments[1], arguments[2:]
	}
	return probe(ctx, arguments, thumbnail, input, output)
}

// startProbe runs command with its standard input and output for a probe to
// exchange messages through. The process is killed when ctx ends, which also
// ends any read of its output; finish closes its input and waits for it.
func startProbe(ctx context.Context, command []string) (io.Writer, io.Reader, func(), error) {
	if len(command) == 0 {
		return nil, nil, nil, errors.New("probe needs a command, for example nixorium-classroom-connect")
	}
	process := exec.CommandContext(ctx, command[0], command[1:]...)
	stdin, err := process.StdinPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return nil, nil, nil, err
	}
	process.Stderr = os.Stderr
	if err := process.Start(); err != nil {
		return nil, nil, nil, err
	}
	return stdin, stdout, func() {
		_ = stdin.Close()
		_ = process.Wait()
	}, nil
}

// maxConnections bounds simultaneous controller channels. The controller
// keeps one; the spare ones let a new channel work at once while a dead one
// (a dropped network or a restarted controller) waits for its read timeout.
const maxConnections = 4

// serve listens on the private socket and answers connections in parallel.
func serve(runtimeDirectory string) error {
	if !filepath.IsAbs(runtimeDirectory) {
		return errors.New("XDG_RUNTIME_DIR must be an absolute path")
	}
	path := filepath.Join(runtimeDirectory, SocketName)
	if err := removeStaleSocket(path); err != nil {
		return err
	}
	old := syscall.Umask(0o077)
	listener, err := net.Listen("unix", path)
	syscall.Umask(old)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	lock := &extensionLocker{}
	return serveConnections(listener, agentContext{userName: currentUserName(), home: home, capture: newMutterCapture(), lock: lock, screen: &extensionShower{locker: lock, runtime: runtimeDirectory}})
}

// agentContext is what every connection of the session's agent shares.
type agentContext struct {
	userName string
	home     string
	capture  capturer
	lock     locker
	screen   screenShower
}

func serveConnections(listener net.Listener, agent agentContext) error {
	slots := make(chan struct{}, maxConnections)
	for {
		connection, err := listener.Accept()
		if err != nil {
			return err
		}
		select {
		case slots <- struct{}{}:
			go func() {
				defer func() { <-slots }()
				handle(connection, agent)
			}()
		default:
			_ = connection.SetWriteDeadline(time.Now().Add(time.Second))
			_ = classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeBusy, Detail: "too many classroom view connections"})
			_ = connection.Close()
		}
	}
}

// removeStaleSocket removes a previous socket of this user, never anything else.
func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if info.Mode()&os.ModeSocket == 0 || !ok || int(stat.Uid) != os.Getuid() {
		return fmt.Errorf("%s exists and is not this user's socket", path)
	}
	return os.Remove(path)
}

func currentUserName() string {
	if current, err := user.Current(); err == nil {
		return current.Username
	}
	return strconv.Itoa(os.Getuid())
}

// handle answers one connection until the peer closes it or misbehaves.
func handle(connection net.Conn, agent agentContext) {
	defer connection.Close()
	userName, capture, lock := agent.userName, agent.capture, agent.lock
	// Files of an unfinished sending are removed when the connection ends,
	// and a showing of the teacher's screen through it ends too.
	var incoming *receiver
	// A collection of the desktop through this connection ends with it.
	var outgoing *collector
	showing := false
	defer func() {
		if incoming != nil {
			incoming.abort()
		}
		if outgoing != nil {
			outgoing.close()
		}
		if showing && agent.screen != nil {
			agent.screen.Stop()
		}
	}()
	for {
		_ = connection.SetReadDeadline(time.Now().Add(10 * time.Minute))
		message, err := classroomview.Read(connection)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_ = classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeBadRequest})
			}
			return
		}
		switch message.Type {
		case classroomview.TypeHello:
			if message.Version != classroomview.ProtocolVersion {
				_ = classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeUnsupported, Detail: fmt.Sprintf("protocol version %d is not supported", message.Version)})
				return
			}
			if err := classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion, Agent: agentVersion, User: userName, Locked: lock.Locked()}); err != nil {
				return
			}
		case classroomview.TypeThumbnailRequest:
			reply := thumbnailReply(capture, message.Width, message.Since)
			if reply.Type == classroomview.TypeThumbnail {
				reply.Locked = lock.Locked()
			}
			if err := classroomview.Write(connection, reply); err != nil {
				return
			}
		case classroomview.TypeLock:
			if err := classroomview.Write(connection, lockReply(lock, message.Locked)); err != nil {
				return
			}
		case classroomview.TypeBroadcastFrame, classroomview.TypeBroadcastStop:
			reply := classroomview.Message{Type: classroomview.TypeBroadcastShown}
			switch {
			case agent.screen == nil:
				reply = classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeBroadcastFailed}
			case message.Type == classroomview.TypeBroadcastStop:
				agent.screen.Stop()
				showing = false
			case len(message.Image) == 0 || len(message.Image) > classroomview.MaxBroadcastBytes:
				reply = classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeBadRequest, Detail: "the picture is empty or too large"}
			default:
				if err := agent.screen.Show(message.Image); err != nil {
					reply = classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeBroadcastFailed, Detail: err.Error()}
				} else {
					showing = true
				}
			}
			if err := classroomview.Write(connection, reply); err != nil {
				return
			}
		case classroomview.TypeFilesBegin, classroomview.TypeFilesChunk, classroomview.TypeFilesEnd:
			reply := filesReply(&incoming, agent.home, message)
			if err := classroomview.Write(connection, reply); err != nil {
				return
			}
		case classroomview.TypeCollectList, classroomview.TypeCollectChunk, classroomview.TypeCollectEnd:
			if err := classroomview.Write(connection, collectReply(&outgoing, agent.home, message)); err != nil {
				return
			}
		case classroomview.TypeInput:
			if err := classroomview.Write(connection, inputReply(capture, message)); err != nil {
				return
			}
		default:
			if err := classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeUnsupported}); err != nil {
				return
			}
		}
	}
}

// filesReply advances a sending to the desktop; any refusal ends it.
func filesReply(incoming **receiver, home string, message classroomview.Message) classroomview.Message {
	refuse := func(err error) classroomview.Message {
		if *incoming != nil {
			(*incoming).abort()
			*incoming = nil
		}
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeFilesRefused, Detail: err.Error()}
	}
	switch message.Type {
	case classroomview.TypeFilesBegin:
		if *incoming != nil {
			(*incoming).abort()
		}
		started, err := newReceiver(home, message.Entries)
		if err != nil {
			*incoming = nil
			return refuse(err)
		}
		*incoming = started
		return classroomview.Message{Type: classroomview.TypeFilesReady}
	case classroomview.TypeFilesChunk:
		if *incoming == nil {
			return refuse(errors.New("no sending is open"))
		}
		if err := (*incoming).chunk(message.Index, message.Data); err != nil {
			return refuse(err)
		}
		return classroomview.Message{Type: classroomview.TypeFilesAck, Index: message.Index}
	default:
		if *incoming == nil {
			return refuse(errors.New("no sending is open"))
		}
		placed, err := (*incoming).finish()
		*incoming = nil
		if err != nil {
			return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeFilesRefused, Detail: err.Error()}
		}
		return classroomview.Message{Type: classroomview.TypeFilesDone, Detail: strings.Join(placed, "\n")}
	}
}

func lockReply(lock locker, locked bool) classroomview.Message {
	if err := lock.SetLocked(locked); err != nil {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeLockUnavailable, Detail: err.Error()}
	}
	return classroomview.Message{Type: classroomview.TypeLockState, Locked: lock.Locked()}
}

func inputReply(capture capturer, message classroomview.Message) classroomview.Message {
	if err := classroomview.ValidateInput(message.Events); err != nil {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeBadRequest, Detail: err.Error()}
	}
	err := capture.Input(message.Events, message.Release)
	if errors.Is(err, errNotReady) {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNotReady, Detail: "the screen is not being viewed"}
	}
	if err != nil {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeCapture, Detail: err.Error()}
	}
	return classroomview.Message{Type: classroomview.TypeInputDone}
}

func thumbnailReply(capture capturer, width int, since int64) classroomview.Message {
	if width < classroomview.MinThumbnailWidth || width > classroomview.MaxThumbnailWidth {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeBadRequest, Detail: "thumbnail width is out of range"}
	}
	image, height, capturedAt, frame, err := capture.Thumbnail(width, since)
	if errors.Is(err, errNotReady) {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNotReady, Detail: err.Error()}
	}
	if errors.Is(err, errLocked) {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeLocked, Detail: err.Error()}
	}
	if err != nil {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeCapture, Detail: err.Error()}
	}
	return classroomview.Message{Type: classroomview.TypeThumbnail, Width: width, Height: height, Image: image, CapturedAt: capturedAt.UnixMilli(), Frame: frame}
}

// commandRunner runs loginctl with fixed arguments and returns its output.
type commandRunner func(ctx context.Context, arguments ...string) (string, error)

// Deadlines that keep a stuck loginctl or a frozen agent from holding the
// controller's channel: the controller then sees the failure instead of
// waiting. helloTimeout is how long an agent has to answer the relay's own
// hello before it counts as stopped; relayGrace is how long the agent has to
// finish after the controller closed its side; relayWriteTimeout bounds one
// write to an agent that stopped reading.
var (
	loginctlTimeout   = 10 * time.Second
	dialTimeout       = 5 * time.Second
	helloTimeout      = 3 * time.Second
	relayGrace        = 5 * time.Second
	relayWriteTimeout = 30 * time.Second
)

func loginctl(ctx context.Context, arguments ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, loginctlTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "loginctl", arguments...)
	// Do not wait for a pipe that a stuck child's own children keep open.
	command.WaitDelay = time.Second
	output, err := command.Output()
	return strings.TrimSpace(string(output)), err
}

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)

// activeSeatUser returns the user of the active graphical session on seat0.
// The login screen is a graphical session too, of the display manager's own
// user: only a session of class "user" counts as someone signed in, so a
// computer waiting at the login screen is "nobody signed in" and not one
// whose agent stopped.
func activeSeatUser(ctx context.Context, run commandRunner) (int, error) {
	session, err := run(ctx, "show-seat", "seat0", "--property=ActiveSession", "--value")
	if err != nil || !sessionIDPattern.MatchString(session) {
		return 0, errors.New("no active session on seat0")
	}
	kind, err := run(ctx, "show-session", session, "--property=Type", "--value")
	if err != nil || (kind != "wayland" && kind != "x11") {
		return 0, errors.New("the active session on seat0 is not graphical")
	}
	class, err := run(ctx, "show-session", session, "--property=Class", "--value")
	if err != nil || class != "user" {
		return 0, errors.New("nobody is signed in on seat0")
	}
	value, err := run(ctx, "show-session", session, "--property=User", "--value")
	if err != nil {
		return 0, errors.New("the active session has no user")
	}
	uid, err := strconv.Atoi(value)
	if err != nil || uid <= 0 {
		return 0, errors.New("the active session has no ordinary user")
	}
	return uid, nil
}

// agentSocket returns the socket of uid after checking it without following links.
func agentSocket(runtimeBase string, uid int) (string, error) {
	path := filepath.Join(runtimeBase, strconv.Itoa(uid), SocketName)
	info, err := os.Lstat(path)
	if err != nil {
		return "", errors.New("the classroom agent is not running in the active session")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if info.Mode()&os.ModeSocket == 0 || !ok || int(stat.Uid) != uid {
		return "", errors.New("the classroom agent socket is not owned by the session user")
	}
	return path, nil
}

// connect relays the controller's stream to the active session's agent. A
// failure is reported as a protocol error message so the controller can show it.
// An agent whose socket exists but that does not answer a hello within a few
// seconds (stopped with SIGSTOP, or hung) is reported as no-agent, like a
// missing socket, and the relay never outlives the controller's channel by
// more than a few seconds, even when the agent is frozen.
func connect(ctx context.Context, input io.Reader, output io.Writer, run commandRunner, runtimeBase string) error {
	uid, err := activeSeatUser(ctx, run)
	if err != nil {
		return classroomview.Write(output, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNoSession, Detail: err.Error()})
	}
	path, err := agentSocket(runtimeBase, uid)
	if err != nil {
		return classroomview.Write(output, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNoAgent, Detail: err.Error()})
	}
	connection, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "unix", path)
	if err != nil {
		return classroomview.Write(output, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNoAgent, Detail: "the classroom agent does not answer"})
	}
	defer connection.Close()
	reply, err := agentHello(connection)
	if err != nil {
		return classroomview.Write(output, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNoAgent, Detail: "the classroom agent does not answer"})
	}
	if reply.Type != classroomview.TypeHello {
		// The agent refuses (too many channels, for example) and closes the
		// connection: the controller gets the refusal itself.
		return classroomview.Write(output, reply)
	}
	return relay(input, output, connection)
}

// agentHello asks the agent for its hello within helloTimeout. The controller
// sends its own hello afterwards through the same connection.
func agentHello(connection net.Conn) (classroomview.Message, error) {
	_ = connection.SetDeadline(time.Now().Add(helloTimeout))
	defer func() { _ = connection.SetDeadline(time.Time{}) }()
	if err := classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}); err != nil {
		return classroomview.Message{}, err
	}
	return classroomview.Read(connection)
}

// relay copies between the controller and the agent until either side ends.
// When the controller closes its side the agent gets relayGrace to finish; a
// write that cannot complete within relayWriteTimeout closes the connection.
func relay(input io.Reader, output io.Writer, connection net.Conn) error {
	grace, writeTimeout := relayGrace, relayWriteTimeout
	finished := make(chan struct{})
	defer close(finished)
	var lingered atomic.Bool
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		buffer := make([]byte, 32<<10)
		for {
			count, readErr := input.Read(buffer)
			if count > 0 {
				_ = connection.SetWriteDeadline(time.Now().Add(writeTimeout))
				if _, err := connection.Write(buffer[:count]); err != nil {
					lingered.Store(true)
					_ = connection.Close()
					return
				}
			}
			if readErr != nil {
				break
			}
		}
		if unix, ok := connection.(*net.UnixConn); ok {
			_ = unix.CloseWrite()
		}
	}()
	go func() {
		select {
		case <-inputDone:
		case <-finished:
			return
		}
		timer := time.NewTimer(grace)
		defer timer.Stop()
		select {
		case <-timer.C:
			lingered.Store(true)
			_ = connection.Close()
		case <-finished:
		}
	}()
	_, err := io.Copy(output, connection)
	if lingered.Load() {
		// The relay ended the connection itself: nothing is left to report.
		return nil
	}
	return err
}

// probeSend sends the contents of a folder to the session's desktop through
// a command, as the controller does. Hidden names, links and special files
// are skipped.
func probeSend(ctx context.Context, command []string, folder string, output io.Writer) error {
	entries, err := classroomview.CollectFolder(folder)
	if err != nil {
		return err
	}
	stdin, stdout, finish, err := startProbe(ctx, command)
	if err != nil {
		return err
	}
	defer finish()
	exchange := func(message classroomview.Message, want string) (classroomview.Message, error) {
		if err := classroomview.Write(stdin, message); err != nil {
			return classroomview.Message{}, err
		}
		reply, err := classroomview.Read(stdout)
		if err == nil && reply.Type != want {
			err = fmt.Errorf("the agent answered %s %s: %s", reply.Type, reply.Code, reply.Detail)
		}
		return reply, err
	}
	if _, err := exchange(classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}, classroomview.TypeHello); err != nil {
		return err
	}
	if _, err := exchange(classroomview.Message{Type: classroomview.TypeFilesBegin, Entries: entries}, classroomview.TypeFilesReady); err != nil {
		return err
	}
	buffer := make([]byte, classroomview.MaxChunkBytes)
	for index, entry := range entries {
		if entry.Dir || entry.Size == 0 {
			continue
		}
		file, err := os.Open(filepath.Join(folder, filepath.FromSlash(entry.Path)))
		if err != nil {
			return err
		}
		for sent := int64(0); sent < entry.Size; {
			count, err := file.Read(buffer[:min(int64(len(buffer)), entry.Size-sent)])
			if count > 0 {
				if _, err := exchange(classroomview.Message{Type: classroomview.TypeFilesChunk, Index: index, Data: buffer[:count]}, classroomview.TypeFilesAck); err != nil {
					_ = file.Close()
					return err
				}
				sent += int64(count)
			}
			if err != nil {
				_ = file.Close()
				return err
			}
		}
		_ = file.Close()
	}
	reply, err := exchange(classroomview.Message{Type: classroomview.TypeFilesEnd}, classroomview.TypeFilesDone)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "files.done %s\n", strings.ReplaceAll(reply.Detail, "\n", ", "))
	return nil
}

// probeShow shows a picture over the session's screen, keeps it for a while
// and ends the showing, as the controller does.
func probeShow(ctx context.Context, command []string, picture string, duration time.Duration, output io.Writer) error {
	image, err := os.ReadFile(picture)
	if err != nil {
		return err
	}
	stdin, stdout, finish, err := startProbe(ctx, command)
	if err != nil {
		return err
	}
	defer finish()
	exchange := func(message classroomview.Message) error {
		if err := classroomview.Write(stdin, message); err != nil {
			return err
		}
		reply, err := classroomview.Read(stdout)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "%s code=%s detail=%s\n", reply.Type, reply.Code, reply.Detail)
		if reply.Type == classroomview.TypeError {
			return errors.New("the agent refused the picture")
		}
		return nil
	}
	if err := exchange(classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}); err != nil {
		return err
	}
	if err := exchange(classroomview.Message{Type: classroomview.TypeBroadcastFrame, Image: image}); err != nil {
		return err
	}
	select {
	case <-time.After(duration):
	case <-ctx.Done():
		return ctx.Err()
	}
	return exchange(classroomview.Message{Type: classroomview.TypeBroadcastStop})
}

// probeLock locks or unlocks through a command, as the controller does.
func probeLock(ctx context.Context, command []string, locked bool, output io.Writer) error {
	stdin, stdout, finish, err := startProbe(ctx, command)
	if err != nil {
		return err
	}
	defer finish()
	for _, message := range []classroomview.Message{
		{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion},
		{Type: classroomview.TypeLock, Locked: locked},
	} {
		if err := classroomview.Write(stdin, message); err != nil {
			return err
		}
		reply, err := classroomview.Read(stdout)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "%s locked=%t code=%s detail=%s\n", reply.Type, reply.Locked, reply.Code, reply.Detail)
		if reply.Type == classroomview.TypeError {
			return errors.New("the agent refused the lock request")
		}
	}
	return nil
}

// probe exchanges one hello through a command's standard input and output.
func probe(ctx context.Context, command []string, thumbnail string, input bool, output io.Writer) error {
	stdin, stdout, finish, err := startProbe(ctx, command)
	if err != nil {
		return err
	}
	defer finish()
	if err := classroomview.Write(stdin, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}); err != nil {
		return err
	}
	reply, err := classroomview.Read(stdout)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "%s version=%d agent=%s user=%s code=%s detail=%s\n", reply.Type, reply.Version, reply.Agent, reply.User, reply.Code, reply.Detail)
	if reply.Type != classroomview.TypeHello {
		return errors.New("the agent did not answer hello")
	}
	if thumbnail == "" {
		return nil
	}
	// Frames come only when the screen changes; retry while the first is pending.
	for attempt := 0; attempt < 10; attempt++ {
		if err := classroomview.Write(stdin, classroomview.Message{Type: classroomview.TypeThumbnailRequest, Width: 320}); err != nil {
			return err
		}
		reply, err = classroomview.Read(stdout)
		if err != nil {
			return err
		}
		if reply.Type == classroomview.TypeThumbnail {
			fmt.Fprintf(output, "thumbnail width=%d height=%d bytes=%d\n", reply.Width, reply.Height, len(reply.Image))
			if err := os.WriteFile(thumbnail, reply.Image, 0o644); err != nil {
				return err
			}
			if !input {
				return nil
			}
			// A pointer move and a Shift tap, then release everything.
			events := []classroomview.InputEvent{
				{Kind: classroomview.InputPointerMove, X: 0.5, Y: 0.5},
				{Kind: classroomview.InputKey, Keysym: 0xffe1, Pressed: true},
				{Kind: classroomview.InputKey, Keysym: 0xffe1},
			}
			if err := classroomview.Write(stdin, classroomview.Message{Type: classroomview.TypeInput, Events: events, Release: true}); err != nil {
				return err
			}
			reply, err = classroomview.Read(stdout)
			if err != nil {
				return err
			}
			fmt.Fprintf(output, "%s code=%s detail=%s\n", reply.Type, reply.Code, reply.Detail)
			if reply.Type != classroomview.TypeInputDone {
				return errors.New("the agent refused the input")
			}
			return nil
		}
		fmt.Fprintf(output, "%s code=%s detail=%s\n", reply.Type, reply.Code, reply.Detail)
		if reply.Code != classroomview.CodeNotReady {
			return errors.New("the agent did not return a thumbnail")
		}
		time.Sleep(time.Second)
	}
	return errors.New("no thumbnail after ten attempts")
}

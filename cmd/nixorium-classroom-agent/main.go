// Command nixorium-classroom-agent is the classroom view agent of a client.
//
//	serve    runs in the user's graphical session and listens on a private
//	         Unix socket in XDG_RUNTIME_DIR.
//	connect  runs as root through the controller's fixed SSH command and
//	         relays standard input and output to the agent of the user of the
//	         active graphical session on seat0.
//	probe    runs a command (for example the SSH connection) and exchanges
//	         one hello through it; used by tests and diagnostics.
package main

import (
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
		err = connect(os.Stdin, os.Stdout, loginctl, "/run/user")
	case "probe":
		arguments, thumbnail := os.Args[2:], ""
		if len(arguments) >= 2 && arguments[0] == "--thumbnail" {
			thumbnail, arguments = arguments[1], arguments[2:]
		}
		err = probe(arguments, thumbnail, os.Stdout)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "nixorium-classroom-agent:", err)
		os.Exit(1)
	}
}

// serve listens on the private socket and answers each connection in turn.
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
	name := currentUserName()
	capture := newMutterCapture()
	for {
		connection, err := listener.Accept()
		if err != nil {
			return err
		}
		handle(connection, name, capture)
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
func handle(connection net.Conn, userName string, capture capturer) {
	defer connection.Close()
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
			if err := classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion, Agent: agentVersion, User: userName}); err != nil {
				return
			}
		case classroomview.TypeThumbnailRequest:
			if err := classroomview.Write(connection, thumbnailReply(capture, message.Width)); err != nil {
				return
			}
		default:
			if err := classroomview.Write(connection, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeUnsupported}); err != nil {
				return
			}
		}
	}
}

func thumbnailReply(capture capturer, width int) classroomview.Message {
	if width < classroomview.MinThumbnailWidth || width > classroomview.MaxThumbnailWidth {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeBadRequest, Detail: "thumbnail width is out of range"}
	}
	image, height, capturedAt, err := capture.Thumbnail(width)
	if errors.Is(err, errNotReady) {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNotReady, Detail: err.Error()}
	}
	if err != nil {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeCapture, Detail: err.Error()}
	}
	return classroomview.Message{Type: classroomview.TypeThumbnail, Width: width, Height: height, Image: image, CapturedAt: capturedAt.UnixMilli()}
}

// commandRunner runs loginctl with fixed arguments and returns its output.
type commandRunner func(arguments ...string) (string, error)

func loginctl(arguments ...string) (string, error) {
	output, err := exec.Command("loginctl", arguments...).Output()
	return strings.TrimSpace(string(output)), err
}

var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)

// activeSeatUser returns the user of the active graphical session on seat0.
func activeSeatUser(run commandRunner) (int, error) {
	session, err := run("show-seat", "seat0", "--property=ActiveSession", "--value")
	if err != nil || !sessionIDPattern.MatchString(session) {
		return 0, errors.New("no active session on seat0")
	}
	kind, err := run("show-session", session, "--property=Type", "--value")
	if err != nil || (kind != "wayland" && kind != "x11") {
		return 0, errors.New("the active session on seat0 is not graphical")
	}
	value, err := run("show-session", session, "--property=User", "--value")
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
func connect(input io.Reader, output io.Writer, run commandRunner, runtimeBase string) error {
	uid, err := activeSeatUser(run)
	if err != nil {
		return classroomview.Write(output, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNoSession, Detail: err.Error()})
	}
	path, err := agentSocket(runtimeBase, uid)
	if err != nil {
		return classroomview.Write(output, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNoAgent, Detail: err.Error()})
	}
	connection, err := net.Dial("unix", path)
	if err != nil {
		return classroomview.Write(output, classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNoAgent, Detail: "the classroom agent does not answer"})
	}
	defer connection.Close()
	go func() {
		_, _ = io.Copy(connection, input)
		if unix, ok := connection.(*net.UnixConn); ok {
			_ = unix.CloseWrite()
		}
	}()
	_, err = io.Copy(output, connection)
	return err
}

// probe exchanges one hello through a command's standard input and output.
func probe(command []string, thumbnail string, output io.Writer) error {
	if len(command) == 0 {
		return errors.New("probe needs a command, for example nixorium-classroom-connect")
	}
	process := exec.Command(command[0], command[1:]...)
	stdin, err := process.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return err
	}
	process.Stderr = os.Stderr
	if err := process.Start(); err != nil {
		return err
	}
	defer func() {
		_ = stdin.Close()
		_ = process.Wait()
	}()
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
			return os.WriteFile(thumbnail, reply.Image, 0o644)
		}
		fmt.Fprintf(output, "%s code=%s detail=%s\n", reply.Type, reply.Code, reply.Detail)
		if reply.Code != classroomview.CodeNotReady {
			return errors.New("the agent did not return a thumbnail")
		}
		time.Sleep(time.Second)
	}
	return errors.New("no thumbnail after ten attempts")
}

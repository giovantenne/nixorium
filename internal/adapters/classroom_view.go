package adapters

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// classroomAgentCommand is the only remote command used for the classroom
// view; the client runs it as root and relays to the session's agent.
const classroomAgentCommand = "nixorium-classroom-connect"

// ClassroomAgentConnector opens classroom view sessions over the controller's
// existing SSH access, with the same fixed arguments as power and Internet.
type ClassroomAgentConnector struct{}

func (ClassroomAgentConnector) Connect(ctx context.Context, host domain.HostMeta) (classroomview.Session, error) {
	command := exec.CommandContext(ctx, "ssh", shutdownSSHArguments(host, 5*time.Second, classroomAgentCommand)...)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	session := &sshAgentSession{command: command, input: stdin, output: stdout}
	reply, err := session.exchange(classroomview.Message{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}, 10*time.Second)
	if err != nil {
		session.Close()
		return nil, classroomview.ErrUnreachable
	}
	if reply.Type != classroomview.TypeHello {
		session.Close()
		return nil, classroomview.AgentError{Code: reply.Code}
	}
	session.locked = reply.Locked
	return session, nil
}

type sshAgentSession struct {
	mutex   sync.Mutex
	command *exec.Cmd
	input   io.WriteCloser
	output  io.Reader
	closed  bool
	locked  bool
}

func (session *sshAgentSession) Locked() bool { return session.locked }

func (session *sshAgentSession) Thumbnail(width int, since int64) (classroomview.Message, error) {
	return session.exchange(classroomview.Message{Type: classroomview.TypeThumbnailRequest, Width: width, Since: since}, 10*time.Second)
}

func (session *sshAgentSession) Input(events []classroomview.InputEvent, release bool) error {
	reply, err := session.exchange(classroomview.Message{Type: classroomview.TypeInput, Events: events, Release: release}, 5*time.Second)
	if err != nil {
		return err
	}
	if reply.Type != classroomview.TypeInputDone {
		return classroomview.AgentError{Code: reply.Code}
	}
	return nil
}

func (session *sshAgentSession) SetLocked(locked bool) (bool, error) {
	reply, err := session.exchange(classroomview.Message{Type: classroomview.TypeLock, Locked: locked}, 10*time.Second)
	if err != nil {
		return false, err
	}
	if reply.Type != classroomview.TypeLockState {
		return false, classroomview.AgentError{Code: reply.Code}
	}
	return reply.Locked, nil
}

func (session *sshAgentSession) ShowFrame(image []byte) error {
	return session.broadcast(classroomview.Message{Type: classroomview.TypeBroadcastFrame, Image: image})
}

func (session *sshAgentSession) StopBroadcast() error {
	return session.broadcast(classroomview.Message{Type: classroomview.TypeBroadcastStop})
}

func (session *sshAgentSession) broadcast(request classroomview.Message) error {
	reply, err := session.exchange(request, 10*time.Second)
	if err != nil {
		return err
	}
	if reply.Type != classroomview.TypeBroadcastShown {
		return classroomview.AgentError{Code: reply.Code}
	}
	return nil
}

func (session *sshAgentSession) SendFiles(entries []classroomview.FileEntry, content func(int) (io.ReadCloser, error)) ([]string, error) {
	expect := func(request classroomview.Message, want string, timeout time.Duration) (classroomview.Message, error) {
		reply, err := session.exchange(request, timeout)
		if err != nil {
			return reply, err
		}
		if reply.Type != want {
			return reply, classroomview.AgentError{Code: reply.Code}
		}
		return reply, nil
	}
	if _, err := expect(classroomview.Message{Type: classroomview.TypeFilesBegin, Entries: entries}, classroomview.TypeFilesReady, 30*time.Second); err != nil {
		return nil, err
	}
	buffer := make([]byte, classroomview.MaxChunkBytes)
	for index, entry := range entries {
		if entry.Dir || entry.Size == 0 {
			continue
		}
		reader, err := content(index)
		if err != nil {
			return nil, err
		}
		for sent := int64(0); sent < entry.Size; {
			count, readErr := io.ReadFull(reader, buffer[:min(int64(len(buffer)), entry.Size-sent)])
			if count > 0 {
				if _, err := expect(classroomview.Message{Type: classroomview.TypeFilesChunk, Index: index, Data: buffer[:count]}, classroomview.TypeFilesAck, 30*time.Second); err != nil {
					_ = reader.Close()
					return nil, err
				}
				sent += int64(count)
			}
			if readErr != nil && sent < entry.Size {
				_ = reader.Close()
				return nil, readErr
			}
		}
		_ = reader.Close()
	}
	reply, err := expect(classroomview.Message{Type: classroomview.TypeFilesEnd}, classroomview.TypeFilesDone, 60*time.Second)
	if err != nil {
		return nil, err
	}
	if reply.Detail == "" {
		return []string{}, nil
	}
	return strings.Split(reply.Detail, "\n"), nil
}

// exchange sends one request and waits for its reply, closing the session
// when the client does not answer in time.
func (session *sshAgentSession) exchange(request classroomview.Message, timeout time.Duration) (classroomview.Message, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	if session.closed {
		return classroomview.Message{}, errors.New("classroom agent session is closed")
	}
	type result struct {
		message classroomview.Message
		err     error
	}
	done := make(chan result, 1)
	go func() {
		if err := classroomview.Write(session.input, request); err != nil {
			done <- result{err: err}
			return
		}
		message, err := classroomview.Read(session.output)
		done <- result{message, err}
	}()
	select {
	case outcome := <-done:
		return outcome.message, outcome.err
	case <-time.After(timeout):
		session.closeLocked()
		return classroomview.Message{}, errors.New("the classroom agent did not answer in time")
	}
}

func (session *sshAgentSession) Close() error {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	session.closeLocked()
	return nil
}

func (session *sshAgentSession) closeLocked() {
	if session.closed {
		return
	}
	session.closed = true
	_ = session.input.Close()
	if session.command.Process != nil {
		_ = session.command.Process.Kill()
	}
	go func() { _ = session.command.Wait() }()
}

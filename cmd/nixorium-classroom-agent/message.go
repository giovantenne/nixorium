package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

// The teacher's message is drawn by the classroom extension in the same
// session; the agent only validates what it is asked to show and passes it
// on over the session bus, like the lock. The message belongs to the session,
// not to the controller's connection: it stays when the connection ends, and
// logging out clears it.

var errMessageUnavailable = errors.New("the classroom extension cannot show messages in this session")

// messenger shows and removes the message through the extension.
type messenger interface {
	ShowMessage(text string, seconds int, untilUnlock bool) error
	HideMessage() error
}

func (locker *extensionLocker) ShowMessage(text string, seconds int, untilUnlock bool) error {
	return locker.messageCall("ShowMessage", text, int32(seconds), untilUnlock)
}

func (locker *extensionLocker) HideMessage() error {
	return locker.messageCall("HideMessage")
}

// messageCall maps any failure, including an extension that predates
// messages, to errMessageUnavailable.
func (locker *extensionLocker) messageCall(method string, arguments ...any) error {
	object, err := locker.object()
	if err != nil {
		return errMessageUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	if err := object.CallWithContext(ctx, lockBusName+"."+method, 0, arguments...).Err; err != nil {
		return errMessageUnavailable
	}
	return nil
}

func messageReply(notice messenger, message classroomview.Message) classroomview.Message {
	unavailable := func(err error) classroomview.Message {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeMessageUnavailable, Detail: err.Error()}
	}
	if notice == nil {
		return unavailable(errMessageUnavailable)
	}
	if message.Type == classroomview.TypeMessageHide {
		if err := notice.HideMessage(); err != nil {
			return unavailable(err)
		}
		return classroomview.Message{Type: classroomview.TypeMessageHidden}
	}
	if err := classroomview.ValidateMessage(message.Text, message.Seconds, message.UntilUnlock); err != nil {
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeBadRequest, Detail: err.Error()}
	}
	if err := notice.ShowMessage(message.Text, message.Seconds, message.UntilUnlock); err != nil {
		return unavailable(err)
	}
	return classroomview.Message{Type: classroomview.TypeMessageShown}
}

// probeMessage shows a message through a command, as the controller does,
// or removes it when show is false. duration is 0 (until the student closes
// it), a number of seconds, or "unlock" (until the computer is unlocked).
func probeMessage(command []string, show bool, text, duration string, output io.Writer) error {
	if len(command) == 0 {
		return errors.New("probe needs a command, for example nixorium-classroom-connect")
	}
	request := classroomview.Message{Type: classroomview.TypeMessageHide}
	if show {
		request = classroomview.Message{Type: classroomview.TypeMessageShow, Text: text}
		if duration == "unlock" {
			request.UntilUnlock = true
		} else if seconds, err := strconv.Atoi(duration); err == nil {
			request.Seconds = seconds
		} else {
			return errors.New("--message needs a text and 0–600 seconds or \"unlock\"")
		}
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
	for _, message := range []classroomview.Message{{Type: classroomview.TypeHello, Version: classroomview.ProtocolVersion}, request} {
		if err := classroomview.Write(stdin, message); err != nil {
			return err
		}
		reply, err := classroomview.Read(stdout)
		if err != nil {
			return err
		}
		fmt.Fprintf(output, "%s code=%s detail=%s\n", reply.Type, reply.Code, reply.Detail)
		if reply.Type == classroomview.TypeError {
			return errors.New("the agent refused the message")
		}
	}
	return nil
}

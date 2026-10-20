package main

import (
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/godbus/dbus/v5"
)

// The shared clipboard carries plain text only, in both directions, and only
// while the teacher controls the computer (the worker asks for ClipboardBegin
// when control starts and ClipboardEnd when it ends). The text is never
// logged and never kept after the control ends.

// textMimes are the text formats offered to the session and accepted from it,
// best first.
var textMimes = []string{"text/plain;charset=utf-8", "text/plain"}

const (
	// clipboardTransferTimeout bounds a read or a write through the pipe
	// Mutter gives us.
	clipboardTransferTimeout = 2 * time.Second
	// clipboardOwnerWait is how long a read waits for a copy that the
	// teacher's Ctrl+C has just started.
	clipboardOwnerWait = 1500 * time.Millisecond
)

var errClipboardOff = errors.New("the shared clipboard is not on")

// clipboardState is guarded by the capture's mutex.
type clipboardState struct {
	on bool
	// text is what the teacher pasted and the session may fetch.
	text string
	// ownerSeq counts the copies made by the session's user; readSeq the
	// ones already returned.
	ownerSeq, readSeq int
	ownerMimes        []string
}

func (capture *mutterCapture) ClipboardBegin() error {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	if !capture.running || capture.remote == nil {
		return errNotReady
	}
	capture.lastRequest = time.Now()
	if capture.clipboard.on {
		return nil
	}
	if err := capture.remote.Call(remoteDesktopName+".Session.EnableClipboard", 0, map[string]dbus.Variant{}).Err; err != nil {
		return err
	}
	capture.clipboard = clipboardState{on: true}
	return nil
}

func (capture *mutterCapture) ClipboardEnd() error {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	capture.clipboardOffLocked()
	return nil
}

// clipboardOffLocked turns the clipboard off and forgets any text.
func (capture *mutterCapture) clipboardOffLocked() {
	if capture.clipboard.on && capture.remote != nil {
		_ = capture.remote.Call(remoteDesktopName+".Session.DisableClipboard", 0).Err
	}
	capture.clipboard = clipboardState{}
	capture.changed.Broadcast()
}

// ClipboardSet makes text the session's clipboard; the session fetches it
// when an application pastes (selectionSignal).
func (capture *mutterCapture) ClipboardSet(text string) error {
	text, err := classroomview.ValidateClipboardText(text)
	if err != nil {
		return err
	}
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	if !capture.running || capture.remote == nil {
		return errNotReady
	}
	if !capture.clipboard.on {
		return errClipboardOff
	}
	capture.lastRequest = time.Now()
	capture.clipboard.text = text
	options := map[string]dbus.Variant{"mime-types": dbus.MakeVariant(textMimes)}
	return capture.remote.Call(remoteDesktopName+".Session.SetSelection", 0, options).Err
}

// ClipboardGet returns the text the session's user copied since the last
// call, waiting a moment for a copy that has just been made; "" means
// nothing new or nothing textual.
func (capture *mutterCapture) ClipboardGet() (string, error) {
	capture.mutex.Lock()
	if !capture.running || capture.remote == nil {
		capture.mutex.Unlock()
		return "", errNotReady
	}
	if !capture.clipboard.on {
		capture.mutex.Unlock()
		return "", errClipboardOff
	}
	capture.lastRequest = time.Now()
	deadline := time.Now().Add(clipboardOwnerWait)
	for capture.clipboard.on && capture.clipboard.ownerSeq == capture.clipboard.readSeq && time.Now().Before(deadline) {
		timer := time.AfterFunc(50*time.Millisecond, capture.changed.Broadcast)
		capture.changed.Wait()
		timer.Stop()
	}
	if !capture.clipboard.on || capture.remote == nil || capture.clipboard.ownerSeq == capture.clipboard.readSeq {
		capture.mutex.Unlock()
		return "", nil
	}
	capture.clipboard.readSeq = capture.clipboard.ownerSeq
	mime := pickTextMime(capture.clipboard.ownerMimes)
	remote := capture.remote
	capture.mutex.Unlock()
	if mime == "" {
		return "", nil
	}
	var descriptor dbus.UnixFD
	if err := remote.Call(remoteDesktopName+".Session.SelectionRead", 0, mime).Store(&descriptor); err != nil {
		return "", err
	}
	data, err := readSelection(fileFromDescriptor(int(descriptor)), classroomview.MaxClipboardBytes, clipboardTransferTimeout)
	if err != nil {
		return "", err
	}
	return classroomview.ValidateClipboardText(string(data))
}

// pickTextMime chooses the best text format the session's owner offers.
func pickTextMime(offered []string) string {
	for _, mime := range textMimes {
		if slices.Contains(offered, mime) {
			return mime
		}
	}
	return ""
}

func isTextMime(mime string) bool { return slices.Contains(textMimes, mime) }

// selectionSignal handles SelectionOwnerChanged (the user copied something)
// and SelectionTransfer (an application pastes the text we set).
func (capture *mutterCapture) selectionSignal(generation int, remote dbus.BusObject, signal *dbus.Signal) {
	switch {
	case strings.HasSuffix(signal.Name, ".SelectionOwnerChanged") && len(signal.Body) == 1:
		options, _ := signal.Body[0].(map[string]dbus.Variant)
		own, _ := options["session-is-owner"].Value().(bool)
		mimes, _ := options["mime-types"].Value().([]string)
		capture.mutex.Lock()
		if generation == capture.generation && capture.clipboard.on && !own {
			capture.clipboard.ownerSeq++
			capture.clipboard.ownerMimes = mimes
			capture.changed.Broadcast()
		}
		capture.mutex.Unlock()
	case strings.HasSuffix(signal.Name, ".SelectionTransfer") && len(signal.Body) == 2:
		mime, _ := signal.Body[0].(string)
		serial, _ := signal.Body[1].(uint32)
		capture.mutex.Lock()
		text := capture.clipboard.text
		allowed := generation == capture.generation && capture.clipboard.on && text != "" && isTextMime(mime)
		capture.mutex.Unlock()
		go answerTransfer(remote, serial, text, allowed)
	}
}

// answerTransfer writes the teacher's text into the pipe Mutter hands out.
func answerTransfer(remote dbus.BusObject, serial uint32, text string, allowed bool) {
	done := func(ok bool) {
		_ = remote.Call(remoteDesktopName+".Session.SelectionWriteDone", 0, serial, ok).Err
	}
	if !allowed {
		done(false)
		return
	}
	var descriptor dbus.UnixFD
	if err := remote.Call(remoteDesktopName+".Session.SelectionWrite", 0, serial).Store(&descriptor); err != nil {
		done(false)
		return
	}
	done(writeSelection(fileFromDescriptor(int(descriptor)), []byte(text), clipboardTransferTimeout) == nil)
}

// fileFromDescriptor wraps a pipe received over D-Bus; it is non-blocking so
// that deadlines work.
func fileFromDescriptor(descriptor int) *os.File {
	_ = syscall.SetNonblock(descriptor, true)
	return os.NewFile(uintptr(descriptor), "selection")
}

// readSelection reads at most limit bytes from file within timeout and closes
// it; more data than limit is an error, not a truncation.
func readSelection(file *os.File, limit int, timeout time.Duration) ([]byte, error) {
	defer file.Close()
	_ = file.SetReadDeadline(time.Now().Add(timeout))
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, classroomview.ErrClipboardTooLarge
	}
	return data, nil
}

// writeSelection writes data to file within timeout and closes it.
func writeSelection(file *os.File, data []byte, timeout time.Duration) error {
	defer file.Close()
	_ = file.SetWriteDeadline(time.Now().Add(timeout))
	_, err := file.Write(data)
	return err
}

// clipboardReply answers the shared clipboard messages. Failures carry short
// codes; the text itself is never put in an error or a log.
func clipboardReply(capture capturer, message classroomview.Message) classroomview.Message {
	var text string
	var err error
	switch message.Type {
	case classroomview.TypeClipboardBegin:
		err = capture.ClipboardBegin()
	case classroomview.TypeClipboardEnd:
		err = capture.ClipboardEnd()
	case classroomview.TypeClipboardSet:
		err = capture.ClipboardSet(message.Text)
	default:
		text, err = capture.ClipboardGet()
	}
	fail := func(code string) classroomview.Message {
		return classroomview.Message{Type: classroomview.TypeError, Code: code}
	}
	switch {
	case err == nil:
		return classroomview.Message{Type: classroomview.TypeClipboardDone, Text: text}
	case errors.Is(err, classroomview.ErrClipboardTooLarge):
		return fail(classroomview.CodeClipboardTooLarge)
	case errors.Is(err, errLocked):
		return fail(classroomview.CodeLocked)
	case errors.Is(err, errNotReady):
		return fail(classroomview.CodeNotReady)
	}
	return fail(classroomview.CodeClipboard)
}

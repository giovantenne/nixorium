package main

import "sync"

// While the teacher controls the computer, the classroom extension shows a
// flag on the student's screen. The agent asks for it with the first input
// and takes it back when the control ends. It is best effort: an extension
// that does not answer (one from before this notice) never stops the control.

// stateNotifier tells the extension two things about the session: the
// teacher controls it (the flag), and a capture session exists (the small
// indicator the extension shows while a full-screen window hides GNOME's
// top bar).
type stateNotifier interface {
	SetControlled(controlled bool) error
	SetSharing(sharing bool) error
}

func (locker *extensionLocker) SetControlled(controlled bool) error {
	return locker.messageCall("SetControlled", controlled)
}

func (locker *extensionLocker) SetSharing(sharing bool) error {
	return locker.messageCall("SetSharing", sharing)
}

// extensionFlag applies one wanted state in the background, so a slow
// session bus never holds the capture; only the latest wish is kept.
type extensionFlag struct {
	apply func(bool) error
	mutex sync.Mutex
	want  bool
	shown bool
	wake  chan struct{}
}

func newExtensionFlag(apply func(bool) error) *extensionFlag {
	flag := &extensionFlag{apply: apply, wake: make(chan struct{}, 1)}
	go flag.run()
	return flag
}

// Set records the wanted state; a state the extension could not take is
// tried again with the next call.
func (flag *extensionFlag) Set(on bool) {
	if flag == nil {
		return
	}
	flag.mutex.Lock()
	changed := flag.want != on || flag.shown != on
	flag.want = on
	flag.mutex.Unlock()
	if changed {
		select {
		case flag.wake <- struct{}{}:
		default:
		}
	}
}

func (flag *extensionFlag) run() {
	for range flag.wake {
		flag.mutex.Lock()
		want, shown := flag.want, flag.shown
		flag.mutex.Unlock()
		if want == shown {
			continue
		}
		if err := flag.apply(want); err == nil {
			flag.mutex.Lock()
			flag.shown = want
			flag.mutex.Unlock()
		}
	}
}

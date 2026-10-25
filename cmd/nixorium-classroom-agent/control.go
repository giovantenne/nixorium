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
	// unknown is set when the extension's state is not known to be shown:
	// before the first answer, after a failed call, after a refresh.
	unknown bool
	wake    chan struct{}
}

func newExtensionFlag(apply func(bool) error) *extensionFlag {
	flag := &extensionFlag{apply: apply, wake: make(chan struct{}, 1), unknown: true}
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
	changed := flag.want != on || flag.shown != on || flag.unknown
	flag.want = on
	flag.mutex.Unlock()
	if changed {
		select {
		case flag.wake <- struct{}{}:
		default:
		}
	}
}

// Refresh applies a wanted "on" again: GNOME disables the extension on the
// lock screen and enables it with its flags off, which the agent cannot see.
func (flag *extensionFlag) Refresh() {
	if flag == nil {
		return
	}
	flag.mutex.Lock()
	on := flag.want
	if on {
		flag.unknown = true
	}
	flag.mutex.Unlock()
	if on {
		flag.Set(true)
	}
}

func (flag *extensionFlag) run() {
	for range flag.wake {
		flag.mutex.Lock()
		want, shown, unknown := flag.want, flag.shown, flag.unknown
		flag.mutex.Unlock()
		if want == shown && !unknown {
			continue
		}
		err := flag.apply(want)
		flag.mutex.Lock()
		// A failed call may have been half done: forget what was shown.
		flag.shown, flag.unknown = want, err != nil
		flag.mutex.Unlock()
	}
}

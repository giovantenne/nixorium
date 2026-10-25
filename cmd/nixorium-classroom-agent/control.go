package main

import "sync"

// While the teacher controls the computer, the classroom extension shows a
// banner on the student's screen. The agent asks for it with the first input
// and takes it back when the control ends. It is best effort: an extension
// that does not answer (one from before this notice) never stops the control.

// controlNotifier shows and removes the banner through the extension.
type controlNotifier interface {
	SetControlled(controlled bool) error
}

func (locker *extensionLocker) SetControlled(controlled bool) error {
	return locker.messageCall("SetControlled", controlled)
}

// controlBanner applies the wanted state in the background, so a slow
// session bus never holds the capture; only the latest wish is kept.
type controlBanner struct {
	notifier controlNotifier
	mutex    sync.Mutex
	want     bool
	shown    bool
	wake     chan struct{}
}

func newControlBanner(notifier controlNotifier) *controlBanner {
	banner := &controlBanner{notifier: notifier, wake: make(chan struct{}, 1)}
	go banner.run()
	return banner
}

// Set records whether the teacher controls the computer now; a banner that
// could not be shown is tried again with the next input.
func (banner *controlBanner) Set(controlled bool) {
	if banner == nil {
		return
	}
	banner.mutex.Lock()
	changed := banner.want != controlled || banner.shown != controlled
	banner.want = controlled
	banner.mutex.Unlock()
	if changed {
		select {
		case banner.wake <- struct{}{}:
		default:
		}
	}
}

func (banner *controlBanner) run() {
	for range banner.wake {
		banner.mutex.Lock()
		want, shown := banner.want, banner.shown
		banner.mutex.Unlock()
		if want == shown {
			continue
		}
		if err := banner.notifier.SetControlled(want); err == nil {
			banner.mutex.Lock()
			banner.shown = want
			banner.mutex.Unlock()
		}
	}
}

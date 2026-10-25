package main

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeControlNotifier struct {
	mutex sync.Mutex
	calls []bool
	fail  bool
}

func (notifier *fakeControlNotifier) SetControlled(controlled bool) error {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	if notifier.fail {
		return errors.New("no extension")
	}
	notifier.calls = append(notifier.calls, controlled)
	return nil
}

func (notifier *fakeControlNotifier) seen() []bool {
	notifier.mutex.Lock()
	defer notifier.mutex.Unlock()
	return append([]bool(nil), notifier.calls...)
}

func waitForCalls(t *testing.T, notifier *fakeControlNotifier, want int) []bool {
	t.Helper()
	for attempt := 0; attempt < 200; attempt++ {
		if calls := notifier.seen(); len(calls) >= want {
			return calls
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d calls, saw %v", want, notifier.seen())
	return nil
}

func TestControlBannerFollowsTheControl(t *testing.T) {
	notifier := &fakeControlNotifier{}
	banner := newExtensionFlag(notifier.SetControlled)
	banner.Set(true)
	waitForCalls(t, notifier, 1)
	// Further input changes nothing.
	banner.Set(true)
	banner.Set(true)
	banner.Set(false)
	calls := waitForCalls(t, notifier, 2)
	if len(calls) != 2 || !calls[0] || calls[1] {
		t.Fatalf("calls = %v, want shown then hidden", calls)
	}
	// Ending a control that never showed asks for nothing.
	banner.Set(false)
	time.Sleep(50 * time.Millisecond)
	if len(notifier.seen()) != 2 {
		t.Fatalf("calls = %v", notifier.seen())
	}
}

func TestControlBannerRetriesAfterAFailure(t *testing.T) {
	notifier := &fakeControlNotifier{fail: true}
	banner := newExtensionFlag(notifier.SetControlled)
	banner.Set(true)
	time.Sleep(50 * time.Millisecond)
	notifier.mutex.Lock()
	notifier.fail = false
	notifier.mutex.Unlock()
	banner.Set(true)
	if calls := waitForCalls(t, notifier, 1); !calls[0] {
		t.Fatalf("calls = %v", calls)
	}
}

func TestNoBannerWithoutNotifier(t *testing.T) {
	var banner *extensionFlag
	banner.Set(true)
	banner.Set(false)
}

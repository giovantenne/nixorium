package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// The teacher's screen is shown by the classroom extension from a picture
// in the user's private runtime folder. Without pictures for a while the
// agent ends the showing itself, so a vanished controller never leaves a
// student's screen covered.
const broadcastSilence = 15 * time.Second

type screenShower interface {
	Show(image []byte) error
	Stop()
}

type extensionShower struct {
	mutex   sync.Mutex
	locker  *extensionLocker
	runtime string
	timer   *time.Timer
	frame   int
}

var errBroadcast = errors.New("the classroom extension could not show the picture")

func (shower *extensionShower) call(method string, arguments ...any) error {
	object, err := shower.locker.object()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), lockTimeout)
	defer cancel()
	return object.CallWithContext(ctx, lockBusName+"."+method, 0, arguments...).Err
}

func (shower *extensionShower) Show(image []byte) error {
	shower.mutex.Lock()
	defer shower.mutex.Unlock()
	if !filepath.IsAbs(shower.runtime) {
		return errBroadcast
	}
	// Two names in turn: the extension reads one while the next is written.
	shower.frame = 1 - shower.frame
	name := filepath.Join(shower.runtime, "nixorium-classroom-frame"+string(rune('a'+shower.frame))+".jpg")
	temporary, err := os.CreateTemp(shower.runtime, ".nixorium-classroom-frame-*")
	if err != nil {
		return err
	}
	if _, err := temporary.Write(image); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporary.Name())
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	if err := os.Rename(temporary.Name(), name); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	if err := shower.call("ShowFrame", name); err != nil {
		return errBroadcast
	}
	if shower.timer == nil {
		shower.timer = time.AfterFunc(broadcastSilence, shower.Stop)
	} else {
		shower.timer.Reset(broadcastSilence)
	}
	return nil
}

func (shower *extensionShower) Stop() {
	shower.mutex.Lock()
	defer shower.mutex.Unlock()
	if shower.timer != nil {
		shower.timer.Stop()
		shower.timer = nil
	}
	_ = shower.call("StopBroadcast")
	for _, letter := range []string{"a", "b"} {
		_ = os.Remove(filepath.Join(shower.runtime, "nixorium-classroom-frame"+letter+".jpg"))
	}
}

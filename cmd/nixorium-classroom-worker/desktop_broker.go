package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

// desktopBroker lets the classroom view page send a file or folder chosen
// by the user who opened it. The service cannot read that user's home; a
// small helper, started with the page's browser and running as that user,
// waits on the classroom socket (desktop-wait). When the page asks, the
// broker hands the helper a job; the helper shows the file chooser,
// prepares the choice with share-begin/chunk and reports the transfer
// (desktop-ready). Users are told apart by the kernel's peer credentials,
// never by anything in a request.
type desktopBroker struct {
	mutex   sync.Mutex
	waiting map[int]chan desktopOffer
	jobs    map[string]*desktopJob
}

// desktopOffer is a job handed to a helper: its id and what to choose.
type desktopOffer struct {
	id, kind string
}

type desktopJob struct {
	uid    int
	result chan desktopResult
}

type desktopResult struct {
	transfer  string
	message   string
	cancelled bool
}

const (
	desktopWait = 25 * time.Second
	// The teacher chooses in a dialog, then the files are prepared.
	desktopTimeout = 10 * time.Minute
)

var errNoDesktopHelper = errors.New("Your files are not reachable from this page. Close the classroom view and open it again from Nixorium.")

func newDesktopBroker() *desktopBroker {
	return &desktopBroker{waiting: map[int]chan desktopOffer{}, jobs: map[string]*desktopJob{}}
}

// Wait is the helper's long poll: it returns a job, or none after a while.
func (broker *desktopBroker) Wait(ctx context.Context, uid int) desktopOffer {
	jobs := make(chan desktopOffer, 1)
	broker.mutex.Lock()
	broker.waiting[uid] = jobs
	broker.mutex.Unlock()
	defer func() {
		broker.mutex.Lock()
		if broker.waiting[uid] == jobs {
			delete(broker.waiting, uid)
		}
		broker.mutex.Unlock()
	}()
	timer := time.NewTimer(desktopWait)
	defer timer.Stop()
	select {
	case job := <-jobs:
		return job
	case <-timer.C:
	case <-ctx.Done():
	}
	return desktopOffer{}
}

// Request asks the helper of uid to choose a file or folder (kind) and
// waits for the prepared transfer; cancelled reports a closed chooser.
func (broker *desktopBroker) Request(ctx context.Context, uid int, kind string) (transfer string, cancelled bool, err error) {
	id, err := randomToken()
	if err != nil {
		return "", false, err
	}
	job := &desktopJob{uid: uid, result: make(chan desktopResult, 1)}
	// A helper between two waits gets a moment to come back.
	var helper chan desktopOffer
	for attempt := 0; attempt < 20 && helper == nil; attempt++ {
		broker.mutex.Lock()
		helper = broker.waiting[uid]
		if helper != nil {
			delete(broker.waiting, uid)
			broker.jobs[id] = job
		}
		broker.mutex.Unlock()
		if helper == nil {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if helper == nil {
		return "", false, errNoDesktopHelper
	}
	defer func() {
		broker.mutex.Lock()
		delete(broker.jobs, id)
		broker.mutex.Unlock()
	}()
	helper <- desktopOffer{id: id, kind: kind}
	timer := time.NewTimer(desktopTimeout)
	defer timer.Stop()
	select {
	case result := <-job.result:
		if result.cancelled {
			return "", true, nil
		}
		if result.transfer == "" {
			return "", false, errors.New(result.message)
		}
		return result.transfer, false, nil
	case <-timer.C:
		return "", false, errors.New("The files took too long to prepare.")
	case <-ctx.Done():
		return "", false, ctx.Err()
	}
}

// Ready records the helper's answer; only the job's own user may answer.
func (broker *desktopBroker) Ready(uid int, id, transfer, message string, cancelled bool) bool {
	broker.mutex.Lock()
	job, found := broker.jobs[id]
	broker.mutex.Unlock()
	if !found || job.uid != uid {
		return false
	}
	if transfer == "" && message == "" {
		message = "The files could not be prepared."
	}
	select {
	case job.result <- desktopResult{transfer: transfer, message: message, cancelled: cancelled}:
	default:
	}
	return true
}

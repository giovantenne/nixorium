package main

import (
	"context"
	"errors"
	"sync"
	"time"
)

// desktopBroker lets the classroom view page send the desktop of the user
// who opened it. The service cannot read that user's home; a small helper,
// started with the page's browser and running as that user, waits on the
// classroom socket (desktop-wait). When the page asks, the broker hands the
// helper a job; the helper prepares the desktop with share-begin/chunk and
// reports the transfer (desktop-ready). Users are told apart by the
// kernel's peer credentials, never by anything in a request.
type desktopBroker struct {
	mutex   sync.Mutex
	waiting map[int]chan string
	jobs    map[string]*desktopJob
}

type desktopJob struct {
	uid    int
	result chan desktopResult
}

type desktopResult struct {
	transfer string
	message  string
}

const (
	desktopWait    = 25 * time.Second
	desktopTimeout = 3 * time.Minute
)

var errNoDesktopHelper = errors.New("Your desktop is not reachable from this page. Close the classroom view and open it again from Nixorium.")

func newDesktopBroker() *desktopBroker {
	return &desktopBroker{waiting: map[int]chan string{}, jobs: map[string]*desktopJob{}}
}

// Wait is the helper's long poll: it returns a job, or "" after a while.
func (broker *desktopBroker) Wait(ctx context.Context, uid int) string {
	jobs := make(chan string, 1)
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
	return ""
}

// Request asks the helper of uid for its desktop and waits for the
// prepared transfer.
func (broker *desktopBroker) Request(ctx context.Context, uid int) (string, error) {
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	job := &desktopJob{uid: uid, result: make(chan desktopResult, 1)}
	// A helper between two waits gets a moment to come back.
	var helper chan string
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
		return "", errNoDesktopHelper
	}
	defer func() {
		broker.mutex.Lock()
		delete(broker.jobs, id)
		broker.mutex.Unlock()
	}()
	helper <- id
	timer := time.NewTimer(desktopTimeout)
	defer timer.Stop()
	select {
	case result := <-job.result:
		if result.transfer == "" {
			return "", errors.New(result.message)
		}
		return result.transfer, nil
	case <-timer.C:
		return "", errors.New("Your desktop took too long to prepare.")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// Ready records the helper's answer; only the job's own user may answer.
func (broker *desktopBroker) Ready(uid int, id, transfer, message string) bool {
	broker.mutex.Lock()
	job, found := broker.jobs[id]
	broker.mutex.Unlock()
	if !found || job.uid != uid {
		return false
	}
	if transfer == "" && message == "" {
		message = "Your desktop could not be prepared."
	}
	select {
	case job.result <- desktopResult{transfer: transfer, message: message}:
	default:
	}
	return true
}

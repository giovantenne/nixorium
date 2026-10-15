package main

import (
	"context"
	"errors"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Saves go the other way from Send files: the service holds files (the
// screenshots it took, or the desktops it collected) and a process of the
// user they are for copies them into that user's home, piece by piece
// over the classroom socket. For the page, that process is the desktop
// helper, which gets a save job through desktop-wait; for the terminal it
// is the command itself. Only the save's own user (the socket's kernel
// peer credentials) may list, read or end it.
type desktopSave struct {
	uid   int
	kind  string
	items []domain.ShareFile
	read  func(index int, offset int64, size int) ([]byte, error)
	// release frees what the save holds (temporary files) once.
	release func()
	used    time.Time
	// done receives the outcome while a page waits for the helper.
	done chan saveOutcome
}

// saveOutcome is what the user's process reports: the names it saved, or
// why it could not.
type saveOutcome struct {
	names   []string
	message string
}

const (
	// saveIdle removes a save nobody read for half an hour.
	saveIdle = 30 * time.Minute
	// saveLimit bounds the saves held at once.
	saveLimit = 8
	// helperSeen: a helper that waited this recently is busy with a job.
	helperSeen = 2 * time.Minute
)

var errSaveMissing = errors.New("these files are no longer available")

// Offer keeps a save for the user uid and returns its identifier.
func (broker *desktopBroker) Offer(uid int, kind string, items []domain.ShareFile, read func(int, int64, int) ([]byte, error), release func()) (string, error) {
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	broker.pruneSaves()
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	if broker.saves == nil {
		broker.saves = map[string]*desktopSave{}
	}
	if len(broker.saves) >= saveLimit {
		return "", errors.New("other files are being saved; try again in a few minutes")
	}
	broker.saves[id] = &desktopSave{uid: uid, kind: kind, items: items, read: read, release: release, used: time.Now()}
	return id, nil
}

// Available tells whether the user uid has a desktop helper: one waiting
// for a job now, or busy with one since it last waited.
func (broker *desktopBroker) Available(uid int) bool {
	for attempt := 0; attempt < 20; attempt++ {
		broker.mutex.Lock()
		waiting := broker.waiting[uid] != nil
		seen := time.Since(broker.seen[uid]) < helperSeen
		broker.mutex.Unlock()
		if waiting || seen {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// Deliver hands a save to the desktop helper of its user, waiting at most
// wait for the helper to be free, and then at most timeout for the outcome.
// Without a helper it returns errNoDesktopHelper and the save stays
// available to its user.
func (broker *desktopBroker) Deliver(ctx context.Context, id string, wait, timeout time.Duration) (saveOutcome, error) {
	broker.mutex.Lock()
	save, found := broker.saves[id]
	broker.mutex.Unlock()
	if !found {
		return saveOutcome{}, errSaveMissing
	}
	var helper chan desktopOffer
	for deadline := time.Now().Add(wait); helper == nil && time.Now().Before(deadline); {
		broker.mutex.Lock()
		helper = broker.waiting[save.uid]
		if helper != nil {
			delete(broker.waiting, save.uid)
			save.done = make(chan saveOutcome, 1)
		}
		broker.mutex.Unlock()
		if helper == nil {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if helper == nil {
		return saveOutcome{}, errNoDesktopHelper
	}
	helper <- desktopOffer{id: id, kind: save.kind}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case outcome := <-save.done:
		return outcome, nil
	case <-timer.C:
		return saveOutcome{}, errors.New("saving the files took too long")
	case <-ctx.Done():
		return saveOutcome{}, ctx.Err()
	}
}

// save returns a save of the user uid and marks it used.
func (broker *desktopBroker) save(uid int, id string) (*desktopSave, error) {
	broker.mutex.Lock()
	defer broker.mutex.Unlock()
	save, found := broker.saves[id]
	if !found || save.uid != uid {
		return nil, errSaveMissing
	}
	save.used = time.Now()
	return save, nil
}

// SaveItems returns one page of a save's items from start, and how many
// there are in all.
func (broker *desktopBroker) SaveItems(uid int, id string, start int) (string, []domain.ShareFile, int, error) {
	save, err := broker.save(uid, id)
	if err != nil {
		return "", nil, 0, err
	}
	if start < 0 || start > len(save.items) {
		return "", nil, 0, errors.New("no such item")
	}
	page, names := []domain.ShareFile{}, 0
	for index := start; index < len(save.items) && len(page) < domain.ClassroomSaveItemsPage; index++ {
		names += len(save.items[index].Path)
		if len(page) > 0 && names > domain.ClassroomSaveNameBytes {
			break
		}
		page = append(page, save.items[index])
	}
	return save.kind, page, len(save.items), nil
}

// SaveRead returns one piece of a file of a save.
func (broker *desktopBroker) SaveRead(uid int, id string, index int, offset int64) ([]byte, error) {
	save, err := broker.save(uid, id)
	if err != nil {
		return nil, err
	}
	if index < 0 || index >= len(save.items) || save.items[index].Dir || offset < 0 || offset >= save.items[index].Size {
		return nil, errors.New("no such piece of a file")
	}
	return save.read(index, offset, int(min(int64(domain.ClassroomShareChunkBytes), save.items[index].Size-offset)))
}

// SaveDone ends a save with what its user's process reports.
func (broker *desktopBroker) SaveDone(uid int, id string, names []string, message string) bool {
	broker.mutex.Lock()
	save, found := broker.saves[id]
	owned := found && save.uid == uid
	var done chan saveOutcome
	if owned {
		delete(broker.saves, id)
		done = save.done
	}
	broker.mutex.Unlock()
	if !owned {
		return false
	}
	if len(names) == 0 && message == "" {
		message = "The files could not be saved."
	}
	// The service's copies are gone before anybody hears the outcome.
	save.release()
	if done != nil {
		done <- saveOutcome{names: names, message: message}
	}
	return true
}

// Withdraw forgets a save, for example when no helper took it.
func (broker *desktopBroker) Withdraw(id string) {
	broker.mutex.Lock()
	save, found := broker.saves[id]
	delete(broker.saves, id)
	broker.mutex.Unlock()
	if found {
		save.release()
	}
}

// pruneSaves forgets saves nobody read for saveIdle.
func (broker *desktopBroker) pruneSaves() {
	broker.mutex.Lock()
	expired := []*desktopSave{}
	for id, save := range broker.saves {
		if time.Since(save.used) > saveIdle {
			expired = append(expired, save)
			delete(broker.saves, id)
		}
	}
	broker.mutex.Unlock()
	for _, save := range expired {
		save.release()
	}
}

// expireSaves prunes idle saves until ctx ends.
func (broker *desktopBroker) expireSaves(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			broker.pruneSaves()
		}
	}
}

// handleSave answers a user's process copying a save; only the save's own
// user, told apart by the classroom socket's kernel peer credentials, may.
func (worker classroomWorker) handleSave(ctx context.Context, request domain.ClassroomRequest) domain.ClassroomResponse {
	response := domain.ClassroomResponse{State: "completed"}
	failed := func(message string) domain.ClassroomResponse {
		return domain.ClassroomResponse{State: "failed", Message: message}
	}
	uid, known := adapters.ClassroomPeerUID(ctx)
	if !known || worker.view == nil || worker.view.desktops == nil {
		return failed("Saving files is not available.")
	}
	broker := worker.view.desktops
	switch request.Operation {
	case domain.ClassroomSaveItemsOperation:
		kind, items, total, err := broker.SaveItems(uid, request.DesktopJob, request.ShareIndex)
		if err != nil {
			return failed("The files to save are not available: " + err.Error() + ".")
		}
		response.DesktopKind, response.SaveItems, response.SaveTotal = kind, items, total
	case domain.ClassroomSaveReadOperation:
		data, err := broker.SaveRead(uid, request.DesktopJob, request.ShareIndex, request.ShareOffset)
		if err != nil {
			return failed("The files to save are not available: " + err.Error() + ".")
		}
		response.SaveData = data
	case domain.ClassroomSaveDoneOperation:
		if !broker.SaveDone(uid, request.DesktopJob, request.SaveNames, request.DesktopError) {
			return failed("These files are no longer available.")
		}
	default:
		return failed("Saving files is not available.")
	}
	return response
}

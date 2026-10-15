package main

import (
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
)

// Exam mode is the page's chain of three existing reviewed actions. The
// worker remembers, in memory only, which steps it applied for exam mode on
// each computer and when, so that a page opened later, or another user's
// page, shows the computers in exam mode and can end it. The memory follows
// only what this worker applied: a step counts once a computer's row of an
// apply tagged for exam mode came back done. The computers keep no exam
// state, and a restart of the worker forgets it.

// Kinds of an exam-tagged action: the step it starts or ends.
const (
	examStart = "start"
	examEnd   = "end"
)

// Steps, named as the page names them, in the order exam mode starts them.
const (
	examInternet = "internet"
	examLock     = "lock"
	examMessage  = "message"
)

var examStepOrder = []string{examInternet, examLock, examMessage}

var examStartActions = map[string]string{actionInternetBlock: examInternet, actionLock: examLock, actionMessage: examMessage}
var examEndActions = map[string]string{actionInternetAllow: examInternet, actionUnlock: examLock, actionMessageHide: examMessage}

// examStepOf is the step an action takes for a kind of exam-tagged request,
// and whether the combination exists at all.
func examStepOf(kind, action string) (string, bool) {
	switch kind {
	case examStart:
		step, found := examStartActions[action]
		return step, found
	case examEnd:
		step, found := examEndActions[action]
		return step, found
	}
	return "", false
}

// examView is what the page reads of a computer in exam mode.
type examView struct {
	// Steps lists the steps still in effect: internet, lock, message.
	Steps []string `json:"steps"`
	// Since is when exam mode started on the computer, in Unix milliseconds.
	Since int64 `json:"since"`
}

type examRecord struct {
	steps map[string]bool
	since time.Time
}

// examBook is the memory of every computer in exam mode. The zero value is
// ready to use.
type examBook struct {
	mutex   sync.Mutex
	records map[string]*examRecord
}

// record notes the computers whose step of an exam-tagged action was done:
// starting adds the step, ending removes it. A computer whose steps are all
// ended is no longer in exam mode.
func (book *examBook) record(kind, step string, rows []actionRow, now time.Time) {
	book.mutex.Lock()
	defer book.mutex.Unlock()
	for _, row := range rows {
		if row.Outcome != outcomeDone {
			continue
		}
		record := book.records[row.Name]
		if kind == examEnd {
			if record != nil {
				delete(record.steps, step)
			}
		} else {
			if record == nil {
				if book.records == nil {
					book.records = map[string]*examRecord{}
				}
				record = &examRecord{steps: map[string]bool{}, since: now}
				book.records[row.Name] = record
			}
			record.steps[step] = true
		}
		if record != nil && len(record.steps) == 0 {
			delete(book.records, row.Name)
		}
	}
}

// view returns a computer's exam mode as it still stands. What the worker
// sees of the computer ends steps by itself: when nobody is signed in, the
// lock and the message are gone, and when the computer's Internet reads as
// allowed again, so is that step. internet is the computer's last known
// Internet state ("blocked", "enabled" or empty when not known).
func (book *examBook) view(name, state, internet string) *examView {
	book.mutex.Lock()
	defer book.mutex.Unlock()
	record := book.records[name]
	if record == nil {
		return nil
	}
	if state == app.ClassroomNoSession {
		delete(record.steps, examLock)
		delete(record.steps, examMessage)
	}
	if internet == "enabled" {
		delete(record.steps, examInternet)
	}
	if len(record.steps) == 0 {
		delete(book.records, name)
		return nil
	}
	view := &examView{Steps: []string{}, Since: record.since.UnixMilli()}
	for _, step := range examStepOrder {
		if record.steps[step] {
			view.Steps = append(view.Steps, step)
		}
	}
	return view
}

// forget drops a computer: a restart or a shutdown ends every step.
func (book *examBook) forget(name string) {
	book.mutex.Lock()
	defer book.mutex.Unlock()
	delete(book.records, name)
}

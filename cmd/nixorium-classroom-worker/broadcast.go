package main

import (
	"context"
	"errors"
	"sync"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

// broadcaster runs at most one showing of the teacher's screen; only the
// classroom view page starts it.
type broadcaster struct {
	manager    *app.BroadcastManager
	repository string
	viewOn     func() bool
	mutex      sync.Mutex
	id         string
	current    *app.Broadcast
}

var errNoBroadcast = errors.New("the showing has ended")

func (control *broadcaster) Plan(ctx context.Context, requested string) domain.BroadcastPlan {
	if control.viewOn != nil && !control.viewOn() {
		message := errViewDisabled.Error()
		return domain.BroadcastPlan{SchemaVersion: domain.SchemaVersion, Operation: "broadcast-plan", State: "blocked", Message: message, Targets: []domain.LockTarget{}, Issues: []domain.ValidationIssue{{Field: "classroom", Message: message}}}
	}
	plan := control.manager.Plan(ctx, control.repository, requested)
	plan.Message, plan.Issues = teacherMessage(plan.Message), teacherIssues(plan.Issues)
	return plan
}

// Start replaces any running showing.
func (control *broadcaster) Start(ctx context.Context, plan domain.BroadcastPlan) (string, error) {
	if plan.Repository != control.repository {
		return "", errors.New("the review does not belong to this laboratory")
	}
	broadcast, err := control.manager.Start(ctx, plan, plan.ReviewToken)
	if err != nil {
		return "", err
	}
	id, err := randomToken()
	if err != nil {
		broadcast.Stop()
		return "", err
	}
	control.mutex.Lock()
	previous := control.current
	control.id, control.current = id, broadcast
	control.mutex.Unlock()
	if previous != nil {
		previous.Stop()
	}
	return id, nil
}

func (control *broadcaster) running(id string) *app.Broadcast {
	control.mutex.Lock()
	defer control.mutex.Unlock()
	if control.current == nil || id == "" || id != control.id || control.current.Stopped() {
		return nil
	}
	return control.current
}

func (control *broadcaster) Frame(id string, image []byte) error {
	broadcast := control.running(id)
	if broadcast == nil {
		return errNoBroadcast
	}
	return broadcast.Frame(image)
}

func (control *broadcaster) Stop(id string) {
	if broadcast := control.running(id); broadcast != nil {
		broadcast.Stop()
	}
}

// Showing names the computers that show the teacher's screen now.
func (control *broadcaster) Showing() map[string]bool {
	control.mutex.Lock()
	broadcast := control.current
	control.mutex.Unlock()
	showing := map[string]bool{}
	if broadcast == nil || broadcast.Stopped() {
		return showing
	}
	for name, state := range broadcast.States() {
		showing[name] = state == "showing"
	}
	return showing
}

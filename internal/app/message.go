package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// MessageManager shows a message over the screens of students' computers,
// and removes it. Like a lock it changes only the students' sessions, so it
// does not take the administrative operation lock.
type MessageManager struct {
	source LockSource
	now    func() time.Time
}

func NewMessageManager(source LockSource) *MessageManager {
	return &MessageManager{source, time.Now}
}

// messageOutdated explains a computer whose classroom view cannot show
// messages: its agent, or the Shell extension of the student's session, is
// older than this controller.
const messageOutdated = "The classroom view on this computer cannot show messages yet; update the computer and sign in again."

// Plan reviews the computers that can show (or lose) a message. Which
// computers qualify is the lock's question: someone signed in with the
// classroom view. The text and duration are bound into the review token.
func (m *MessageManager) Plan(ctx context.Context, repository, requested string, action domain.MessageAction, content domain.MessageContent) domain.MessagePlan {
	p := domain.MessagePlan{SchemaVersion: domain.SchemaVersion, Operation: "message-plan", State: "blocked", Action: action, Requested: requested, Targets: []domain.LockTarget{}, Issues: []domain.ValidationIssue{}}
	fail := func(message string) domain.MessagePlan {
		p.Message = message
		p.Issues = append(p.Issues, domain.ValidationIssue{Field: "message", Message: message})
		return p
	}
	if !action.Valid() {
		return fail("Choose show or remove.")
	}
	if action == domain.MessageShow {
		content.Text = classroomview.TrimMessage(content.Text)
		if err := classroomview.ValidateMessage(content.Text, content.Seconds, content.UntilUnlock); err != nil {
			return fail(err.Error())
		}
		p.Content = content
	}
	lock := (&LockManager{source: m.source, now: m.now}).Plan(ctx, repository, requested, domain.LockOn)
	p.Repository, p.Requested, p.Targets, p.ExpiresAt = lock.Repository, lock.Requested, lock.Targets, lock.ExpiresAt
	if lock.HasErrors() {
		p.Issues = lock.Issues
		p.Message = lock.Message
		return p
	}
	eligible := 0
	for _, target := range p.Targets {
		if target.Eligible {
			eligible++
		}
	}
	p.State = "ready"
	if action == domain.MessageHide {
		p.Message = fmt.Sprintf("The message is removed from %d of %d computers.", eligible, len(p.Targets))
	} else {
		p.Message = fmt.Sprintf("This is shown on %d of %d computers, above everything, a lock included. %s Keyboard and mouse stay with the student unless the computer is locked.", eligible, len(p.Targets), messageDuration(content))
	}
	p.ReviewToken = domain.MessageReviewToken(p)
	return p
}

// messageDuration words how long a message stays, for the review.
func messageDuration(content domain.MessageContent) string {
	switch {
	case content.UntilUnlock:
		return "It goes away when the computer is unlocked; on a computer that is not locked the student can close it."
	case content.Seconds >= 120 && content.Seconds%60 == 0:
		return fmt.Sprintf("It goes away by itself after %d minutes.", content.Seconds/60)
	case content.Seconds == 60:
		return "It goes away by itself after 1 minute."
	case content.Seconds > 0:
		return fmt.Sprintf("It goes away by itself after %d seconds.", content.Seconds)
	}
	return fmt.Sprintf("A student can close it after %d seconds.", classroomview.MessageCloseSeconds)
}

func (m *MessageManager) Apply(ctx context.Context, p domain.MessagePlan, token string) domain.MessageReport {
	r := domain.MessageReport{SchemaVersion: domain.SchemaVersion, Operation: "message-apply", State: "blocked", Action: p.Action, Targets: []domain.MessageOutcome{}}
	if p.HasErrors() || !p.Action.Valid() || token == "" || token != p.ReviewToken || token != domain.MessageReviewToken(p) || !m.now().Before(p.ExpiresAt) {
		r.Message = "Review expired or changed; create a fresh plan."
		return r
	}
	// The text is checked again: the review token binds it, but a plan is
	// data from outside this process.
	if p.Action == domain.MessageShow {
		if err := classroomview.ValidateMessage(p.Content.Text, p.Content.Seconds, p.Content.UntilUnlock); err != nil {
			r.Message = err.Error()
			return r
		}
	}
	meta, err := m.source.LabMeta(ctx, p.Repository)
	if err != nil {
		r.Message = err.Error()
		return r
	}
	identities := map[string]string{}
	for _, host := range meta.Clients.Hosts {
		identities[host.Name] = host.IP
	}
	hosts := make([]domain.HostMeta, len(p.Targets))
	for index, target := range p.Targets {
		if target.Name == meta.Controller.Name || identities[target.Name] != target.IP {
			r.Message = "Client inventory changed; review again."
			return r
		}
		hosts[index] = target.HostMeta
	}
	outcomes := make([]domain.MessageOutcome, len(hosts))
	eachHost(hosts, lockParallel, func(index int, host domain.HostMeta) {
		outcome := domain.MessageOutcome{Name: host.Name, State: domain.MessageNotSent, Detail: "Unavailable in the reviewed plan; nothing was sent."}
		if p.Targets[index].Eligible {
			outcome = m.deliver(ctx, host, p)
		}
		outcomes[index] = outcome
	})
	done := 0
	for _, outcome := range outcomes {
		if outcome.State == domain.MessageDone {
			done++
		}
	}
	r.Targets = outcomes
	r.State = "partial"
	if done == len(outcomes) {
		r.State = "completed"
	}
	verb := "Showed the message on"
	if p.Action == domain.MessageHide {
		verb = "Removed the message from"
	}
	r.Message = fmt.Sprintf("%s %d of %d selected computers.", verb, done, len(outcomes))
	return r
}

func (m *MessageManager) deliver(ctx context.Context, host domain.HostMeta, p domain.MessagePlan) domain.MessageOutcome {
	outcome := domain.MessageOutcome{Name: host.Name, State: domain.MessageNotSent}
	session, err := m.source.Connect(ctx, host)
	if err != nil {
		outcome.Detail = lockUnavailable(err)
		return outcome
	}
	defer session.Close()
	outcome.State = domain.MessageFailed
	messenger, ok := session.(classroomview.Messenger)
	if !ok {
		outcome.Detail = messageOutdated
		return outcome
	}
	if p.Action == domain.MessageHide {
		err = messenger.HideMessage()
	} else {
		err = messenger.ShowMessage(p.Content.Text, p.Content.Seconds, p.Content.UntilUnlock)
	}
	var agentError classroomview.AgentError
	switch {
	case err == nil:
		outcome.State, outcome.Detail = domain.MessageDone, "Message shown."
		if p.Action == domain.MessageHide {
			outcome.Detail = "Message removed."
		}
	case errors.As(err, &agentError) && (agentError.Code == classroomview.CodeUnsupported || agentError.Code == classroomview.CodeMessageUnavailable):
		outcome.Detail = messageOutdated
	case errors.As(err, &agentError) && (agentError.Code == classroomview.CodeNoSession || agentError.Code == classroomview.CodeNoAgent):
		outcome.State, outcome.Detail = domain.MessageNotSent, lockUnavailable(err)
	case errors.As(err, &agentError):
		outcome.Detail = "The computer did not accept the message."
	default:
		outcome.Detail = "The computer did not confirm; check it before trying again."
	}
	return outcome
}

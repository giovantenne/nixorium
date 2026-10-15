package app

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// fakeMessageSource is the lock lab whose sessions also show messages,
// except for the computers listed in plain (an agent that predates them).
type fakeMessageSource struct {
	*fakeLockSource
	mutex  sync.Mutex
	calls  []string
	errors map[string]error
	plain  map[string]bool
}

type fakeMessenger struct {
	fakeLockSession
	source *fakeMessageSource
}

func (source *fakeMessageSource) Connect(ctx context.Context, host domain.HostMeta) (classroomview.Session, error) {
	session, err := source.fakeLockSource.Connect(ctx, host)
	if err != nil || source.plain[host.Name] {
		return session, err
	}
	return fakeMessenger{session.(fakeLockSession), source}, nil
}

func (session fakeMessenger) record(call string) error {
	session.source.mutex.Lock()
	defer session.source.mutex.Unlock()
	session.source.calls = append(session.source.calls, session.name+" "+call)
	return session.source.errors[session.name]
}

func (session fakeMessenger) ShowMessage(text string, seconds int, untilUnlock bool) error {
	return session.record("show " + strings.ReplaceAll(text, "\n", "|") + " " + strconv.Itoa(seconds) + "s " + map[bool]string{true: "unlock", false: "-"}[untilUnlock])
}

func (session fakeMessenger) HideMessage() error { return session.record("hide") }

func messageLab() *fakeMessageSource {
	return &fakeMessageSource{fakeLockSource: lockLab(), errors: map[string]error{}, plain: map[string]bool{}}
}

func TestMessagePlanAndApply(t *testing.T) {
	source := messageLab()
	source.away["pc02"] = classroomview.ErrUnreachable
	source.plain["pc03"] = true
	manager := NewMessageManager(source)
	content := domain.MessageContent{Text: "\n Exam in progress.\nWait for the teacher. \n", Seconds: 30}
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01,pc02,pc03", domain.MessageShow, content)
	if plan.HasErrors() || plan.ReviewToken == "" || len(plan.Targets) != 3 || !plan.Targets[0].Eligible || plan.Targets[1].Eligible || !plan.Targets[2].Eligible {
		t.Fatalf("plan = %+v", plan)
	}
	// The text is trimmed, bound into the token, and kept out of the
	// controller's own sentence.
	if plan.Content.Text != "Exam in progress.\nWait for the teacher." || strings.Contains(plan.Message, "Exam") {
		t.Fatalf("plan content = %+v, message = %q", plan.Content, plan.Message)
	}
	if !strings.Contains(plan.Message, "2 of 3 computers") || !strings.Contains(plan.Message, "It goes away by itself after 30 seconds.") || !strings.Contains(plan.Message, "unless the computer is locked") {
		t.Fatalf("plan message = %q", plan.Message)
	}
	report := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if report.State != "partial" || len(report.Targets) != 3 {
		t.Fatalf("report = %+v", report)
	}
	if got := report.Targets[0]; got.State != domain.MessageDone || got.Detail != "Message shown." {
		t.Fatalf("pc01 = %+v", got)
	}
	if got := report.Targets[1]; got.State != domain.MessageNotSent || !strings.Contains(got.Detail, "Unavailable in the reviewed plan") {
		t.Fatalf("pc02 = %+v", got)
	}
	if got := report.Targets[2]; got.State != domain.MessageFailed || !strings.Contains(got.Detail, "cannot show messages yet") {
		t.Fatalf("pc03 = %+v", got)
	}
	if want := []string{"pc01 show Exam in progress.|Wait for the teacher. 30s -"}; strings.Join(source.calls, ";") != strings.Join(want, ";") {
		t.Fatalf("calls = %q", source.calls)
	}
}

func TestMessageHideNeedsNoText(t *testing.T) {
	source := messageLab()
	manager := NewMessageManager(source)
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01,pc03", domain.MessageHide, domain.MessageContent{})
	if plan.HasErrors() || plan.Content.Text != "" || !strings.Contains(plan.Message, "removed from 2 of 2 computers") {
		t.Fatalf("plan = %+v", plan)
	}
	report := manager.Apply(context.Background(), plan, plan.ReviewToken)
	sort.Strings(source.calls)
	if report.State != "completed" || report.Targets[0].Detail != "Message removed." || strings.Join(source.calls, ";") != "pc01 hide;pc03 hide" {
		t.Fatalf("report = %+v, calls = %q", report, source.calls)
	}
}

func TestMessagePlanRefusesWhatCannotBeShownBeforeContactingAComputer(t *testing.T) {
	for name, content := range map[string]domain.MessageContent{
		"empty":              {Text: " \n "},
		"too long":           {Text: strings.Repeat("x", classroomview.MaxMessageRunes+1)},
		"control":            {Text: "a\tb"},
		"too many lines":     {Text: strings.Repeat("x\n", classroomview.MaxMessageLines) + "x"},
		"negative":           {Text: "Wait", Seconds: -5},
		"two durations":      {Text: "Wait", Seconds: 30, UntilUnlock: true},
		"beyond ten minutes": {Text: "Wait", Seconds: classroomview.MaxMessageSeconds + 1},
	} {
		source := messageLab()
		plan := NewMessageManager(source).Plan(context.Background(), "/srv/lab", "pc01", domain.MessageShow, content)
		if !plan.HasErrors() || plan.ReviewToken != "" || plan.Message == "" || source.connect != 0 {
			t.Errorf("%s: plan = %+v, %d connections", name, plan, source.connect)
		}
	}
	for name, action := range map[string]domain.MessageAction{"unknown action": "freeze", "no action": ""} {
		if plan := NewMessageManager(messageLab()).Plan(context.Background(), "/srv/lab", "pc01", action, domain.MessageContent{Text: "Wait"}); !plan.HasErrors() {
			t.Errorf("%s: plan = %+v", name, plan)
		}
	}
	if plan := NewMessageManager(messageLab()).Plan(context.Background(), "/srv/lab", "pc99", domain.MessageShow, domain.MessageContent{Text: "Wait"}); !plan.HasErrors() {
		t.Errorf("the controller was accepted: %+v", plan)
	}
}

func TestMessageApplyRefusesStaleOrChangedReviews(t *testing.T) {
	source := messageLab()
	manager := NewMessageManager(source)
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01", domain.MessageShow, domain.MessageContent{Text: "Exam in progress."})
	otherText, otherDuration, otherAction := plan, plan, plan
	otherText.Content.Text = "Free for all!"
	otherDuration.Content.UntilUnlock = true
	otherAction.Action = domain.MessageHide
	for name, report := range map[string]domain.MessageReport{
		"wrong token":    manager.Apply(context.Background(), plan, "sha256:other"),
		"no token":       manager.Apply(context.Background(), plan, ""),
		"changed text":   manager.Apply(context.Background(), otherText, plan.ReviewToken),
		"changed time":   manager.Apply(context.Background(), otherDuration, plan.ReviewToken),
		"changed action": manager.Apply(context.Background(), otherAction, plan.ReviewToken),
	} {
		if report.State != "blocked" || !strings.Contains(report.Message, "expired or changed") {
			t.Fatalf("%s = %+v", name, report)
		}
	}
	// A text the extension must not show is refused even under a valid token.
	forged := plan
	forged.Content.Text = "a\tb"
	forged.ReviewToken = domain.MessageReviewToken(forged)
	if report := manager.Apply(context.Background(), forged, forged.ReviewToken); report.State != "blocked" || !strings.Contains(report.Message, "control characters") {
		t.Fatalf("forged text = %+v", report)
	}
	manager.now = func() time.Time { return time.Now().Add(6 * time.Minute) }
	if report := manager.Apply(context.Background(), plan, plan.ReviewToken); report.State != "blocked" || !strings.Contains(report.Message, "expired") {
		t.Fatalf("expired = %+v", report)
	}
	manager.now = time.Now
	source.meta.Clients.Hosts[0].IP = "10.0.0.9"
	if report := manager.Apply(context.Background(), plan, plan.ReviewToken); report.State != "blocked" || !strings.Contains(report.Message, "inventory") {
		t.Fatalf("inventory = %+v", report)
	}
	if len(source.calls) != 0 {
		t.Fatalf("a refused review sent %q", source.calls)
	}
}

func TestMessageOutcomesSayWhatWentWrong(t *testing.T) {
	for name, test := range map[string]struct {
		err   error
		state string
		text  string
	}{
		"an older agent":     {classroomview.AgentError{Code: classroomview.CodeUnsupported}, domain.MessageFailed, "cannot show messages yet"},
		"an older extension": {classroomview.AgentError{Code: classroomview.CodeMessageUnavailable}, domain.MessageFailed, "sign in again"},
		"nobody signed in":   {classroomview.AgentError{Code: classroomview.CodeNoSession}, domain.MessageNotSent, "Nobody is signed in."},
		"a refusal":          {classroomview.AgentError{Code: classroomview.CodeBadRequest}, domain.MessageFailed, "did not accept"},
		"no answer":          {errors.New("the classroom agent did not answer in time"), domain.MessageFailed, "did not confirm"},
	} {
		source := messageLab()
		source.errors["pc01"] = test.err
		manager := NewMessageManager(source)
		plan := manager.Plan(context.Background(), "/srv/lab", "pc01", domain.MessageShow, domain.MessageContent{Text: "Wait"})
		report := manager.Apply(context.Background(), plan, plan.ReviewToken)
		if got := report.Targets[0]; got.State != test.state || !strings.Contains(got.Detail, test.text) || report.State != "partial" {
			t.Errorf("%s: %+v in %+v", name, got, report)
		}
	}
}

func TestMessageSaysWhenMonitoringStoppedOnAComputer(t *testing.T) {
	// At the review, the computer's agent does not answer: it is listed with
	// the words of the lock and skipped.
	source := messageLab()
	source.away["pc02"] = classroomview.AgentError{Code: classroomview.CodeNoAgent}
	manager := NewMessageManager(source)
	plan := manager.Plan(context.Background(), "/srv/lab", "pc01,pc02", domain.MessageShow, domain.MessageContent{Text: "Wait"})
	if plan.HasErrors() || plan.Targets[1].Eligible || plan.Targets[1].Detail != ClassroomNoAgentDetail {
		t.Fatalf("plan = %+v", plan)
	}
	report := manager.Apply(context.Background(), plan, plan.ReviewToken)
	if got := report.Targets[1]; got.State != domain.MessageNotSent || !strings.Contains(got.Detail, "Unavailable in the reviewed plan") {
		t.Fatalf("pc02 = %+v", got)
	}
	// Between the review and the apply the agent stopped answering: the
	// computer is not sent anything and says why with the same words.
	source = messageLab()
	manager = NewMessageManager(source)
	plan = manager.Plan(context.Background(), "/srv/lab", "pc01", domain.MessageShow, domain.MessageContent{Text: "Wait"})
	source.away["pc01"] = classroomview.AgentError{Code: classroomview.CodeNoAgent}
	report = manager.Apply(context.Background(), plan, plan.ReviewToken)
	if got := report.Targets[0]; got.State != domain.MessageNotSent || got.Detail != ClassroomNoAgentDetail {
		t.Fatalf("pc01 = %+v", got)
	}
	// An agent that answers hello and then loses its relay mid-message.
	source = messageLab()
	source.errors["pc01"] = classroomview.AgentError{Code: classroomview.CodeNoAgent}
	manager = NewMessageManager(source)
	plan = manager.Plan(context.Background(), "/srv/lab", "pc01", domain.MessageShow, domain.MessageContent{Text: "Wait"})
	if report = manager.Apply(context.Background(), plan, plan.ReviewToken); report.Targets[0].State != domain.MessageNotSent || report.Targets[0].Detail != ClassroomNoAgentDetail {
		t.Fatalf("pc01 = %+v", report.Targets[0])
	}
}

func TestMessageDurationWording(t *testing.T) {
	for content, want := range map[domain.MessageContent]string{
		{Text: "x"}:                    "A student can close it after 10 seconds.",
		{Text: "x", Seconds: 30}:       "It goes away by itself after 30 seconds.",
		{Text: "x", Seconds: 60}:       "It goes away by itself after 1 minute.",
		{Text: "x", Seconds: 120}:      "It goes away by itself after 2 minutes.",
		{Text: "x", UntilUnlock: true}: "It goes away when the computer is unlocked; on a computer that is not locked the student can close it.",
	} {
		if got := messageDuration(content); got != want {
			t.Errorf("%+v: %q, want %q", content, got, want)
		}
	}
}

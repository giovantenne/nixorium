package adapters

import (
	"errors"
	"testing"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

func TestMessageSessionShowsAndHidesWithTheRequestedDuration(t *testing.T) {
	session, requests := pipedAgentSession(t, []classroomview.Message{
		{Type: classroomview.TypeMessageShown},
		{Type: classroomview.TypeMessageShown},
		{Type: classroomview.TypeMessageHidden},
	})
	if err := session.ShowMessage("Exam in progress.", 120, false); err != nil {
		t.Fatal(err)
	}
	if request := <-requests; request.Type != classroomview.TypeMessageShow || request.Text != "Exam in progress." || request.Seconds != 120 || request.UntilUnlock {
		t.Fatalf("show request = %+v", request)
	}
	if err := session.ShowMessage("Wait.", 0, true); err != nil {
		t.Fatal(err)
	}
	if request := <-requests; request.Seconds != 0 || !request.UntilUnlock {
		t.Fatalf("until-unlock request = %+v", request)
	}
	if err := session.HideMessage(); err != nil {
		t.Fatal(err)
	}
	if request := <-requests; request.Type != classroomview.TypeMessageHide || request.Text != "" {
		t.Fatalf("hide request = %+v", request)
	}
}

func TestMessageSessionReportsWhatTheAgentRefused(t *testing.T) {
	for name, code := range map[string]string{
		"an older agent":          classroomview.CodeUnsupported,
		"no extension":            classroomview.CodeMessageUnavailable,
		"a message out of limits": classroomview.CodeBadRequest,
	} {
		session, _ := pipedAgentSession(t, []classroomview.Message{{Type: classroomview.TypeError, Code: code}})
		var agentError classroomview.AgentError
		if err := session.ShowMessage("Wait.", 0, false); !errors.As(err, &agentError) || agentError.Code != code {
			t.Fatalf("%s: error = %v", name, err)
		}
	}
	// A reply of another type is not a confirmation.
	session, _ := pipedAgentSession(t, []classroomview.Message{{Type: classroomview.TypeLockState}})
	if err := session.HideMessage(); err == nil {
		t.Fatal("an unrelated reply confirmed the hiding")
	}
}

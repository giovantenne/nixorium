package adapters

import (
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

// The classroom agent session also shows a message on the student's screen
// (message.show, message.hide).
var _ classroomview.Messenger = (*sshAgentSession)(nil)

func (session *sshAgentSession) messageExchange(request classroomview.Message, want string) error {
	reply, err := session.exchange(request, 10*time.Second)
	if err != nil {
		return err
	}
	if reply.Type != want {
		return classroomview.AgentError{Code: reply.Code}
	}
	return nil
}

func (session *sshAgentSession) ShowMessage(text string, seconds int, untilUnlock bool) error {
	return session.messageExchange(classroomview.Message{Type: classroomview.TypeMessageShow, Text: text, Seconds: seconds, UntilUnlock: untilUnlock}, classroomview.TypeMessageShown)
}

func (session *sshAgentSession) HideMessage() error {
	return session.messageExchange(classroomview.Message{Type: classroomview.TypeMessageHide}, classroomview.TypeMessageHidden)
}

package adapters

import (
	"errors"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

// The classroom agent session also reads the student's desktop
// (collect.list, collect.chunk, collect.end).
var _ classroomview.Collector = (*sshAgentSession)(nil)

func (session *sshAgentSession) collectExchange(request classroomview.Message, want string, timeout time.Duration) (classroomview.Message, error) {
	reply, err := session.exchange(request, timeout)
	if err != nil {
		return reply, err
	}
	if reply.Type != want {
		return reply, classroomview.AgentError{Code: reply.Code}
	}
	return reply, nil
}

func (session *sshAgentSession) ListDesktop() ([]classroomview.FileEntry, int, error) {
	reply, err := session.collectExchange(classroomview.Message{Type: classroomview.TypeCollectList}, classroomview.TypeCollectListed, 60*time.Second)
	if err != nil {
		return nil, 0, err
	}
	if reply.Entries == nil {
		reply.Entries = []classroomview.FileEntry{}
	}
	return reply.Entries, reply.Skipped, nil
}

// ReadDesktopFile asks for each piece of one file in turn; every piece
// must belong to that file and the file must not grow past its size.
func (session *sshAgentSession) ReadDesktopFile(index int, size int64, piece func([]byte) error) error {
	for received := int64(0); received < size; {
		reply, err := session.collectExchange(classroomview.Message{Type: classroomview.TypeCollectChunk, Index: index}, classroomview.TypeCollectData, 30*time.Second)
		if err != nil {
			return err
		}
		if reply.Index != index || len(reply.Data) == 0 || len(reply.Data) > classroomview.MaxChunkBytes || received+int64(len(reply.Data)) > size {
			return errors.New("the computer sent a piece that does not belong to the file")
		}
		if err := piece(reply.Data); err != nil {
			return err
		}
		received += int64(len(reply.Data))
	}
	return nil
}

func (session *sshAgentSession) EndCollect() error {
	_, err := session.collectExchange(classroomview.Message{Type: classroomview.TypeCollectEnd}, classroomview.TypeCollectDone, 10*time.Second)
	return err
}

package app

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// screenshotSession answers not-ready a few times, then a picture.
type screenshotSession struct {
	fakeAgentSession
	mutex   *sync.Mutex
	widths  *[]int
	pending *int
	code    string
}

func (session screenshotSession) Thumbnail(width int, since int64) (classroomview.Message, error) {
	session.mutex.Lock()
	defer session.mutex.Unlock()
	*session.widths = append(*session.widths, width)
	if session.code != "" {
		return classroomview.Message{Type: classroomview.TypeError, Code: session.code}, nil
	}
	if *session.pending > 0 {
		*session.pending--
		return classroomview.Message{Type: classroomview.TypeError, Code: classroomview.CodeNotReady}, nil
	}
	if since != 0 {
		return classroomview.Message{}, errors.New("a picture was asked for since a frame")
	}
	return classroomview.Message{Type: classroomview.TypeThumbnail, Image: []byte("full picture")}, nil
}

func TestScreenshotAsksALiveComputerForAFullSizePicture(t *testing.T) {
	hub := NewClassroomViewHub(func(context.Context) ([]domain.HostMeta, error) { return nil, nil }, &fakeAgentConnector{})
	var mutex sync.Mutex
	widths, pending := []int{}, 1
	session := func(code string) classroomview.Session {
		return screenshotSession{fakeAgentSession: fakeAgentSession{mutex: &mutex, closed: new(int), inputs: new(int)}, mutex: &mutex, widths: &widths, pending: &pending, code: code}
	}
	hub.computers["pc01"] = &ClassroomComputer{Name: "pc01", State: ClassroomViewing}
	hub.sessions["pc01"] = session("")
	hub.computers["pc02"] = &ClassroomComputer{Name: "pc02", State: ClassroomLocked}
	hub.sessions["pc02"] = session("")
	hub.computers["pc03"] = &ClassroomComputer{Name: "pc03", State: ClassroomUnreachable}
	hub.computers["pc04"] = &ClassroomComputer{Name: "pc04", State: ClassroomViewing}
	hub.sessions["pc04"] = session(classroomview.CodeLocked)

	image, computer, err := hub.Screenshot("pc01")
	if err != nil || string(image) != "full picture" || computer.Name != "pc01" {
		t.Fatalf("pc01 = %q %+v %v", image, computer, err)
	}
	if len(widths) != 2 || widths[0] != classroomview.MaxThumbnailWidth {
		t.Fatalf("widths = %v", widths)
	}
	for name, state := range map[string]string{"pc02": ClassroomLocked, "pc03": ClassroomUnreachable, "pc04": ClassroomLocked} {
		if _, computer, err := hub.Screenshot(name); !errors.Is(err, ErrClassroomNoPicture) || computer.State != state {
			t.Fatalf("%s = %+v %v", name, computer, err)
		}
	}
	if _, _, err := hub.Screenshot("pc09"); !errors.Is(err, ErrClassroomUnknown) {
		t.Fatalf("unknown = %v", err)
	}
}

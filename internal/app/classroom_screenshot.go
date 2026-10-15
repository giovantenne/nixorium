package app

import (
	"errors"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

// ErrClassroomNoPicture means a computer has no live picture to save; the
// computer returned with it tells why (its state and detail).
var ErrClassroomNoPicture = errors.New("this computer has no live picture")

// ErrClassroomUnknown means a computer is not on the overview.
var ErrClassroomUnknown = errors.New("unknown computer")

// screenshotAttempts bounds the wait for a first full-size frame: the
// agent restarts its capture at the larger width.
const screenshotAttempts = 6

// Screenshot asks a computer on the overview for a picture of its screen
// now, at up to classroomview.MaxThumbnailWidth pixels, through the channel
// the overview keeps open. Only a computer whose screen is live has one.
func (hub *ClassroomViewHub) Screenshot(name string) ([]byte, ClassroomComputer, error) {
	hub.mutex.Lock()
	computer, found := hub.computers[name]
	session := hub.sessions[name]
	var snapshot ClassroomComputer
	if found {
		snapshot = *computer
		snapshot.imageBytes = nil
	}
	hub.mutex.Unlock()
	if !found {
		return nil, snapshot, ErrClassroomUnknown
	}
	if snapshot.State != ClassroomViewing || session == nil {
		return nil, snapshot, ErrClassroomNoPicture
	}
	for attempt := 0; attempt < screenshotAttempts; attempt++ {
		reply, err := session.Thumbnail(classroomview.MaxThumbnailWidth, 0)
		switch {
		case err != nil:
			return nil, snapshot, err
		case reply.Type == classroomview.TypeThumbnail && len(reply.Image) > 0:
			return reply.Image, snapshot, nil
		case reply.Type == classroomview.TypeThumbnail, reply.Code == classroomview.CodeNotReady:
			time.Sleep(500 * time.Millisecond)
		default:
			snapshot.State, snapshot.Detail = classroomFailureState(classroomview.AgentError{Code: reply.Code})
			return nil, snapshot, ErrClassroomNoPicture
		}
	}
	return nil, snapshot, errors.New("the computer sent no picture in time")
}

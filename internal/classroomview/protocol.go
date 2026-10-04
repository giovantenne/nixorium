// Package classroomview holds the classroom view protocol spoken between the
// controller and the agent in each client's graphical session. The channel is
// the existing controller SSH access, so the protocol carries no credentials;
// it only bounds and validates what either side may send.
package classroomview

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ProtocolVersion changes whenever a message is added or reinterpreted.
const ProtocolVersion = 1

// MaxMessageBytes bounds one encoded message, including future image payloads.
const MaxMessageBytes = 4 << 20

// Message types.
const (
	TypeHello            = "hello"
	TypeError            = "error"
	TypeThumbnailRequest = "thumbnail.request"
	TypeThumbnail        = "thumbnail"
	TypeInput            = "input"
	TypeInputDone        = "input.done"
	// TypeLock asks the agent to lock (Locked) or unlock the computer; the
	// agent answers TypeLockState with the resulting state.
	TypeLock      = "lock.set"
	TypeLockState = "lock.state"
)

// Frame width limits accepted by the agent: small for the overview, up to
// full HD for the enlarged view.
const (
	MinThumbnailWidth = 64
	MaxThumbnailWidth = 1920
	// MaxInputEvents bounds one input message.
	MaxInputEvents = 128
)

// Input event kinds. Pointer positions are fractions of the screen (0–1);
// buttons are left, middle or right; keys are X keysyms.
const (
	InputPointerMove = "move"
	InputButton      = "button"
	InputScroll      = "scroll"
	InputKey         = "key"
)

// InputEvent is one mouse or keyboard event for the enlarged view.
type InputEvent struct {
	Kind    string  `json:"kind"`
	X       float64 `json:"x,omitempty"`
	Y       float64 `json:"y,omitempty"`
	Button  string  `json:"button,omitempty"`
	Pressed bool    `json:"pressed,omitempty"`
	Steps   int     `json:"steps,omitempty"`
	Keysym  uint32  `json:"keysym,omitempty"`
}

// ValidateInput refuses malformed or oversized input before it reaches a
// client.
func ValidateInput(events []InputEvent) error {
	if len(events) > MaxInputEvents {
		return errors.New("too many input events")
	}
	for _, event := range events {
		switch event.Kind {
		case InputPointerMove:
			if event.X < 0 || event.X > 1 || event.Y < 0 || event.Y > 1 {
				return errors.New("pointer position is outside the screen")
			}
		case InputButton:
			if event.Button != "left" && event.Button != "middle" && event.Button != "right" {
				return errors.New("unknown pointer button")
			}
		case InputScroll:
			if event.Steps < -10 || event.Steps > 10 || event.Steps == 0 {
				return errors.New("scroll steps are out of range")
			}
		case InputKey:
			if event.Keysym == 0 || event.Keysym > 0x10ffffff {
				return errors.New("key symbol is out of range")
			}
		default:
			return errors.New("unknown input event")
		}
	}
	return nil
}

// Error codes reported by the agent or the connect helper.
const (
	CodeNoSession   = "no-session"
	CodeNoAgent     = "no-agent"
	CodeUnsupported = "unsupported"
	CodeBadRequest  = "bad-request"
	CodeCapture     = "capture-failed"
	CodeNotReady    = "not-ready"
	CodeLocked      = "screen-locked"
	CodeBusy        = "busy"
	// CodeLockUnavailable: the session has no classroom extension to lock it.
	CodeLockUnavailable = "lock-unavailable"
)

// Message is one protocol message. Unknown fields are refused on decode.
type Message struct {
	Type    string `json:"type"`
	Version int    `json:"version,omitempty"`
	Agent   string `json:"agent,omitempty"`
	User    string `json:"user,omitempty"`
	Code    string `json:"code,omitempty"`
	Detail  string `json:"detail,omitempty"`
	// Thumbnail request and reply. Image is a JPEG; CapturedAt is the time of
	// the last screen change in Unix milliseconds (screens send frames only
	// when they change).
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Image      []byte `json:"image,omitempty"`
	CapturedAt int64  `json:"capturedAt,omitempty"`
	// Frame numbers each screen change; a request with Since equal to the
	// latest frame gets a reply without an image.
	Frame int64 `json:"frame,omitempty"`
	Since int64 `json:"since,omitempty"`
	// Input carries mouse and keyboard events; Release lets go of every key
	// and button still pressed.
	Events  []InputEvent `json:"events,omitempty"`
	Release bool         `json:"release,omitempty"`
	// Locked is the requested state in TypeLock and the current state in
	// TypeLockState and thumbnail replies.
	Locked bool `json:"locked,omitempty"`
}

// Write encodes a message as a big-endian length followed by JSON.
func Write(writer io.Writer, message Message) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if len(data) > MaxMessageBytes {
		return errors.New("classroom view message is too large")
	}
	frame := make([]byte, 4, 4+len(data))
	binary.BigEndian.PutUint32(frame, uint32(len(data)))
	_, err = writer.Write(append(frame, data...))
	return err
}

// Read decodes one message, refusing oversized frames and unknown fields.
func Read(reader io.Reader) (Message, error) {
	var header [4]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return Message{}, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > MaxMessageBytes {
		return Message{}, fmt.Errorf("classroom view message size %d is out of range", size)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return Message{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var message Message
	if err := decoder.Decode(&message); err != nil {
		return Message{}, fmt.Errorf("invalid classroom view message: %w", err)
	}
	if message.Type == "" {
		return Message{}, errors.New("classroom view message has no type")
	}
	return message, nil
}

// Session is one open channel to a client's classroom agent.
type Session interface {
	Thumbnail(width int, since int64) (Message, error)
	Input(events []InputEvent, release bool) error
	// SetLocked locks or unlocks the computer and returns the new state.
	SetLocked(locked bool) (bool, error)
	Close() error
}

// ErrUnreachable means the client did not answer over SSH.
var ErrUnreachable = errors.New("the computer did not answer")

// AgentError carries an agent or relay error code.
type AgentError struct{ Code string }

func (err AgentError) Error() string { return "classroom agent: " + err.Code }

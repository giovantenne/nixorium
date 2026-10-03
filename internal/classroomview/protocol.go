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
)

// Thumbnail width limits accepted by the agent.
const (
	MinThumbnailWidth = 64
	MaxThumbnailWidth = 640
)

// Error codes reported by the agent or the connect helper.
const (
	CodeNoSession   = "no-session"
	CodeNoAgent     = "no-agent"
	CodeUnsupported = "unsupported"
	CodeBadRequest  = "bad-request"
	CodeCapture     = "capture-failed"
	CodeNotReady    = "not-ready"
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
	Thumbnail(width int) (Message, error)
	Close() error
}

// ErrUnreachable means the client did not answer over SSH.
var ErrUnreachable = errors.New("the computer did not answer")

// AgentError carries an agent or relay error code.
type AgentError struct{ Code string }

func (err AgentError) Error() string { return "classroom agent: " + err.Code }

package classroomview

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestMessagesRoundTrip(t *testing.T) {
	var buffer bytes.Buffer
	if err := Write(&buffer, Message{Type: TypeHello, Version: ProtocolVersion, User: "student"}); err != nil {
		t.Fatal(err)
	}
	message, err := Read(&buffer)
	if err != nil || message.Type != TypeHello || message.Version != ProtocolVersion || message.User != "student" {
		t.Fatalf("message = %+v, %v", message, err)
	}
}

func TestReadRefusesMalformedFrames(t *testing.T) {
	frame := func(body string) *bytes.Reader {
		data := make([]byte, 4)
		binary.BigEndian.PutUint32(data, uint32(len(body)))
		return bytes.NewReader(append(data, body...))
	}
	for name, reader := range map[string]*bytes.Reader{
		"empty":         frame(""),
		"unknown field": frame(`{"type":"hello","command":"rm -rf /"}`),
		"no type":       frame(`{"version":1}`),
		"not json":      frame(`hello`),
		"truncated":     bytes.NewReader([]byte{0, 0, 0, 9, '{'}),
		"oversized":     bytes.NewReader([]byte{0xff, 0xff, 0xff, 0xff}),
	} {
		if _, err := Read(reader); err == nil {
			t.Errorf("%s frame was accepted", name)
		}
	}
	if err := Write(&bytes.Buffer{}, Message{Type: TypeError, Detail: strings.Repeat("x", MaxMessageBytes)}); err == nil {
		t.Error("oversized message was written")
	}
}

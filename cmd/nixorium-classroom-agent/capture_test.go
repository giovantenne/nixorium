package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math/rand"
	"testing"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

func noisyJPEG(t *testing.T) []byte {
	t.Helper()
	random := rand.New(rand.NewSource(1))
	picture := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for y := range 480 {
		for x := range 640 {
			picture.Set(x, y, color.RGBA{uint8(random.Intn(256)), uint8(random.Intn(256)), uint8(random.Intn(256)), 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, picture, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestFitFrameShrinksOrDropsOversizedPictures(t *testing.T) {
	frame := noisyJPEG(t)
	if got := fitFrame(frame, len(frame)); !bytes.Equal(got, frame) {
		t.Fatal("a frame that fits was changed")
	}
	limit := len(frame) / 2
	smaller := fitFrame(frame, limit)
	if smaller == nil || len(smaller) > limit {
		t.Fatalf("shrunk frame = %d bytes, limit %d", len(smaller), limit)
	}
	if _, err := jpeg.Decode(bytes.NewReader(smaller)); err != nil {
		t.Fatalf("shrunk frame is not a JPEG: %v", err)
	}
	if fitFrame(frame, 100) != nil {
		t.Fatal("a frame that cannot fit was kept")
	}
	if fitFrame(bytes.Repeat([]byte{1}, 200), 100) != nil {
		t.Fatal("an undecodable oversized frame was kept")
	}
}

func TestLargestPictureFitsInAMessage(t *testing.T) {
	reply := classroomview.Message{Type: classroomview.TypeThumbnail, Width: classroomview.MaxThumbnailWidth, Height: 4320, Image: make([]byte, classroomview.MaxImageBytes), CapturedAt: 1700000000000, Frame: 1 << 40}
	if err := classroomview.Write(io.Discard, reply); err != nil {
		t.Fatalf("the largest picture does not fit: %v", err)
	}
	// A frame past the cap becomes an error reply, not a failed write.
	oversized := thumbnailReply(fakeCapture{image: make([]byte, classroomview.MaxImageBytes+1)}, 320, 0)
	if oversized.Type != classroomview.TypeError || oversized.Code != classroomview.CodeCapture {
		t.Fatalf("oversized reply = %+v", oversized)
	}
}

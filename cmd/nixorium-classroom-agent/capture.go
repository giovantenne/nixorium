package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	remoteDesktopName = "org.gnome.Mutter.RemoteDesktop"
	screenCastName    = "org.gnome.Mutter.ScreenCast"
	frameBoundary     = "nixoriumframe"
	// captureIdle stops capturing when nobody asked for a frame, so the
	// sharing indicator disappears when the teacher stops watching.
	captureIdle   = 30 * time.Second
	firstFrameMax = 3 * time.Second
)

// capturer provides thumbnails of the session's screen.
type capturer interface {
	Thumbnail(width int) (image []byte, height int, capturedAt time.Time, err error)
}

// mutterCapture keeps one Mutter screen cast and one GStreamer pipeline open
// while frames are requested, and keeps the last JPEG they produced.
type mutterCapture struct {
	mutex       sync.Mutex
	changed     *sync.Cond
	running     bool
	width       int
	height      int
	frame       []byte
	frameAt     time.Time
	lastRequest time.Time
	failure     error
	stop        func()
	generation  int
}

func newMutterCapture() *mutterCapture {
	capture := &mutterCapture{}
	capture.changed = sync.NewCond(&capture.mutex)
	go capture.stopWhenIdle()
	return capture
}

func (capture *mutterCapture) Thumbnail(width int) ([]byte, int, time.Time, error) {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	capture.lastRequest = time.Now()
	if !capture.running || capture.width != width {
		capture.stopLocked()
		if err := capture.startLocked(width); err != nil {
			return nil, 0, time.Time{}, err
		}
	}
	deadline := time.Now().Add(firstFrameMax)
	for capture.frame == nil && capture.running && time.Now().Before(deadline) {
		timer := time.AfterFunc(100*time.Millisecond, capture.changed.Broadcast)
		capture.changed.Wait()
		timer.Stop()
	}
	if capture.frame == nil {
		if capture.failure != nil {
			return nil, 0, time.Time{}, capture.failure
		}
		return nil, 0, time.Time{}, errNotReady
	}
	return capture.frame, capture.height, capture.frameAt, nil
}

var errNotReady = errors.New("no screen frame yet")

func (capture *mutterCapture) stopWhenIdle() {
	for range time.Tick(5 * time.Second) {
		capture.mutex.Lock()
		if capture.running && time.Since(capture.lastRequest) > captureIdle {
			capture.stopLocked()
		}
		capture.mutex.Unlock()
	}
}

func (capture *mutterCapture) stopLocked() {
	if capture.stop != nil {
		capture.stop()
	}
	capture.running, capture.stop, capture.frame, capture.failure = false, nil, nil, nil
}

// startLocked opens the Mutter sessions (no consent dialog: these are the
// compositor's own interfaces) and starts the JPEG pipeline on the stream.
func (capture *mutterCapture) startLocked(width int) error {
	connection, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("session bus: %w", err)
	}
	fail := func(err error) error {
		connection.Close()
		return err
	}
	var remotePath, castPath, streamPath dbus.ObjectPath
	if err := connection.Object(remoteDesktopName, "/org/gnome/Mutter/RemoteDesktop").Call(remoteDesktopName+".CreateSession", 0).Store(&remotePath); err != nil {
		return fail(fmt.Errorf("remote desktop session: %w", err))
	}
	remote := connection.Object(remoteDesktopName, remotePath)
	sessionID, err := remote.GetProperty(remoteDesktopName + ".Session.SessionId")
	if err != nil {
		return fail(fmt.Errorf("remote desktop session id: %w", err))
	}
	options := map[string]dbus.Variant{"remote-desktop-session-id": sessionID}
	if err := connection.Object(screenCastName, "/org/gnome/Mutter/ScreenCast").Call(screenCastName+".CreateSession", 0, options).Store(&castPath); err != nil {
		return fail(fmt.Errorf("screen cast session: %w", err))
	}
	record := map[string]dbus.Variant{"cursor-mode": dbus.MakeVariant(uint32(1))}
	if err := connection.Object(screenCastName, castPath).Call(screenCastName+".Session.RecordMonitor", 0, "", record).Store(&streamPath); err != nil {
		return fail(fmt.Errorf("record monitor: %w", err))
	}
	signals := make(chan *dbus.Signal, 16)
	connection.Signal(signals)
	for _, match := range [][]dbus.MatchOption{
		{dbus.WithMatchObjectPath(streamPath), dbus.WithMatchInterface(screenCastName + ".Stream"), dbus.WithMatchMember("PipeWireStreamAdded")},
		{dbus.WithMatchObjectPath(remotePath), dbus.WithMatchInterface(remoteDesktopName + ".Session"), dbus.WithMatchMember("Closed")},
	} {
		if err := connection.AddMatchSignal(match...); err != nil {
			return fail(err)
		}
	}
	if err := remote.Call(remoteDesktopName+".Session.Start", 0).Err; err != nil {
		return fail(fmt.Errorf("start session: %w", err))
	}
	node, err := waitForNode(signals, streamPath)
	if err != nil {
		return fail(err)
	}
	height := thumbnailHeight(width, connection.Object(screenCastName, streamPath))
	pipeline := exec.Command("gst-launch-1.0", "-q",
		"pipewiresrc", fmt.Sprintf("path=%d", node), "always-copy=true", "!",
		"videoconvert", "!", "videoscale", "!",
		fmt.Sprintf("video/x-raw,width=%d,height=%d", width, height), "!",
		"jpegenc", "quality=60", "!",
		"multipartmux", "boundary="+frameBoundary, "!", "fdsink", "fd=1")
	output, err := pipeline.StdoutPipe()
	if err != nil {
		return fail(err)
	}
	if err := pipeline.Start(); err != nil {
		return fail(fmt.Errorf("start GStreamer: %w", err))
	}
	capture.generation++
	generation := capture.generation
	capture.running, capture.width, capture.height = true, width, height
	var once sync.Once
	capture.stop = func() {
		once.Do(func() {
			_ = pipeline.Process.Kill()
			_ = remote.Call(remoteDesktopName+".Session.Stop", 0).Err
			connection.Close()
		})
	}
	go func() {
		err := readFrames(output, func(frame []byte) {
			capture.mutex.Lock()
			capture.frame, capture.frameAt = frame, time.Now()
			capture.changed.Broadcast()
			capture.mutex.Unlock()
		})
		_ = pipeline.Wait()
		capture.ended(generation, err)
	}()
	go func() {
		for signal := range signals {
			if signal.Path == remotePath && strings.HasSuffix(signal.Name, ".Closed") {
				capture.ended(generation, errors.New("the screen cast was closed"))
				return
			}
		}
	}()
	return nil
}

// ended records that a capture stopped; the next request starts a new one.
func (capture *mutterCapture) ended(generation int, err error) {
	capture.mutex.Lock()
	defer capture.mutex.Unlock()
	if generation != capture.generation || !capture.running {
		return
	}
	capture.stop()
	capture.running, capture.stop, capture.frame = false, nil, nil
	if err != nil && !errors.Is(err, io.EOF) {
		capture.failure = err
	}
	capture.changed.Broadcast()
}

func waitForNode(signals <-chan *dbus.Signal, streamPath dbus.ObjectPath) (uint32, error) {
	timeout := time.After(5 * time.Second)
	for {
		select {
		case signal := <-signals:
			if signal.Path == streamPath && strings.HasSuffix(signal.Name, ".PipeWireStreamAdded") && len(signal.Body) == 1 {
				if node, ok := signal.Body[0].(uint32); ok {
					return node, nil
				}
			}
		case <-timeout:
			return 0, errors.New("the screen cast produced no PipeWire stream")
		}
	}
}

// thumbnailHeight keeps the monitor's aspect ratio, falling back to 16:10.
func thumbnailHeight(width int, stream dbus.BusObject) int {
	screenWidth, screenHeight := 16, 10
	if value, err := stream.GetProperty(screenCastName + ".Stream.Parameters"); err == nil {
		if parameters, ok := value.Value().(map[string]dbus.Variant); ok {
			if size, ok := parameters["size"].Value().([]any); ok && len(size) == 2 {
				if w, ok := size[0].(int32); ok && w > 0 {
					if h, ok := size[1].(int32); ok && h > 0 {
						screenWidth, screenHeight = int(w), int(h)
					}
				}
			}
		}
	}
	return scaledHeight(width, screenWidth, screenHeight)
}

func scaledHeight(width, screenWidth, screenHeight int) int {
	height := width * screenHeight / screenWidth
	if height%2 == 1 {
		height++
	}
	return max(2, height)
}

// readFrames splits GStreamer's multipart output into JPEG frames.
func readFrames(output io.Reader, deliver func([]byte)) error {
	reader := bufio.NewReader(output)
	headers := textproto.NewReader(reader)
	for {
		line, err := headers.ReadLine()
		if err != nil {
			return err
		}
		if line != "--"+frameBoundary {
			continue
		}
		mime, err := headers.ReadMIMEHeader()
		if err != nil {
			return err
		}
		length, err := strconv.Atoi(mime.Get("Content-Length"))
		if err != nil || length <= 0 || length > 4<<20 {
			return fmt.Errorf("invalid frame length %q", mime.Get("Content-Length"))
		}
		frame := make([]byte, length)
		if _, err := io.ReadFull(reader, frame); err != nil {
			return err
		}
		deliver(frame)
	}
}

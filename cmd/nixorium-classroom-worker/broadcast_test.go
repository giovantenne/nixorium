package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

type screenSession struct {
	source *screenSource
}

func (session screenSession) Thumbnail(int, int64) (classroomview.Message, error) {
	return classroomview.Message{}, nil
}
func (session screenSession) Input([]classroomview.InputEvent, bool) error { return nil }
func (session screenSession) SetLocked(locked bool) (bool, error)          { return locked, nil }
func (session screenSession) Locked() bool                                 { return false }
func (session screenSession) SendFiles([]classroomview.FileEntry, func(int) (io.ReadCloser, error)) ([]string, error) {
	return nil, nil
}
func (session screenSession) Close() error { return nil }
func (session screenSession) ShowFrame(image []byte) error {
	session.source.mutex.Lock()
	defer session.source.mutex.Unlock()
	session.source.frames = append(session.source.frames, string(image))
	return nil
}
func (session screenSession) StopBroadcast() error {
	session.source.mutex.Lock()
	defer session.source.mutex.Unlock()
	session.source.stops++
	return nil
}

type screenSource struct {
	mutex  sync.Mutex
	frames []string
	stops  int
}

func (source *screenSource) LabMeta(context.Context, string) (domain.LabMeta, error) {
	meta := domain.LabMeta{}
	meta.Controller.Name = "pc99"
	meta.Clients.Hosts = []domain.HostMeta{{Name: "pc01", IP: "10.0.0.1"}}
	return meta, nil
}

func (source *screenSource) Connect(context.Context, domain.HostMeta) (classroomview.Session, error) {
	return screenSession{source}, nil
}

func TestPageShowsTheTeachersScreen(t *testing.T) {
	fake := &fakeActions{}
	server, host, cookie := openedViewServer(t, fake)
	source := &screenSource{}
	server.broadcasts = &broadcaster{manager: app.NewBroadcastManager(source), repository: "/srv/lab", viewOn: func() bool { return true }}
	origin := "http://" + host
	plan := actionPost(server, host, cookie, origin, "/api/actions/plan", `{"action":"show-screen","computers":["pc01"]}`)
	var review actionReview
	if err := json.Unmarshal(plan.Body.Bytes(), &review); err != nil || !review.Ready || review.Confirm != "Show my screen on 1 computer" {
		t.Fatalf("review = %s", plan.Body.String())
	}
	apply := actionPost(server, host, cookie, origin, "/api/actions/apply", `{"id":"`+review.ID+`"}`)
	var result actionResult
	if err := json.Unmarshal(apply.Body.Bytes(), &result); err != nil || result.State != "started" || result.Broadcast == "" {
		t.Fatalf("result = %s", apply.Body.String())
	}
	post := func(path, contentType, body string) int {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		request.Host = host
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", contentType)
		request.AddCookie(&http.Cookie{Name: viewCookie, Value: cookie})
		recorder := httptest.NewRecorder()
		server.ServeHTTP(recorder, request)
		return recorder.Code
	}
	if code := post("/api/broadcast/frame?id="+result.Broadcast, "image/jpeg", "picture"); code != http.StatusNoContent {
		t.Fatalf("frame = %d", code)
	}
	if code := post("/api/broadcast/frame?id="+result.Broadcast, "application/json", "picture"); code == http.StatusNoContent {
		t.Fatal("a frame with the wrong type was accepted")
	}
	if code := post("/api/broadcast/frame?id=other", "image/jpeg", "picture"); code != http.StatusGone {
		t.Fatalf("frame of another showing = %d", code)
	}
	for attempt := 0; ; attempt++ {
		list := viewRequest(server, "/api/computers", host, cookie)
		if strings.Contains(list.Body.String(), `"showing":true`) {
			break
		}
		if attempt > 200 {
			t.Fatalf("pc01 is not showing: %s", list.Body.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if code := post("/api/broadcast/stop?id="+result.Broadcast, "application/json", "{}"); code != http.StatusNoContent {
		t.Fatalf("stop = %d", code)
	}
	if code := post("/api/broadcast/frame?id="+result.Broadcast, "image/jpeg", "picture"); code != http.StatusGone {
		t.Fatalf("frame after the end = %d", code)
	}
	for attempt := 0; ; attempt++ {
		source.mutex.Lock()
		stops, frames := source.stops, len(source.frames)
		source.mutex.Unlock()
		if stops == 1 && frames >= 1 {
			break
		}
		if attempt > 200 {
			t.Fatalf("stops %d frames %d", stops, frames)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
)

// pictureSource has a live screen on pc01 and pc04, the student's lock on
// pc02, pc03 switched off and nobody on pc05.
type pictureSource struct {
	fakeViewSource
}

func (pictureSource) Screenshot(name string) ([]byte, app.ClassroomComputer, error) {
	switch name {
	case "pc01", "pc04":
		return []byte("\xff\xd8" + name + "\xff\xd9"), app.ClassroomComputer{Name: name, State: app.ClassroomViewing}, nil
	case "pc02":
		return nil, app.ClassroomComputer{Name: name, State: app.ClassroomLocked}, app.ErrClassroomNoPicture
	case "pc03":
		return nil, app.ClassroomComputer{Name: name, State: app.ClassroomUnreachable}, app.ErrClassroomNoPicture
	case "pc05":
		return nil, app.ClassroomComputer{Name: name, State: app.ClassroomNoSession}, app.ErrClassroomNoPicture
	case "pc06":
		return nil, app.ClassroomComputer{Name: name, State: app.ClassroomViewing}, errors.New("the classroom agent did not answer in time")
	}
	return nil, app.ClassroomComputer{}, app.ErrClassroomUnknown
}

func screenshotServer(t *testing.T) (*viewServer, string, string) {
	t.Helper()
	server, host, cookie := openedViewServer(t, &fakeActions{})
	server.source = pictureSource{}
	server.desktops = newDesktopBroker()
	server.now = func() time.Time { return time.Date(2026, 10, 15, 10, 30, 12, 0, time.Local) }
	return server, host, cookie
}

func TestScreenshotsGoToTheHelperOfThePageUser(t *testing.T) {
	server, host, cookie := screenshotServer(t)
	home := t.TempDir()
	// The helper of the page's user saves the pictures in its home.
	helperSavesScreenshots(t, server.desktops, 1000, home)
	response := actionPost(server, host, cookie, "http://"+host, "/api/screenshots", `{"computers":["pc01","pc02","pc03","pc04","pc05","pc06"]}`)
	var result screenshotResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.Rows) != 6 || len(result.Downloads) != 0 {
		t.Fatalf("result = %d %s", response.Code, response.Body.String())
	}
	want := []struct{ outcome, note string }{
		{outcomeDone, "Saved as Nixorium/Screenshots/pc01-2026-10-15-10.30.12.jpg."},
		{outcomeSkipped, "Idle: the student's screen lock is on."},
		{outcomeOff, app.ClassroomOffDetail},
		{outcomeDone, "Saved as Nixorium/Screenshots/pc04-2026-10-15-10.30.12.jpg."},
		{outcomeSkipped, "Nobody is signed in."},
		{outcomeFailed, "The screen could not be captured; try again."},
	}
	for index, row := range result.Rows {
		if row.Outcome != want[index].outcome || row.Note != want[index].note {
			t.Fatalf("%s = %+v", row.Name, row)
		}
	}
	if result.Rows[0].File != "Nixorium/Screenshots/pc01-2026-10-15-10.30.12.jpg" || result.State != "partial" {
		t.Fatalf("result = %+v", result)
	}
	content, err := os.ReadFile(filepath.Join(home, "Nixorium", "Screenshots", "pc04-2026-10-15-10.30.12.jpg"))
	if err != nil || string(content) != "\xff\xd8pc04\xff\xd9" {
		t.Fatalf("picture = %q, %v", content, err)
	}
	// The same second again keeps the first picture.
	response = actionPost(server, host, cookie, "http://"+host, "/api/screenshots", `{"computers":["pc01"]}`)
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Rows[0].File != "Nixorium/Screenshots/pc01-2026-10-15-10.30.12 (2).jpg" {
		t.Fatalf("again = %s", response.Body.String())
	}
}

// helperSavesScreenshots answers save jobs like the desktop helper, into
// ~/Nixorium/Screenshots of home.
func helperSavesScreenshots(t *testing.T, broker *desktopBroker, uid int, home string) {
	t.Helper()
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			job := broker.Wait(t.Context(), uid)
			if job.id == "" {
				continue
			}
			placed, err := helperCopy(broker, uid, job.id, filepath.Join(home, "Nixorium", "Screenshots"))
			names, message := []string{}, ""
			if err != nil {
				message = err.Error()
			}
			for _, name := range placed {
				names = append(names, "Nixorium/Screenshots/"+name)
			}
			broker.SaveDone(uid, job.id, names, message)
		}
	}()
	time.Sleep(20 * time.Millisecond)
}

func TestScreenshotsDownloadWithoutAHelper(t *testing.T) {
	server, host, cookie := screenshotServer(t)
	response := actionPost(server, host, cookie, "http://"+host, "/api/screenshots", `{"computers":["pc01","pc03"]}`)
	var result screenshotResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.Downloads) != 1 || result.Downloads[0].Name != "pc01-2026-10-15-10.30.12.jpg" {
		t.Fatalf("result = %s", response.Body.String())
	}
	if result.Rows[0].Outcome != outcomeDone || result.Rows[0].File != "pc01-2026-10-15-10.30.12.jpg" || result.Rows[1].Outcome != outcomeOff {
		t.Fatalf("rows = %+v", result.Rows)
	}
	path := "/api/screenshots/file?id=" + result.Downloads[0].ID
	download := viewRequest(server, path, host, cookie)
	if download.Code != http.StatusOK || download.Header().Get("Content-Type") != "image/jpeg" || download.Header().Get("Content-Disposition") != `attachment; filename=pc01-2026-10-15-10.30.12.jpg` || download.Body.String() != "\xff\xd8pc01\xff\xd9" {
		t.Fatalf("download = %d %v %q", download.Code, download.Header(), download.Body.String())
	}
	// Each picture downloads once, and only with the page's session.
	if again := viewRequest(server, path, host, cookie); again.Code != http.StatusNotFound {
		t.Fatalf("second download = %d", again.Code)
	}
	response = actionPost(server, host, cookie, "http://"+host, "/api/screenshots", `{"computers":["pc04"]}`)
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.Downloads) != 1 {
		t.Fatalf("result = %s", response.Body.String())
	}
	if anonymous := viewRequest(server, "/api/screenshots/file?id="+result.Downloads[0].ID, host, ""); anonymous.Code != http.StatusForbidden {
		t.Fatalf("anonymous download = %d", anonymous.Code)
	}
	// Pictures not downloaded expire.
	server.now = func() time.Time { return time.Now().Add(time.Hour) }
	if expired := viewRequest(server, "/api/screenshots/file?id="+result.Downloads[0].ID, host, cookie); expired.Code != http.StatusNotFound {
		t.Fatalf("expired download = %d", expired.Code)
	}
}

func TestScreenshotsRefuseForeignRequests(t *testing.T) {
	server, host, cookie := screenshotServer(t)
	origin := "http://" + host
	for name, response := range map[string]*httptest.ResponseRecorder{
		"foreign origin": actionPost(server, host, cookie, "http://evil.example", "/api/screenshots", `{"computers":["pc01"]}`),
		"no session":     actionPost(server, host, "", origin, "/api/screenshots", `{"computers":["pc01"]}`),
		"bad name":       actionPost(server, host, cookie, origin, "/api/screenshots", `{"computers":["../pc01"]}`),
		"no computers":   actionPost(server, host, cookie, origin, "/api/screenshots", `{"computers":[]}`),
		"unknown field":  actionPost(server, host, cookie, origin, "/api/screenshots", `{"computers":["pc01"],"folder":"/etc"}`),
		"download post":  actionPost(server, host, cookie, origin, "/api/screenshots/file", `{}`),
	} {
		if response.Code == http.StatusOK {
			t.Fatalf("%s was accepted: %s", name, response.Body.String())
		}
	}
	// A page whose source cannot take pictures refuses too.
	server.source = fakeViewSource{}
	if response := actionPost(server, host, cookie, origin, "/api/screenshots", `{"computers":["pc01"]}`); response.Code == http.StatusOK || strings.Contains(response.Body.String(), "downloads") {
		t.Fatalf("no source = %d %s", response.Code, response.Body.String())
	}
}

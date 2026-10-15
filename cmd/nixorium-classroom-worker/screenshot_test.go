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

// pictureSource has a live screen on pc01, the student's lock on pc02,
// pc03 switched off, nobody on pc05 and a capture that fails on pc06.
type pictureSource struct {
	fakeViewSource
}

func (pictureSource) Screenshot(name string) ([]byte, app.ClassroomComputer, error) {
	switch name {
	case "pc01":
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

func takeTestScreenshot(t *testing.T, server *viewServer, host, cookie, computer string) screenshotResult {
	t.Helper()
	response := actionPost(server, host, cookie, "http://"+host, "/api/screenshot", `{"computer":"`+computer+`"}`)
	var result screenshotResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("%s = %d %s", computer, response.Code, response.Body.String())
	}
	return result
}

func TestScreenshotGoesToTheHelperOfThePageUser(t *testing.T) {
	server, host, cookie := screenshotServer(t)
	home := t.TempDir()
	// The helper of the page's user saves the picture in its home.
	helperSavesScreenshots(t, server.desktops, 1000, home)
	result := takeTestScreenshot(t, server, host, cookie, "pc01")
	if result.State != screenshotSaved || result.File != "Nixorium/Screenshots/pc01-2026-10-15-10.30.12.jpg" || result.Download != "" {
		t.Fatalf("pc01 = %+v", result)
	}
	content, err := os.ReadFile(filepath.Join(home, "Nixorium", "Screenshots", "pc01-2026-10-15-10.30.12.jpg"))
	if err != nil || string(content) != "\xff\xd8pc01\xff\xd9" {
		t.Fatalf("picture = %q, %v", content, err)
	}
	// The same second again keeps the first picture.
	if again := takeTestScreenshot(t, server, host, cookie, "pc01"); again.File != "Nixorium/Screenshots/pc01-2026-10-15-10.30.12 (2).jpg" {
		t.Fatalf("again = %+v", again)
	}
	// Computers without a live picture say why, in the card's words.
	for computer, want := range map[string]screenshotResult{
		"pc02": {State: screenshotSkipped, Message: "Idle: the student's screen lock is on."},
		"pc03": {State: screenshotSkipped, Message: app.ClassroomOffDetail},
		"pc05": {State: screenshotSkipped, Message: "Nobody is signed in."},
		"pc06": {State: screenshotFailed, Message: "The screen could not be captured; try again."},
		"pc09": {State: screenshotFailed, Message: "This computer is not on the overview."},
	} {
		if result := takeTestScreenshot(t, server, host, cookie, computer); result != want {
			t.Fatalf("%s = %+v", computer, result)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(home, "Nixorium", "Screenshots")); len(entries) != 2 {
		t.Fatalf("pictures = %v", entries)
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

func TestScreenshotDownloadsWithoutAHelper(t *testing.T) {
	server, host, cookie := screenshotServer(t)
	result := takeTestScreenshot(t, server, host, cookie, "pc01")
	if result.State != screenshotDownload || result.File != "pc01-2026-10-15-10.30.12.jpg" || result.Download == "" {
		t.Fatalf("result = %+v", result)
	}
	path := "/api/screenshot/file?id=" + result.Download
	download := viewRequest(server, path, host, cookie)
	if download.Code != http.StatusOK || download.Header().Get("Content-Type") != "image/jpeg" || download.Header().Get("Content-Disposition") != `attachment; filename=pc01-2026-10-15-10.30.12.jpg` || download.Body.String() != "\xff\xd8pc01\xff\xd9" {
		t.Fatalf("download = %d %v %q", download.Code, download.Header(), download.Body.String())
	}
	// The picture downloads once, and only with the page's session.
	if again := viewRequest(server, path, host, cookie); again.Code != http.StatusNotFound {
		t.Fatalf("second download = %d", again.Code)
	}
	result = takeTestScreenshot(t, server, host, cookie, "pc01")
	if anonymous := viewRequest(server, "/api/screenshot/file?id="+result.Download, host, ""); anonymous.Code != http.StatusForbidden {
		t.Fatalf("anonymous download = %d", anonymous.Code)
	}
	// A picture not downloaded expires.
	server.now = func() time.Time { return time.Now().Add(time.Hour) }
	if expired := viewRequest(server, "/api/screenshot/file?id="+result.Download, host, cookie); expired.Code != http.StatusNotFound {
		t.Fatalf("expired download = %d", expired.Code)
	}
}

func TestScreenshotRefusesForeignOrManyComputerRequests(t *testing.T) {
	server, host, cookie := screenshotServer(t)
	origin := "http://" + host
	for name, response := range map[string]*httptest.ResponseRecorder{
		"foreign origin":  actionPost(server, host, cookie, "http://evil.example", "/api/screenshot", `{"computer":"pc01"}`),
		"no session":      actionPost(server, host, "", origin, "/api/screenshot", `{"computer":"pc01"}`),
		"bad name":        actionPost(server, host, cookie, origin, "/api/screenshot", `{"computer":"../pc01"}`),
		"no computer":     actionPost(server, host, cookie, origin, "/api/screenshot", `{}`),
		"many computers":  actionPost(server, host, cookie, origin, "/api/screenshot", `{"computers":["pc01","pc04"]}`),
		"unknown field":   actionPost(server, host, cookie, origin, "/api/screenshot", `{"computer":"pc01","folder":"/etc"}`),
		"download post":   actionPost(server, host, cookie, origin, "/api/screenshot/file", `{}`),
		"former endpoint": actionPost(server, host, cookie, origin, "/api/screenshots", `{"computers":["pc01"]}`),
	} {
		if response.Code == http.StatusOK {
			t.Fatalf("%s was accepted: %s", name, response.Body.String())
		}
	}
	// A page whose source cannot take pictures refuses too.
	server.source = fakeViewSource{}
	if response := actionPost(server, host, cookie, origin, "/api/screenshot", `{"computer":"pc01"}`); response.Code == http.StatusOK || strings.Contains(response.Body.String(), "download") {
		t.Fatalf("no source = %d %s", response.Code, response.Body.String())
	}
}

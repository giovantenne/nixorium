package main

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Save screenshot: in a computer's window, the service asks that computer
// for a picture of its screen at up to 1920 pixels and gives it to the
// desktop helper of the page's user, which saves it as
// ~/Nixorium/Screenshots/<computer>-<YYYY-MM-DD>-<HH.MM.SS>.jpg. A page
// opened without the helper downloads the picture through the browser.

func init() {
	postPaths["/api/screenshot"] = true
}

// screenshotSource takes full-size pictures of computers on the overview.
type screenshotSource interface {
	Screenshot(name string) ([]byte, app.ClassroomComputer, error)
}

const (
	// A busy helper gets a moment before the browser downloads the
	// picture instead; screenshotSaveTime bounds its work on it.
	screenshotHelperWait = 2 * time.Second
	screenshotSaveTime   = time.Minute
	// screenshotKeep is how long a picture waits for its download, and
	// screenshotKeepBytes bounds the pictures waiting.
	screenshotKeep      = 2 * time.Minute
	screenshotKeepBytes = 64 << 20
	screenshotLayout    = "2006-01-02-15.04.05"
)

// Outcomes of a screenshot.
const (
	screenshotSaved    = "saved"
	screenshotDownload = "download"
	screenshotSkipped  = "skipped"
	screenshotFailed   = "failed"
)

// screenshotResult is what the page shows: where the picture was saved
// (File, in the home), the picture to download (Download identifies it
// for /api/screenshot/file, under the name File), or why there is none.
type screenshotResult struct {
	State    string `json:"state"`
	Message  string `json:"message,omitempty"`
	File     string `json:"file,omitempty"`
	Download string `json:"download,omitempty"`
}

// screenshotDownloads keeps pictures for a short while until the page
// downloads them, each once.
type screenshotDownloads struct {
	mutex    sync.Mutex
	pictures map[string]keptPicture
}

type keptPicture struct {
	name    string
	image   []byte
	expires time.Time
}

func (kept *screenshotDownloads) add(name string, image []byte, now time.Time) (string, error) {
	id, err := randomToken()
	if err != nil {
		return "", err
	}
	kept.mutex.Lock()
	defer kept.mutex.Unlock()
	if kept.pictures == nil {
		kept.pictures = map[string]keptPicture{}
	}
	total := len(image)
	for key, picture := range kept.pictures {
		if now.After(picture.expires) {
			delete(kept.pictures, key)
		} else {
			total += len(picture.image)
		}
	}
	if total > screenshotKeepBytes {
		return "", errors.New("too many pictures are waiting to be downloaded")
	}
	kept.pictures[id] = keptPicture{name: name, image: image, expires: now.Add(screenshotKeep)}
	return id, nil
}

func (kept *screenshotDownloads) take(id string, now time.Time) (keptPicture, bool) {
	kept.mutex.Lock()
	defer kept.mutex.Unlock()
	picture, found := kept.pictures[id]
	delete(kept.pictures, id)
	return picture, found && now.Before(picture.expires)
}

func (server *viewServer) screenshotKeeper() *screenshotDownloads {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if server.downloads == nil {
		server.downloads = &screenshotDownloads{}
	}
	return server.downloads
}

// screenshot serves /api/screenshot (take and save one computer's
// picture) and /api/screenshot/file (download a picture no helper saved).
func (server *viewServer) screenshot(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/api/screenshot/file" {
		if request.Method != http.MethodGet {
			http.Error(writer, "Not available.", http.StatusMethodNotAllowed)
			return
		}
		picture, found := server.screenshotKeeper().take(request.URL.Query().Get("id"), server.now())
		if !found {
			http.Error(writer, "This picture is no longer available.", http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", "image/jpeg")
		writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": picture.name}))
		_, _ = writer.Write(picture.image)
		return
	}
	if request.Method != http.MethodPost {
		http.Error(writer, "Not available.", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Computer string `json:"computer"`
	}
	if !decodePageJSON(writer, request, &body) {
		return
	}
	source, capable := server.source.(screenshotSource)
	grant, signed := server.session(request)
	if !computerNamePattern.MatchString(body.Computer) || !capable || !signed {
		http.Error(writer, "Invalid request.", http.StatusBadRequest)
		return
	}
	writePageJSON(writer, server.takeScreenshot(request.Context(), source, grant.uid, body.Computer))
}

func (server *viewServer) takeScreenshot(ctx context.Context, source screenshotSource, uid int, name string) screenshotResult {
	image, computer, err := source.Screenshot(name)
	switch {
	case errors.Is(err, app.ErrClassroomNoPicture):
		return screenshotResult{State: screenshotSkipped, Message: noPictureReason(computer.State)}
	case errors.Is(err, app.ErrClassroomUnknown):
		return screenshotResult{State: screenshotFailed, Message: "This computer is not on the overview."}
	case err != nil:
		return screenshotResult{State: screenshotFailed, Message: "The screen could not be captured; try again."}
	}
	file := name + "-" + server.now().Format(screenshotLayout) + ".jpg"
	saved, message := server.saveScreenshot(ctx, uid, file, image)
	switch {
	case saved != "":
		return screenshotResult{State: screenshotSaved, File: saved}
	case message != "":
		return screenshotResult{State: screenshotFailed, Message: "The screenshot could not be saved: " + message}
	}
	id, err := server.screenshotKeeper().add(file, image, server.now())
	if err != nil {
		return screenshotResult{State: screenshotFailed, Message: "The screenshot could not be kept for downloading; try again."}
	}
	return screenshotResult{State: screenshotDownload, File: file, Download: id}
}

// saveScreenshot gives the picture to the page user's desktop helper and
// returns where it was saved. Without a helper both results are empty and
// the page downloads the picture; message says why saving failed.
func (server *viewServer) saveScreenshot(ctx context.Context, uid int, file string, image []byte) (string, string) {
	if server.desktops == nil {
		return "", ""
	}
	read := func(index int, offset int64, size int) ([]byte, error) {
		if index != 0 || offset < 0 || offset+int64(size) > int64(len(image)) {
			return nil, errors.New("no such piece of the picture")
		}
		return image[offset : offset+int64(size)], nil
	}
	items := []domain.ShareFile{{Path: file, Size: int64(len(image))}}
	id, err := server.desktops.Offer(uid, domain.DesktopSaveScreenshots, items, read, func() {})
	if err != nil {
		return "", err.Error() + "."
	}
	outcome, err := server.desktops.Deliver(ctx, id, screenshotHelperWait, screenshotSaveTime)
	switch {
	case errors.Is(err, errNoDesktopHelper):
		server.desktops.Withdraw(id)
		return "", ""
	case err != nil:
		server.desktops.Withdraw(id)
		return "", err.Error() + "."
	case len(outcome.names) != 1 && outcome.message != "":
		return "", outcome.message
	case len(outcome.names) != 1:
		return "", "the picture was not saved."
	}
	return outcome.names[0], ""
}

// noPictureReason words a computer without a live picture as the page
// does for its card.
func noPictureReason(state string) string {
	switch state {
	case app.ClassroomUnreachable:
		return app.ClassroomOffDetail
	case app.ClassroomLocked:
		return "Idle: the student's screen lock is on."
	case app.ClassroomNoSession:
		return "Nobody is signed in."
	case app.ClassroomNoAgent:
		return app.ClassroomNoAgentDetail
	case app.ClassroomConnecting:
		return "Still connecting; try again in a moment."
	}
	return "The screen could not be captured."
}

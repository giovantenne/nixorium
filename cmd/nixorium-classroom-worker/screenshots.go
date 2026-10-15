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

// Save screenshots: the service asks each selected computer for a picture
// of its screen at up to 1920 pixels and gives the pictures to the desktop
// helper of the page's user, which saves them as
// ~/Nixorium/Screenshots/<computer>-<YYYY-MM-DD>-<HH.MM.SS>.jpg. A page
// opened without the helper downloads them through the browser instead.

func init() {
	postPaths["/api/screenshots"] = true
}

// screenshotSource takes full-size pictures of computers on the overview.
type screenshotSource interface {
	Screenshot(name string) ([]byte, app.ClassroomComputer, error)
}

const (
	screenshotParallel = 4
	// A busy helper gets a moment before the browser downloads the
	// pictures instead; screenshotSaveTime bounds its work on them.
	screenshotHelperWait = 2 * time.Second
	screenshotSaveTime   = 2 * time.Minute
	// screenshotKeep is how long a picture waits for its download.
	screenshotKeep = 2 * time.Minute
	// screenshotKeepBytes bounds the pictures waiting for a download.
	screenshotKeepBytes = 128 << 20
	screenshotLayout    = "2006-01-02-15.04.05"
)

// screenshotRow is one computer of the outcome; File is the saved or
// downloaded picture.
type screenshotRow struct {
	actionRow
	File string `json:"file,omitempty"`
}

type screenshotResult struct {
	State   string          `json:"state"`
	Message string          `json:"message,omitempty"`
	Rows    []screenshotRow `json:"rows"`
	// Downloads are pictures the page downloads itself when no helper
	// saved them.
	Downloads []screenshotDownload `json:"downloads,omitempty"`
}

type screenshotDownload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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

// screenshots serves /api/screenshots (take and save) and
// /api/screenshots/file (download a picture no helper saved).
func (server *viewServer) screenshots(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/api/screenshots/file" {
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
		Computers []string `json:"computers"`
	}
	if !decodePageJSON(writer, request, &body) {
		return
	}
	source, capable := server.source.(screenshotSource)
	grant, signed := server.session(request)
	if !validActionComputers(body.Computers) || !capable || !signed {
		http.Error(writer, "Invalid request.", http.StatusBadRequest)
		return
	}
	writePageJSON(writer, server.takeScreenshots(request.Context(), source, grant.uid, body.Computers))
}

func (server *viewServer) takeScreenshots(ctx context.Context, source screenshotSource, uid int, computers []string) screenshotResult {
	result := screenshotResult{State: "completed", Rows: make([]screenshotRow, len(computers))}
	images := make([][]byte, len(computers))
	hosts := make([]domain.HostMeta, len(computers))
	for index, name := range computers {
		hosts[index] = domain.HostMeta{Name: name}
	}
	eachComputer(hosts, screenshotParallel, func(index int, host domain.HostMeta) {
		image, computer, err := source.Screenshot(host.Name)
		row := screenshotRow{actionRow: actionRow{Name: host.Name}}
		switch {
		case err == nil:
			images[index] = image
			row.File = host.Name + "-" + server.now().Format(screenshotLayout) + ".jpg"
		case errors.Is(err, app.ErrClassroomNoPicture):
			row.actionRow = screenshotSkipped(host.Name, computer.State)
		case errors.Is(err, app.ErrClassroomUnknown):
			row.Skip, row.Outcome, row.Note = true, outcomeFailed, "This computer is not on the overview."
		default:
			row.Skip, row.Outcome, row.Note = true, outcomeFailed, "The screen could not be captured; try again."
		}
		result.Rows[index] = row
	})
	items, taken := []domain.ShareFile{}, []int{}
	for index, image := range images {
		if image != nil {
			items = append(items, domain.ShareFile{Path: result.Rows[index].File, Size: int64(len(image))})
			taken = append(taken, index)
		}
	}
	for _, row := range result.Rows {
		if row.Outcome != "" {
			result.State = "partial"
		}
	}
	if len(items) == 0 {
		result.Message = "No picture was taken."
		return result
	}
	saved, message := server.saveScreenshots(ctx, uid, items, images, taken)
	for position, index := range taken {
		row := &result.Rows[index]
		switch {
		case saved != nil:
			row.Outcome, row.File = outcomeDone, saved[position]
			row.Note = "Saved as " + saved[position] + "."
		case message != "":
			row.Skip, row.Outcome, row.File = true, outcomeFailed, ""
			row.Note = "The picture could not be saved: " + message
			result.State = "partial"
		default:
			id, err := server.screenshotKeeper().add(row.File, images[index], server.now())
			if err != nil {
				row.Skip, row.Outcome, row.File, row.Note = true, outcomeFailed, "", "The picture could not be kept for downloading; try again."
				result.State = "partial"
				continue
			}
			row.Outcome, row.Note = outcomeDone, "Downloaded through the browser as "+row.File+"."
			result.Downloads = append(result.Downloads, screenshotDownload{ID: id, Name: row.File})
		}
	}
	return result
}

// saveScreenshots gives the pictures to the page user's desktop helper and
// returns where it saved each one. Without a helper both results are empty
// and the page downloads the pictures; message says why saving failed.
func (server *viewServer) saveScreenshots(ctx context.Context, uid int, items []domain.ShareFile, images [][]byte, taken []int) ([]string, string) {
	if server.desktops == nil {
		return nil, ""
	}
	read := func(index int, offset int64, size int) ([]byte, error) {
		image := images[taken[index]]
		if offset < 0 || offset+int64(size) > int64(len(image)) {
			return nil, errors.New("no such piece of a picture")
		}
		return image[offset : offset+int64(size)], nil
	}
	id, err := server.desktops.Offer(uid, domain.DesktopSaveScreenshots, items, read, func() {})
	if err != nil {
		return nil, err.Error() + "."
	}
	outcome, err := server.desktops.Deliver(ctx, id, screenshotHelperWait, screenshotSaveTime)
	switch {
	case errors.Is(err, errNoDesktopHelper):
		server.desktops.Withdraw(id)
		return nil, ""
	case err != nil:
		server.desktops.Withdraw(id)
		return nil, err.Error() + "."
	case len(outcome.names) != len(items) && outcome.message != "":
		return nil, outcome.message
	case len(outcome.names) != len(items):
		return nil, "Not every picture was saved."
	}
	return outcome.names, ""
}

// screenshotSkipped words a computer without a live picture as the page
// does for its card.
func screenshotSkipped(name, state string) actionRow {
	row := actionRow{Name: name, Skip: true, Outcome: outcomeSkipped}
	switch state {
	case app.ClassroomUnreachable:
		row.Outcome, row.Note = outcomeOff, app.ClassroomOffDetail
	case app.ClassroomLocked:
		row.Note = "Idle: the student's screen lock is on."
	case app.ClassroomNoSession:
		row.Note = "Nobody is signed in."
	case app.ClassroomNoAgent:
		row.Note = "The classroom view is not running on this computer."
	case app.ClassroomConnecting:
		row.Note = "Still connecting; try again in a moment."
	default:
		row.Outcome, row.Note = outcomeFailed, "The screen could not be captured."
	}
	return row
}

// eachComputer runs work for every computer, at most parallel at a time.
func eachComputer(hosts []domain.HostMeta, parallel int, work func(int, domain.HostMeta)) {
	var group sync.WaitGroup
	slots := make(chan struct{}, parallel)
	for index, host := range hosts {
		group.Add(1)
		slots <- struct{}{}
		go func() {
			defer func() { <-slots; group.Done() }()
			work(index, host)
		}()
	}
	group.Wait()
}

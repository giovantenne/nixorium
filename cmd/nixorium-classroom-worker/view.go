package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
)

//go:embed view/index.html view/app.js view/style.css
var viewAssets embed.FS

const (
	viewCookie       = "nixorium_classroom"
	viewTokenTTL     = time.Minute
	viewSessionTTL   = 12 * time.Hour
	viewSecurityRule = "default-src 'none'; img-src 'self' blob:; script-src 'self'; style-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
)

var computerNamePattern = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)

var postPaths = map[string]bool{"/api/input": true, "/api/actions/plan": true, "/api/actions/apply": true, "/api/share/choose": true, "/api/broadcast/frame": true, "/api/broadcast/stop": true}

// viewSource lists computers, their images, and forwards input.
type viewSource interface {
	Computers(ctx context.Context) ([]app.ClassroomComputer, error)
	Thumbnail(name string) ([]byte, bool)
	Frame(name string, since int64) ([]byte, int64, bool)
	Input(name string, events []classroomview.InputEvent, release bool) error
}

// viewServer serves the teacher's classroom view on the controller's
// loopback only. A one-time token from the classroom socket (teacher and
// administrator) becomes a browser session; other local users, such as the
// student account, cannot enter.
type viewServer struct {
	source  viewSource
	actions classroomHandler
	// broadcasts is nil when the page cannot show the teacher's screen.
	broadcasts *broadcaster
	pending    map[string]pendingAction
	labels     map[string]computerLabels
	mutex      sync.Mutex
	address    string
	tokens     map[string]viewGrant
	sessions   map[string]viewGrant
	// desktops reaches the desktop helper of a page's user, if any.
	desktops *desktopBroker
	// internet reads the computers' real Internet state while the page is
	// open; nil keeps only the page's own changes.
	internet *internetWatcher
	now      func() time.Time
}

func newViewServer(source viewSource) *viewServer {
	return &viewServer{source: source, pending: map[string]pendingAction{}, labels: map[string]computerLabels{}, tokens: map[string]viewGrant{}, sessions: map[string]viewGrant{}, now: time.Now}
}

// Open starts the page server on first use and returns a one-time address.
// viewGrant is a one-time token or a page session, for one user of the
// controller (whoever asked through the classroom socket).
type viewGrant struct {
	expires time.Time
	uid     int
}

// Open returns a one-time address for the user uid.
func (server *viewServer) Open(uid int) (string, error) {
	server.mutex.Lock()
	defer server.mutex.Unlock()
	if server.address == "" {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", err
		}
		server.address = listener.Addr().String()
		httpServer := &http.Server{Handler: server, ReadHeaderTimeout: 10 * time.Second}
		go func() { _ = httpServer.Serve(listener) }()
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	server.tokens[token] = viewGrant{expires: server.now().Add(viewTokenTTL), uid: uid}
	return "http://" + server.address + "/open?token=" + token, nil
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func (server *viewServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	header := writer.Header()
	header.Set("Content-Security-Policy", viewSecurityRule)
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Cache-Control", "no-store")
	server.mutex.Lock()
	address := server.address
	server.mutex.Unlock()
	// A page on another site cannot reach this server through DNS tricks,
	// and only the page's own endpoints accept a POST from this page itself.
	contentType := "application/json"
	switch request.URL.Path {
	case "/api/broadcast/frame":
		contentType = "image/jpeg"
	}
	post := request.Method == http.MethodPost && postPaths[request.URL.Path] &&
		request.Header.Get("Origin") == "http://"+address &&
		strings.HasPrefix(request.Header.Get("Content-Type"), contentType)
	if request.Host != address || (request.Method != http.MethodGet && !post) {
		http.Error(writer, "Not available.", http.StatusBadRequest)
		return
	}
	if request.URL.Path == "/open" {
		server.enter(writer, request)
		return
	}
	if !server.authorized(request) {
		http.Error(writer, "Open the classroom view again from Nixorium.", http.StatusForbidden)
		return
	}
	switch request.URL.Path {
	case "/":
		server.asset(writer, "view/index.html", "text/html; charset=utf-8")
	case "/app.js":
		server.asset(writer, "view/app.js", "text/javascript; charset=utf-8")
	case "/style.css":
		server.asset(writer, "view/style.css", "text/css; charset=utf-8")
	case "/api/computers":
		ctx, cancel := context.WithTimeout(request.Context(), 2*time.Minute)
		defer cancel()
		computers, err := server.source.Computers(ctx)
		if err != nil {
			http.Error(writer, "The list of computers is not available.", http.StatusServiceUnavailable)
			return
		}
		type pageComputer struct {
			app.ClassroomComputer
			Internet string `json:"internet,omitempty"`
			Power    string `json:"power,omitempty"`
			Showing  bool   `json:"showing,omitempty"`
		}
		if server.internet != nil {
			server.internet.Touch()
		}
		showing := map[string]bool{}
		if server.broadcasts != nil {
			showing = server.broadcasts.Showing()
		}
		list := make([]pageComputer, 0, len(computers))
		for _, computer := range computers {
			internet, power := server.labelsFor(computer.Name)
			list = append(list, pageComputer{ClassroomComputer: computer, Internet: internet, Power: power, Showing: showing[computer.Name]})
		}
		header.Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]any{"computers": list})
	case "/api/thumbnail":
		name := request.URL.Query().Get("name")
		if !computerNamePattern.MatchString(name) {
			http.Error(writer, "Unknown computer.", http.StatusBadRequest)
			return
		}
		image, found := server.source.Thumbnail(name)
		if !found {
			http.Error(writer, "No image yet.", http.StatusNotFound)
			return
		}
		header.Set("Content-Type", "image/jpeg")
		_, _ = writer.Write(image)
	case "/api/frame":
		name := request.URL.Query().Get("name")
		since, _ := strconv.ParseInt(request.URL.Query().Get("since"), 10, 64)
		if !computerNamePattern.MatchString(name) {
			http.Error(writer, "Unknown computer.", http.StatusBadRequest)
			return
		}
		image, frame, found := server.source.Frame(name, since)
		if !found {
			http.Error(writer, "Unknown computer.", http.StatusNotFound)
			return
		}
		header.Set("X-Frame", strconv.FormatInt(frame, 10))
		if image == nil {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		header.Set("Content-Type", "image/jpeg")
		_, _ = writer.Write(image)
	case "/api/broadcast/frame", "/api/broadcast/stop":
		if request.Method != http.MethodPost || server.broadcasts == nil {
			http.Error(writer, "Not available.", http.StatusMethodNotAllowed)
			return
		}
		id := request.URL.Query().Get("id")
		if request.URL.Path == "/api/broadcast/stop" {
			server.broadcasts.Stop(id)
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		image, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, classroomview.MaxBroadcastBytes))
		if err != nil || len(image) == 0 {
			http.Error(writer, "Invalid picture.", http.StatusBadRequest)
			return
		}
		if err := server.broadcasts.Frame(id, image); err != nil {
			// Gone tells the page that the showing ended (silence, another
			// showing, or the service restarted).
			http.Error(writer, err.Error(), http.StatusGone)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	case "/api/actions/plan", "/api/actions/apply", "/api/share/choose":
		if request.Method != http.MethodPost || server.actions == nil {
			http.Error(writer, "Not available.", http.StatusMethodNotAllowed)
			return
		}
		switch request.URL.Path {
		case "/api/actions/plan":
			server.planAction(writer, request)
		case "/api/actions/apply":
			server.applyAction(writer, request)
		default:
			server.shareChosen(writer, request)
		}
	case "/api/input":
		if request.Method != http.MethodPost {
			http.Error(writer, "Not available.", http.StatusMethodNotAllowed)
			return
		}
		name := request.URL.Query().Get("name")
		if !computerNamePattern.MatchString(name) {
			http.Error(writer, "Unknown computer.", http.StatusBadRequest)
			return
		}
		var body struct {
			Events  []classroomview.InputEvent `json:"events"`
			Release bool                       `json:"release"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 64<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil {
			http.Error(writer, "Invalid input.", http.StatusBadRequest)
			return
		}
		if err := server.source.Input(name, body.Events, body.Release); err != nil {
			http.Error(writer, "The computer did not accept the input.", http.StatusConflict)
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(writer, request)
	}
}

func (server *viewServer) enter(writer http.ResponseWriter, request *http.Request) {
	token := request.URL.Query().Get("token")
	server.mutex.Lock()
	grant, found := server.tokens[token]
	delete(server.tokens, token)
	server.mutex.Unlock()
	if !found || server.now().After(grant.expires) {
		http.Error(writer, "This link has expired. Open the classroom view again from Nixorium.", http.StatusForbidden)
		return
	}
	session, err := randomToken()
	if err != nil {
		http.Error(writer, "The classroom view could not start.", http.StatusInternalServerError)
		return
	}
	server.mutex.Lock()
	server.sessions[session] = viewGrant{expires: server.now().Add(viewSessionTTL), uid: grant.uid}
	server.mutex.Unlock()
	http.SetCookie(writer, &http.Cookie{Name: viewCookie, Value: session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(viewSessionTTL.Seconds())})
	http.Redirect(writer, request, "/", http.StatusSeeOther)
}

func (server *viewServer) authorized(request *http.Request) bool {
	_, ok := server.session(request)
	return ok
}

// session returns the page session of a request.
func (server *viewServer) session(request *http.Request) (viewGrant, bool) {
	cookie, err := request.Cookie(viewCookie)
	if err != nil {
		return viewGrant{}, false
	}
	server.mutex.Lock()
	defer server.mutex.Unlock()
	grant, found := server.sessions[cookie.Value]
	return grant, found && server.now().Before(grant.expires)
}

func (server *viewServer) asset(writer http.ResponseWriter, name, contentType string) {
	content, err := viewAssets.ReadFile(name)
	if err != nil {
		http.Error(writer, "Missing page.", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", contentType)
	_, _ = writer.Write(content)
}

var errViewDisabled = errors.New("The classroom view is not turned on for this laboratory. Ask the administrator. Code: CLASSROOM-VIEW-OFF.")

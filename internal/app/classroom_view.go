package app

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// ClassroomAgentConnector opens a channel to a client's agent.
type ClassroomAgentConnector interface {
	Connect(ctx context.Context, host domain.HostMeta) (classroomview.Session, error)
}

// ClassroomComputer is one card of the teacher's overview.
type ClassroomComputer struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	Detail     string `json:"detail"`
	HasImage   bool   `json:"hasImage"`
	ImageAt    int64  `json:"imageAt,omitempty"`
	ScreenAt   int64  `json:"screenAt,omitempty"`
	imageBytes []byte
}

// Computer states shown to the teacher.
const (
	ClassroomConnecting  = "connecting"
	ClassroomViewing     = "viewing"
	ClassroomNoSession   = "no-session"
	ClassroomNoAgent     = "no-agent"
	ClassroomUnreachable = "unreachable"
	ClassroomFailed      = "failed"
)

// ClassroomViewHub keeps one agent channel per client while the overview is
// open, refreshing thumbnails; every channel closes when nobody looks.
type ClassroomViewHub struct {
	hosts     func(ctx context.Context) ([]domain.HostMeta, error)
	connector ClassroomAgentConnector
	width     int
	interval  time.Duration
	idle      time.Duration

	mutex      sync.Mutex
	computers  map[string]*ClassroomComputer
	running    map[string]bool
	lastViewed time.Time
	now        func() time.Time
}

func NewClassroomViewHub(hosts func(ctx context.Context) ([]domain.HostMeta, error), connector ClassroomAgentConnector) *ClassroomViewHub {
	return &ClassroomViewHub{
		hosts: hosts, connector: connector, width: 320,
		interval: 1500 * time.Millisecond, idle: 15 * time.Second,
		computers: map[string]*ClassroomComputer{}, running: map[string]bool{}, now: time.Now,
	}
}

// Computers lists the clients for the overview and keeps their thumbnails
// refreshing for a while; each call counts as somebody looking.
func (hub *ClassroomViewHub) Computers(ctx context.Context) ([]ClassroomComputer, error) {
	hosts, err := hub.hosts(ctx)
	if err != nil {
		return nil, err
	}
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hub.lastViewed = hub.now()
	result := make([]ClassroomComputer, 0, len(hosts))
	for _, host := range hosts {
		computer, found := hub.computers[host.Name]
		if !found {
			computer = &ClassroomComputer{Name: host.Name, State: ClassroomConnecting}
			hub.computers[host.Name] = computer
		}
		if !hub.running[host.Name] {
			hub.running[host.Name] = true
			go hub.poll(host)
		}
		snapshot := *computer
		snapshot.imageBytes = nil
		result = append(result, snapshot)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// Thumbnail returns the last image of a known computer.
func (hub *ClassroomViewHub) Thumbnail(name string) ([]byte, bool) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	computer, found := hub.computers[name]
	if !found || len(computer.imageBytes) == 0 {
		return nil, false
	}
	return computer.imageBytes, true
}

func (hub *ClassroomViewHub) watched() bool {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	return hub.now().Sub(hub.lastViewed) < hub.idle
}

func (hub *ClassroomViewHub) update(name string, change func(*ClassroomComputer)) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if computer, found := hub.computers[name]; found {
		change(computer)
	}
}

func (hub *ClassroomViewHub) poll(host domain.HostMeta) {
	defer func() {
		hub.mutex.Lock()
		hub.running[host.Name] = false
		hub.mutex.Unlock()
	}()
	var session classroomview.Session
	defer func() {
		if session != nil {
			_ = session.Close()
		}
	}()
	for hub.watched() {
		if session == nil {
			// The channel lives as long as the overview is watched; each
			// exchange has its own timeout in the connector.
			opened, err := hub.connector.Connect(context.Background(), host)
			if err != nil {
				state, detail := classroomFailureState(err)
				hub.update(host.Name, func(computer *ClassroomComputer) { computer.State, computer.Detail = state, detail })
				hub.sleep(5 * hub.interval)
				continue
			}
			session = opened
		}
		reply, err := session.Thumbnail(hub.width)
		switch {
		case err != nil:
			_ = session.Close()
			session = nil
			hub.update(host.Name, func(computer *ClassroomComputer) {
				computer.State, computer.Detail = ClassroomUnreachable, "The computer stopped answering."
			})
		case reply.Type == classroomview.TypeThumbnail:
			at := hub.now().UnixMilli()
			hub.update(host.Name, func(computer *ClassroomComputer) {
				computer.State, computer.Detail = ClassroomViewing, ""
				computer.imageBytes, computer.HasImage = reply.Image, true
				computer.ImageAt, computer.ScreenAt = at, reply.CapturedAt
			})
		case reply.Code == classroomview.CodeNotReady:
		default:
			state, detail := classroomFailureState(classroomview.AgentError{Code: reply.Code})
			hub.update(host.Name, func(computer *ClassroomComputer) { computer.State, computer.Detail = state, detail })
			if reply.Code == classroomview.CodeNoSession || reply.Code == classroomview.CodeNoAgent {
				_ = session.Close()
				session = nil
				hub.sleep(3 * hub.interval)
			}
		}
		hub.sleep(hub.interval)
	}
}

func (hub *ClassroomViewHub) sleep(duration time.Duration) {
	time.Sleep(duration)
}

func classroomFailureState(err error) (string, string) {
	var agentError classroomview.AgentError
	if errors.As(err, &agentError) {
		switch agentError.Code {
		case classroomview.CodeNoSession:
			return ClassroomNoSession, "Nobody is signed in."
		case classroomview.CodeNoAgent:
			return ClassroomNoAgent, "The classroom view is not running on this computer."
		default:
			return ClassroomFailed, "The screen could not be captured."
		}
	}
	return ClassroomUnreachable, "Switched off or not reachable."
}

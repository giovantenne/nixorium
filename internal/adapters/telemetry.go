package adapters

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// Telemetry has fixed production paths. Alternate paths and HTTP transports are
// supplied only by tests, never through environment variables or CLI arguments.
type Telemetry struct {
	Directory   string
	ConfigPath  string
	MachinePath string
	Client      *http.Client
}

func DefaultTelemetry() Telemetry {
	return Telemetry{Directory: "/var/lib/nixorium/telemetry", ConfigPath: "/etc/nixorium/telemetry.json", MachinePath: "/etc/machine-id"}
}
func (t Telemetry) TelemetryConfig() (domain.TelemetryConfig, string, error) {
	var c domain.TelemetryConfig
	read := func(path string) ([]byte, error) {
		f, e := os.Open(path)
		if e != nil {
			return nil, e
		}
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, 8193))
	}
	b, e := read(t.ConfigPath)
	if e != nil {
		return c, "", e
	}
	if domain.DecodeTelemetry(b, &c) != nil || !c.Valid() {
		return c, "", errors.New("invalid telemetry configuration")
	}
	b, e = read(t.MachinePath)
	if e != nil {
		return c, "", e
	}
	machine := strings.TrimSpace(string(b))
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(machine) {
		return c, "", errors.New("invalid machine identity")
	}
	return c, machine, nil
}
func telemetrySafeFile(fd int, mode uint32) error {
	var s unix.Stat_t
	if e := unix.Fstat(fd, &s); e != nil {
		return e
	}
	if s.Uid != uint32(os.Geteuid()) || s.Mode&unix.S_IFMT != mode || s.Mode&0777 != map[uint32]uint32{unix.S_IFDIR: 0700, unix.S_IFREG: 0600}[mode] || (mode == unix.S_IFREG && s.Nlink != 1) {
		return errors.New("unsafe telemetry state ownership or permissions")
	}
	return nil
}
func (t Telemetry) TelemetryTransaction(ctx context.Context, wait, repair bool, fn func(*domain.TelemetryState, func() error) (bool, error)) error {
	dir, e := unix.Open(t.Directory, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if e != nil {
		return errors.New("telemetry state unavailable; apply the controller configuration first")
	}
	defer unix.Close(dir)
	if e = telemetrySafeFile(dir, unix.S_IFDIR); e != nil {
		return e
	}
	lock, e := unix.Openat(dir, "lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if e != nil {
		return e
	}
	defer unix.Close(lock)
	if e = telemetrySafeFile(lock, unix.S_IFREG); e != nil {
		return e
	}
	deadline := time.NewTimer(7 * time.Second)
	defer deadline.Stop()
	for {
		e = unix.Flock(lock, unix.LOCK_EX|unix.LOCK_NB)
		if e == nil {
			break
		}
		if !errors.Is(e, unix.EWOULDBLOCK) || !wait {
			return errors.New("telemetry is busy; retry shortly")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("telemetry is busy; retry shortly")
		case <-time.After(20 * time.Millisecond):
		}
	}
	defer unix.Flock(lock, unix.LOCK_UN)
	s := domain.TelemetryState{SchemaVersion: 1, Consent: "undecided"}
	fd, e := unix.Openat(dir, "state.json", unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if e == nil {
		f := os.NewFile(uintptr(fd), "telemetry-state")
		if e = telemetrySafeFile(fd, unix.S_IFREG); e != nil {
			f.Close()
			return e
		}
		b, readErr := io.ReadAll(io.LimitReader(f, 8193))
		f.Close()
		if readErr != nil {
			return readErr
		}
		if domain.DecodeTelemetry(b, &s) != nil || s.Validate() != nil {
			if !repair {
				return errors.New("telemetry state is invalid; run nixorium telemetry disable to reset consent")
			}
			s = domain.TelemetryState{SchemaVersion: 1, Consent: "undecided"}
		}
	} else if !errors.Is(e, unix.ENOENT) {
		return e
	}
	changed, e := fn(&s, func() error { return saveTelemetryState(dir, s) })
	if e != nil || !changed {
		return e
	}
	return saveTelemetryState(dir, s)
}

func saveTelemetryState(dir int, s domain.TelemetryState) error {
	if e := s.Validate(); e != nil {
		return e
	}
	b, e := json.Marshal(s)
	if e != nil {
		return e
	}
	nonce := make([]byte, 12)
	if _, e = rand.Read(nonce); e != nil {
		return e
	}
	name := ".state-" + hex.EncodeToString(nonce)
	fd, e := unix.Openat(dir, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if e != nil {
		return e
	}
	f := os.NewFile(uintptr(fd), "telemetry-state")
	defer unix.Unlinkat(dir, name, 0)
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = unix.Renameat(dir, name, dir, "state.json"); e != nil {
		return e
	}
	return unix.Fsync(dir)
}
func (t Telemetry) SendTelemetry(ctx context.Context, p domain.TelemetryPayload) string {
	b, e := json.Marshal(p)
	if e != nil {
		return "rejected"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, domain.TelemetryEndpoint, bytes.NewReader(b))
	if e != nil {
		return "rejected"
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Nixorium-Telemetry/1")
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	res, e := client.Do(req)
	if e != nil {
		return "network-unavailable"
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case 204:
		return "sent"
	case 429:
		return "rate-limited"
	}
	if res.StatusCode >= 500 {
		return "server-unavailable"
	}
	return "rejected"
}

// Remember only the historical boolean after existing authenticated probes.
// Telemetry failure never changes the outcome or delays a laboratory operation.
func rememberTelemetryBoot(probes map[string]domain.HostSystemProbe) {
	DefaultTelemetry().rememberBoot(probes)
}

func (t Telemetry) rememberBoot(probes map[string]domain.HostSystemProbe) {
	verified := false
	for _, p := range probes {
		if p.Detail == "" && validGitRevision(p.Revision) && validSystemPath(p.SystemPath) && strings.Contains(p.SystemPath, "-nixos-system-") {
			verified = true
			break
		}
	}
	if !verified {
		return
	}
	_, machine, e := t.TelemetryConfig()
	if e != nil {
		return
	}
	_ = t.TelemetryTransaction(context.Background(), false, false, func(s *domain.TelemetryState, _ func() error) (bool, error) {
		if s.Consent != "enabled" || s.Machine != machine || s.BootVerified {
			return false, nil
		}
		s.BootVerified = true
		return true, nil
	})
}

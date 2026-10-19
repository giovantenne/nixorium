package adapters

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/systemfiles"
)

// Errors of an allowlisted system file read, as the Assistant tool reports them.
var (
	ErrSystemFileRefused     = errors.New("refused_path")
	ErrSystemFileMissing     = errors.New("not_found")
	ErrSystemFileUnreadable  = errors.New("not_readable")
	ErrSystemFileOldComputer = errors.New("computer_needs_update")
)

// ReadSystemFile reads one allowlisted file or directory (ADR 0032). On the
// controller the helper runs as the administrator; a client is read as root
// through the existing strict SSH trust, like diagnostics.
func (Local) ReadSystemFile(ctx context.Context, host domain.HostMeta, controller bool, path string) (systemfiles.Result, error) {
	if controller {
		name, err := os.Hostname()
		if err != nil || name != host.Name {
			return systemfiles.Result{}, errors.New("declared controller is not this computer")
		}
	}
	return readSystemFile(ctx, host, controller, path, runBoundedCommand)
}

func readSystemFile(ctx context.Context, host domain.HostMeta, controller bool, path string, run assistantCommandRunner) (systemfiles.Result, error) {
	if !systemfiles.ValidRequest(path) || !systemfiles.Readable(path) {
		return systemfiles.Result{}, ErrSystemFileRefused
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	name := "/run/current-system/sw/bin/nixorium-host-diagnostics"
	args := []string{"file", path}
	if !controller {
		name = "ssh"
		args = supportSSHArguments(host, 5*time.Second)
		// The path matched a plain character set, so it is one safe word.
		args[len(args)-1] = "nixorium-host-diagnostics file " + path
		args = append([]string{"-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2"}, args...)
	}
	stdout, stderr, err := run(ctx, systemfiles.MaxOutputBytes+1024, name, args...)
	if err != nil || ctx.Err() != nil {
		switch {
		case strings.Contains(stderr, "not readable by the Assistant"):
			return systemfiles.Result{}, ErrSystemFileRefused
		case strings.Contains(stderr, "No such file or directory"):
			return systemfiles.Result{}, ErrSystemFileMissing
		case strings.Contains(stderr, "This account cannot read it"):
			return systemfiles.Result{}, ErrSystemFileUnreadable
		case strings.Contains(stderr, "Choose one supported diagnostics area"):
			return systemfiles.Result{}, ErrSystemFileOldComputer
		}
		return systemfiles.Result{}, errors.New("system file unavailable")
	}
	return systemfiles.Decode([]byte(strings.TrimSpace(stdout)), path)
}

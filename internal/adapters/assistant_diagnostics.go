package adapters

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/hostdiagnostics"
)

func (Local) HostDiagnostics(ctx context.Context, host domain.HostMeta, controller bool, area string) (hostdiagnostics.Report, error) {
	if controller {
		name, err := os.Hostname()
		if err != nil || name != host.Name {
			return hostdiagnostics.Report{}, errors.New("declared controller is not this computer")
		}
	}
	return hostDiagnostics(ctx, host, controller, area, runBoundedCommand)
}
func hostDiagnostics(ctx context.Context, host domain.HostMeta, controller bool, area string, run assistantCommandRunner) (hostdiagnostics.Report, error) {
	if !hostdiagnostics.ValidArea(area) {
		return hostdiagnostics.Report{}, errors.New("unsupported diagnostics area")
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	name := "/run/current-system/sw/bin/nixorium-host-diagnostics"
	args := []string{area}
	if !controller {
		name = "ssh"
		args = supportSSHArguments(host, 5*time.Second)
		args[len(args)-1] = "nixorium-host-diagnostics " + area
		args = append([]string{"-o", "ServerAliveInterval=5", "-o", "ServerAliveCountMax=2"}, args...)
	}
	stdout, stderr, err := run(ctx, hostdiagnostics.MaxBytes, name, args...)
	var exit *exec.ExitError
	if err != nil && errors.As(err, &exit) && exit.ExitCode() == 1 && strings.Contains(stderr, hostdiagnostics.HelperRefusal) {
		return hostdiagnostics.Report{}, hostdiagnostics.ErrHelperOutdated
	}
	if err != nil || ctx.Err() != nil {
		return hostdiagnostics.Report{}, errors.New("authenticated diagnostics unavailable")
	}
	return hostdiagnostics.Decode([]byte(stdout), area, controller)
}

package adapters

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var controllerApplyUnitPattern = regexp.MustCompile(`^nixorium-apply-controller@[0-9a-f]{40}\.service$`)

func (Local) StartSystemUnit(ctx context.Context, unit string) error {
	err := (Local{}).ControlSystemUnit(ctx, "start", unit)
	if err == nil || !strings.Contains(err.Error(), "failed because the control process exited") {
		return err
	}
	// systemctl only says that the job failed; the unit's own messages say
	// why. Read the last ones of this run, bounded, for a cause to act on.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	invocation, _ := run(readCtx, "systemctl", "show", "--property=InvocationID", "--value", unit)
	invocation = strings.TrimSpace(invocation)
	if invocation == "" {
		return err
	}
	journal, _ := run(readCtx, "journalctl", "_SYSTEMD_INVOCATION_ID="+invocation, "--output=cat", "--lines=200", "--no-pager")
	if cause := unitFailureCause(journal); cause != "" {
		return fmt.Errorf("%s (details: journalctl -u %s)", cause, unit)
	}
	return err
}

var unresolvedHostPattern = regexp.MustCompile(`Could not resolve host:? ([A-Za-z0-9.-]+)`)

// unitFailureCause turns the last messages of a failed Nixorium unit into a
// cause an administrator can act on, or "" when none is recognized.
func unitFailureCause(journal string) string {
	lines := strings.Split(strings.TrimSpace(journal), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		switch {
		case unresolvedHostPattern.MatchString(line):
			host := unresolvedHostPattern.FindStringSubmatch(line)[1]
			return "the controller cannot reach the Internet: it could not find " + host + ". Check its network cable and its DNS server, then try again"
		case strings.Contains(line, "No space left on device"):
			return "the controller's disk is full. Free space in Maintenance → Free disk space, then try again"
		}
	}
	for index := len(lines) - 1; index >= 0; index-- {
		if line := strings.TrimSpace(lines[index]); strings.HasPrefix(line, "error:") {
			return "Nix reported: " + strings.TrimSpace(strings.TrimPrefix(line, "error:"))
		}
	}
	return ""
}

func (Local) ControlSystemUnit(ctx context.Context, verb, unit string) error {
	return controlSystemUnit(ctx, managedCoordinationDirectory, checkManagedJobConflict, func(ctx context.Context, arguments ...string) error {
		_, err := run(ctx, "systemctl", arguments...)
		return err
	}, verb, unit)
}

// controlSystemUnit keeps the allowlist and refusal order of
// ControlSystemUnit; the coordination directory, job check and systemctl
// runner are parameters so tests can observe what reaches systemd.
func controlSystemUnit(ctx context.Context, coordination string, jobConflict func(context.Context) error, systemctl func(context.Context, ...string) error, verb, unit string) error {
	allowed := map[string]map[string]bool{
		"nixorium-restart-cache.service":    {"start": true},
		"nixorium-install-secrets.service":  {"start": true},
		"nixorium-apply-controller.service": {"start": true},
		"nixorium-prepare-pxe.service":      {"start": true},
		"nixorium-remote-install.service":   {"start": true},
		"nixorium-pxe.service":              {"start": true, "stop": true, "reset-failed": true},
		"nixorium-pxe-network.service":      {"stop": true, "reset-failed": true},
		"nixorium-pxe-recover.service":      {"start": true},
	}
	if !allowed[unit][verb] && !(verb == "start" && (controllerApplyUnitPattern.MatchString(unit) || cleanupUnitPattern.MatchString(unit))) {
		return fmt.Errorf("system unit action %q %q is not an allowed Nixorium action", verb, unit)
	}
	if verb == "start" && (managedUnitOperation(unit) != "" || unit == "nixorium-pxe.service" || cleanupUnitPattern.MatchString(unit)) {
		if err := jobConflict(ctx); err != nil {
			return err
		}
	}
	// This also protects a newly launched command before the installed systemd
	// scripts have been upgraded to recognize durable deployment evidence.
	if verb == "start" && startRefusedByPendingDeployment(unit) {
		if err := checkDeploymentPendingPath(coordination); err != nil {
			return err
		}
	}
	return systemctl(ctx, systemctlArguments(verb, unit)...)
}

// startRefusedByPendingDeployment reports whether an interrupted client
// update blocks starting unit. Controller activation is exempt (owner
// decision 2026-10-15, ADR 0023 amendment): it changes only the controller,
// not the client facts the record protects, and a newer release must always
// be able to reach a controller whose own recovery may be the bug. The PXE
// network recovery only restores the controller's own addressing.
func startRefusedByPendingDeployment(unit string) bool {
	switch {
	case unit == "nixorium-pxe-recover.service":
		return false
	case unit == "nixorium-apply-controller.service", controllerApplyUnitPattern.MatchString(unit):
		return false
	}
	return true
}

// systemctlArguments appends --no-ask-password: Nixorium relies on sudo and
// polkit rules, and the tty password agent would watch a runtime directory
// that may belong to another user (for example after `su` without a dash).
func systemctlArguments(arguments ...string) []string {
	return append(append([]string{}, arguments...), "--no-ask-password")
}

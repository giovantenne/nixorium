package adapters

import (
	"context"
	"fmt"
	"regexp"
)

var controllerApplyUnitPattern = regexp.MustCompile(`^nixorium-apply-controller@[0-9a-f]{40}\.service$`)

func (Local) StartSystemUnit(ctx context.Context, unit string) error {
	return (Local{}).ControlSystemUnit(ctx, "start", unit)
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
		"nixorium-pxe.service":              {"start": true, "stop": true},
		"nixorium-pxe-network.service":      {"stop": true},
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

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
		if err := checkManagedJobConflict(ctx); err != nil {
			return err
		}
	}
	// This also protects a newly launched command before the installed systemd
	// scripts have been upgraded to recognize durable deployment evidence.
	if verb == "start" && unit != "nixorium-pxe-recover.service" {
		if err := checkDeploymentPendingPath(managedCoordinationDirectory); err != nil {
			return err
		}
	}
	_, err := run(ctx, "systemctl", verb, unit)
	return err
}

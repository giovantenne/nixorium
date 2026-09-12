package adapters

import (
	"context"
	"fmt"
)

func (Local) StartSystemUnit(ctx context.Context, unit string) error {
	return (Local{}).ControlSystemUnit(ctx, "start", unit)
}

func (Local) ControlSystemUnit(ctx context.Context, verb, unit string) error {
	allowed := map[string]map[string]bool{
		"nixorium-install-secrets.service":  {"start": true},
		"nixorium-apply-controller.service": {"start": true},
		"nixorium-prepare-pxe.service":      {"start": true},
		"nixorium-pxe.service":              {"start": true, "stop": true},
		"nixorium-pxe-network.service":      {"stop": true},
		"nixorium-pxe-recover.service":      {"start": true},
	}
	if !allowed[unit][verb] {
		return fmt.Errorf("system unit action %q %q is not an allowed Nixorium action", verb, unit)
	}
	_, err := run(ctx, "systemctl", verb, unit)
	return err
}

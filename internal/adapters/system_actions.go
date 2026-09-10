package adapters

import (
	"context"
	"fmt"
)

func (Local) StartSystemUnit(ctx context.Context, unit string) error {
	allowed := map[string]bool{
		"nixorium-install-secrets.service": true,
	}
	if !allowed[unit] {
		return fmt.Errorf("system unit %q is not an allowed Nixorium action", unit)
	}
	_, err := run(ctx, "systemctl", "start", unit)
	return err
}

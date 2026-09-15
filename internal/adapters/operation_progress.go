package adapters

import (
	"fmt"
)

const maximumOperationProgressBytes = 16 * 1024

var operationProgressPaths = map[string]string{
	"pxe-prepare": "/var/lib/nixorium/prepared/progress.json",
}

func (Local) ReadOperationProgress(operation string) ([]byte, error) {
	path, allowed := operationProgressPaths[operation]
	if !allowed {
		return nil, fmt.Errorf("operation progress %q is not allowed", operation)
	}
	return readOperationProgressFile(path)
}

func readOperationProgressFile(path string) ([]byte, error) {
	content, mode, err := readRegularFileNoFollowLimit(path, maximumOperationProgressBytes)
	if err != nil {
		return nil, err
	}
	if mode != 0600 {
		return nil, fmt.Errorf("operation progress has unsafe mode %04o; expected 0600", mode)
	}
	return content, nil
}

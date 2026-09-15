package app

import (
	"fmt"

	"github.com/giovantenne/nixorium/internal/domain"
)

type OperationProgressSource interface {
	ReadOperationProgress(operation string) ([]byte, error)
}

type OperationProgressManager struct {
	source OperationProgressSource
}

func NewOperationProgressManager(source OperationProgressSource) OperationProgressManager {
	return OperationProgressManager{source: source}
}

func (m OperationProgressManager) Current(operation string) (domain.OperationProgress, error) {
	data, err := m.source.ReadOperationProgress(operation)
	if err != nil {
		return domain.OperationProgress{}, fmt.Errorf("read %s progress: %w", operation, err)
	}
	progress, err := domain.DecodeOperationProgress(data)
	if err != nil {
		return domain.OperationProgress{}, err
	}
	if progress.Operation != operation {
		return domain.OperationProgress{}, fmt.Errorf("progress operation %q does not match requested operation %q", progress.Operation, operation)
	}
	return progress, nil
}

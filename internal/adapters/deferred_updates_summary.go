package adapters

import "context"

// DeferredUpdateSummary counts queued client updates for the Overview from
// the queue file and the repository revision only.
func (local Local) DeferredUpdateSummary(ctx context.Context, repository string) (waiting, attention int) {
	queue, err := ManagedDeferredUpdates().Read()
	if err != nil || len(queue.Updates) == 0 {
		return 0, 0
	}
	revision, revisionErr := local.GitRevision(ctx, repository)
	for _, update := range queue.Updates {
		// A changed configuration or repeated failures need a look.
		if revisionErr == nil && update.Stale(revision) || update.Attempts >= 3 {
			attention++
		} else {
			waiting++
		}
	}
	return waiting, attention
}

package domain

// ManagedJob is an observation, never authorization or activation evidence.
// Progress is populated only when it belongs to the observed unit invocation.
type ManagedJob struct {
	Operation string
	Unit      string
	State     string // idle, running, interrupted, completed, failed, unknown
	Progress  OperationProgress
	Detail    string
}

func (job ManagedJob) BlocksStart() bool {
	return job.State == "running" || job.State == "unknown"
}

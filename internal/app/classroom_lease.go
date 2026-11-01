package app

import "io"

// classroomLeaseSource lets classroom controls (Internet access, power) run
// beside maintenance: instead of waiting for the operation gate they leave
// alone only the computers named in an unfinished client update. Sources
// without it keep the shared operation gate.
type classroomLeaseSource interface {
	ClassroomBlockedHosts() (map[string]string, error)
	AcquireClassroomLease(label string) (io.Closer, map[string]string, error)
}

// classroomLeaseFor returns the lease source when the given source offers it.
func classroomLeaseFor(source any) (classroomLeaseSource, bool) {
	lease, ok := source.(classroomLeaseSource)
	return lease, ok
}

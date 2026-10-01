package app

import "io"

type labeledOperationSource interface {
	AcquireClientOperationFor(string) (io.Closer, error)
}

// acquireOperation takes the shared gate, naming the operation when the
// source supports it.
func acquireOperation(source interface {
	AcquireClientOperation() (io.Closer, error)
}, label string) (io.Closer, error) {
	if labeled, ok := source.(labeledOperationSource); ok {
		return labeled.AcquireClientOperationFor(label)
	}
	return source.AcquireClientOperation()
}

// operationRefusal turns the shared gate observation into the message shown
// to the operator. A busy gate may explain itself (owner, pending deployment,
// USB reservation); otherwise the caller's fallback is used.
func operationRefusal(active bool, err error, fallback, inspectPrefix string) (string, bool) {
	switch {
	case active && err != nil:
		return err.Error(), true
	case active:
		return fallback, true
	case err != nil:
		return inspectPrefix + err.Error(), true
	}
	return "", false
}

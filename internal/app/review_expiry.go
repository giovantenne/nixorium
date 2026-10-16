package app

import "time"

// reviewGrid is the step on which rebuildable reviews expire. A CLI apply
// rebuilds the review it is asked to confirm, so the expiry must be derivable
// from the clock; a fine grid keeps a review valid for nearly its whole
// lifetime even when it was created just before a grid line.
const reviewGrid = time.Minute

// reviewExpiry is the expiry of a review created now.
func reviewExpiry(now time.Time, lifetime time.Duration) time.Time {
	return now.UTC().Truncate(reviewGrid).Add(lifetime)
}

// reviewExpiries lists, newest first, every expiry that a review created
// within the last lifetime can carry and that has not passed yet. Matching a
// token against these never extends a review beyond its own expiry.
func reviewExpiries(now time.Time, lifetime time.Duration) []time.Time {
	var expiries []time.Time
	for expiry := reviewExpiry(now, lifetime); expiry.After(now); expiry = expiry.Add(-reviewGrid) {
		expiries = append(expiries, expiry)
	}
	return expiries
}

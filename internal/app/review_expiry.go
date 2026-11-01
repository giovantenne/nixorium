package app

import "time"

// reviewGrid is the step on which rebuildable reviews expire. A CLI apply
// rebuilds the review it is asked to confirm, so the expiry must be derivable
// from the clock; a fine grid keeps the extra validity small.
const reviewGrid = time.Minute

// reviewExpiredLookback bounds how far back an expired review is still
// recognised, so that the operator is told the review expired instead of
// being told it changed.
const reviewExpiredLookback = 24 * time.Hour

// ReviewMatch is the outcome of matching a confirmed review token against a
// freshly built plan.
type ReviewMatch int

const (
	// ReviewChanged means the token does not belong to this plan: something
	// other than time changed since the review.
	ReviewChanged ReviewMatch = iota
	// ReviewMatched means the token belongs to this plan and is still valid.
	ReviewMatched
	// ReviewExpired means the token belongs to this plan but ran out.
	ReviewExpired
)

// reviewExpiry is the expiry of a review created now. The expiry is rounded up
// to the grid, so a review is valid for at least lifetime and for less than
// lifetime plus one grid step, however close to a grid line it was created.
func reviewExpiry(now time.Time, lifetime time.Duration) time.Time {
	return now.UTC().Truncate(reviewGrid).Add(reviewGrid + lifetime)
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

// matchReview finds the expiry under which fresh, a plan rebuilt now, carries
// token. rebuild returns the plan with that expiry and its token; tokenOf reads
// the token back. A match under a live expiry returns that plan and
// ReviewMatched; a match under an expiry that has passed returns ReviewExpired.
func matchReview[P any](now time.Time, lifetime time.Duration, fresh P, token string, rebuild func(P, time.Time) P, tokenOf func(P) string) (P, ReviewMatch) {
	if token == "" {
		return fresh, ReviewChanged
	}
	live := reviewExpiries(now, lifetime)
	for _, expiry := range live {
		if candidate := rebuild(fresh, expiry); tokenOf(candidate) == token {
			return candidate, ReviewMatched
		}
	}
	oldest := reviewExpiry(now, lifetime)
	if len(live) > 0 {
		oldest = live[len(live)-1]
	}
	for expiry := oldest.Add(-reviewGrid); expiry.After(now.Add(-reviewExpiredLookback)); expiry = expiry.Add(-reviewGrid) {
		if candidate := rebuild(fresh, expiry); tokenOf(candidate) == token {
			return candidate, ReviewExpired
		}
	}
	return fresh, ReviewChanged
}

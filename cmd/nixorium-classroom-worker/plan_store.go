package main

import (
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// maxStoredPlans bounds the reviews waiting for their apply.
const maxStoredPlans = 256

var errUnknownReview = errors.New("the review is unknown, already used or expired")

// planStore keeps every plan the worker issued until it is applied or expires.
// A review token is an unkeyed digest of its plan, so a caller that sends a
// plan back could forge plan and token together: the apply therefore uses the
// plan kept here, found by its token, and never the one in the request.
type planStore struct {
	mutex sync.Mutex
	plans map[string]storedPlan
}

type storedPlan struct {
	content []byte
	expires time.Time
}

func newPlanStore() *planStore { return &planStore{plans: map[string]storedPlan{}} }

// rememberPlan keeps a copy of plan under its token until it expires. A plan
// without a token or an expiry is not kept and so can never be applied.
func rememberPlan[T any](store *planStore, kind, token string, expires time.Time, plan T) {
	if store == nil || token == "" || expires.IsZero() {
		return
	}
	content, err := json.Marshal(plan)
	if err != nil {
		return
	}
	store.mutex.Lock()
	defer store.mutex.Unlock()
	now := time.Now()
	for key, entry := range store.plans {
		if !entry.expires.After(now) {
			delete(store.plans, key)
		}
	}
	for len(store.plans) >= maxStoredPlans {
		oldest, found := "", false
		for key, entry := range store.plans {
			if !found || entry.expires.Before(store.plans[oldest].expires) {
				oldest, found = key, true
			}
		}
		delete(store.plans, oldest)
	}
	store.plans[kind+"|"+token] = storedPlan{content: content, expires: expires}
}

// takePlan returns the plan issued under token and forgets it, so a review
// applies once.
func takePlan[T any](store *planStore, kind, token string) (T, error) {
	var plan T
	if store == nil {
		return plan, errUnknownReview
	}
	key := kind + "|" + token
	store.mutex.Lock()
	entry, found := store.plans[key]
	delete(store.plans, key)
	store.mutex.Unlock()
	if !found || !entry.expires.After(time.Now()) || json.Unmarshal(entry.content, &plan) != nil {
		var zero T
		return zero, errUnknownReview
	}
	return plan, nil
}

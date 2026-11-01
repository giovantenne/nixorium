package main

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

// The page and the classroom socket send plans back, but only the plan the
// worker issued is ever applied, and only once.
func TestApplyUsesOnlyThePlanTheWorkerIssued(t *testing.T) {
	worker := collectWorker(t)
	teacher := adapters.WithClassroomPeerUID(context.Background(), 1000)
	send := func(operation domain.ClassroomOperation, configure func(*domain.ClassroomRequest)) domain.ClassroomResponse {
		message, err := domain.NewClassroomRequest(operation)
		if err != nil {
			t.Fatal(err)
		}
		configure(&message)
		return worker.handle(teacher, message)
	}
	issued := send(domain.ClassroomCollectPlanOperation, func(message *domain.ClassroomRequest) {
		message.Requested, message.CollectFolder = "pc01", "/home/teacher/Homework"
	})
	if issued.CollectPlan == nil || issued.CollectPlan.HasErrors() {
		t.Fatalf("plan = %+v", issued)
	}
	// A plan changed by the caller, with a token computed locally to match.
	forged := *issued.CollectPlan
	forged.Destination = "/home/teacher/elsewhere"
	forged.ReviewToken = domain.CollectReviewToken(forged)
	refused := send(domain.ClassroomCollectApplyOperation, func(message *domain.ClassroomRequest) { message.CollectPlan = &forged })
	if refused.State != "failed" || refused.CollectReport != nil {
		t.Fatalf("forged plan = %+v", refused)
	}
	// With the issued token the stored plan is applied whatever the body says.
	tampered := *issued.CollectPlan
	tampered.Destination = "/home/teacher/elsewhere"
	applied := send(domain.ClassroomCollectApplyOperation, func(message *domain.ClassroomRequest) { message.CollectPlan = &tampered })
	if applied.CollectReport == nil || applied.CollectReport.State == "failed" {
		t.Fatalf("apply = %+v", applied)
	}
	// A review applies once.
	replay := send(domain.ClassroomCollectApplyOperation, func(message *domain.ClassroomRequest) { message.CollectPlan = issued.CollectPlan })
	if replay.State != "failed" || replay.CollectReport != nil {
		t.Fatalf("replay = %+v", replay)
	}
}

func TestPlanStoreExpiresAndStaysBounded(t *testing.T) {
	store := newPlanStore()
	rememberPlan(store, "lock", "sha256:old", time.Now().Add(-time.Second), domain.LockPlan{})
	if _, err := takePlan[domain.LockPlan](store, "lock", "sha256:old"); err == nil {
		t.Fatal("an expired plan was returned")
	}
	rememberPlan(store, "lock", "", time.Now().Add(time.Minute), domain.LockPlan{})
	rememberPlan(store, "lock", "sha256:a", time.Time{}, domain.LockPlan{})
	if len(store.plans) != 0 {
		t.Fatalf("plans without token or expiry were kept: %d", len(store.plans))
	}
	for index := 0; index < 3*maxStoredPlans; index++ {
		rememberPlan(store, "lock", "sha256:"+strconv.Itoa(index), time.Now().Add(time.Minute+time.Duration(index)), domain.LockPlan{})
	}
	if len(store.plans) > maxStoredPlans {
		t.Fatalf("store holds %d plans", len(store.plans))
	}
	rememberPlan(store, "lock", "sha256:kept", time.Now().Add(time.Hour), domain.LockPlan{Repository: "/srv/lab"})
	if _, err := takePlan[domain.SharePlan](store, "share", "sha256:kept"); err == nil {
		t.Fatal("a plan of another kind was returned")
	}
	if plan, err := takePlan[domain.LockPlan](store, "lock", "sha256:kept"); err != nil || plan.Repository != "/srv/lab" {
		t.Fatalf("kept plan = %+v, %v", plan, err)
	}
}

package domain

import (
	"strings"
	"testing"
	"time"
)

func deferredFixture(host string) DeferredUpdate {
	return DeferredUpdate{Host: host, IP: "10.0.0.1", Revision: strings.Repeat("a", 40), QueuedAt: time.Unix(1_800_000_000, 0).UTC()}
}

func TestDeferredUpdateQueueKeepsOneEntryPerComputer(t *testing.T) {
	queue := DeferredUpdateQueue{SchemaVersion: DeferredUpdateSchemaVersion}
	queue.Put(deferredFixture("pc02"))
	queue.Put(deferredFixture("pc01"))
	newer := deferredFixture("pc02")
	newer.Revision = strings.Repeat("b", 40)
	queue.Put(newer)
	if len(queue.Updates) != 2 || queue.Updates[0].Host != "pc01" || queue.Updates[1].Revision != newer.Revision {
		t.Fatalf("queue = %+v", queue.Updates)
	}
	if err := queue.Validate(); err != nil {
		t.Fatal(err)
	}
	if !queue.Remove("pc01") || queue.Remove("pc01") || len(queue.Updates) != 1 {
		t.Fatalf("remove = %+v", queue.Updates)
	}
}

func TestDeferredUpdateQueueRejectsInvalidRecords(t *testing.T) {
	for name, mutate := range map[string]func(*DeferredUpdate){
		"host":     func(u *DeferredUpdate) { u.Host = "../pc01" },
		"address":  func(u *DeferredUpdate) { u.IP = "pc01" },
		"revision": func(u *DeferredUpdate) { u.Revision = "main" },
		"time":     func(u *DeferredUpdate) { u.QueuedAt = time.Time{} },
	} {
		update := deferredFixture("pc01")
		mutate(&update)
		queue := DeferredUpdateQueue{SchemaVersion: DeferredUpdateSchemaVersion, Updates: []DeferredUpdate{update}}
		if queue.Validate() == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	duplicate := DeferredUpdateQueue{SchemaVersion: DeferredUpdateSchemaVersion, Updates: []DeferredUpdate{deferredFixture("pc01"), deferredFixture("pc01")}}
	if duplicate.Validate() == nil {
		t.Fatal("duplicate host accepted")
	}
}

func TestDeferredUpdateBacksOffAndStaysOnItsRevision(t *testing.T) {
	update := deferredFixture("pc01")
	start := time.Unix(1_800_000_000, 0)
	if !update.Due(start) || update.Stale(strings.Repeat("a", 40)) || !update.Stale(strings.Repeat("c", 40)) {
		t.Fatal("fresh entry state")
	}
	update.RecordFailure(start, "ssh:\n  connection refused")
	if update.Due(start.Add(30*time.Second)) || !update.Due(start.Add(time.Minute)) || update.LastError != "ssh: connection refused" {
		t.Fatalf("after one failure: %+v", update)
	}
	for range 10 {
		update.RecordFailure(start, strings.Repeat("x", 2000))
	}
	if update.Due(start.Add(59*time.Minute)) || !update.Due(start.Add(time.Hour)) || len(update.LastError) > 512 {
		t.Fatalf("backoff is not bounded to an hour: %+v", update.Attempts)
	}
}

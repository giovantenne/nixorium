package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

func offerTestSave(t *testing.T, broker *desktopBroker, uid int, items []domain.ShareFile, released *int) string {
	t.Helper()
	read := func(index int, offset int64, size int) ([]byte, error) {
		return []byte(strings.Repeat("x", size)), nil
	}
	id, err := broker.Offer(uid, domain.DesktopSaveScreenshots, items, read, func() { *released++ })
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSavesAreReadOnlyByTheirOwnUser(t *testing.T) {
	broker := newDesktopBroker()
	released := 0
	items := []domain.ShareFile{{Path: "pc01-2026-10-15-10.30.12.jpg", Size: domain.ClassroomShareChunkBytes + 10}}
	id := offerTestSave(t, broker, 1000, items, &released)
	if _, _, _, err := broker.SaveItems(1001, id, 0); err == nil {
		t.Fatal("another user listed the save")
	}
	if _, err := broker.SaveRead(1001, id, 0, 0); err == nil {
		t.Fatal("another user read the save")
	}
	if broker.SaveDone(1001, id, []string{"x"}, "") || released != 0 {
		t.Fatal("another user ended the save")
	}
	kind, page, total, err := broker.SaveItems(1000, id, 0)
	if err != nil || kind != domain.DesktopSaveScreenshots || total != 1 || len(page) != 1 {
		t.Fatalf("items = %q %+v %d %v", kind, page, total, err)
	}
	if data, err := broker.SaveRead(1000, id, 0, 0); err != nil || len(data) != domain.ClassroomShareChunkBytes {
		t.Fatalf("first piece = %d, %v", len(data), err)
	}
	if data, err := broker.SaveRead(1000, id, 0, domain.ClassroomShareChunkBytes); err != nil || len(data) != 10 {
		t.Fatalf("last piece = %d, %v", len(data), err)
	}
	for name, position := range map[string][2]int64{"past the end": {0, domain.ClassroomShareChunkBytes + 10}, "negative": {0, -1}, "no such item": {1, 0}} {
		if _, err := broker.SaveRead(1000, id, int(position[0]), position[1]); err == nil {
			t.Fatalf("%s was read", name)
		}
	}
	if !broker.SaveDone(1000, id, []string{"Nixorium/Screenshots/pc01-2026-10-15-10.30.12.jpg"}, "") || released != 1 {
		t.Fatal("the save did not end")
	}
	if _, _, _, err := broker.SaveItems(1000, id, 0); err == nil {
		t.Fatal("an ended save is still listed")
	}
}

func TestSaveItemsComeInPages(t *testing.T) {
	broker := newDesktopBroker()
	released := 0
	items := []domain.ShareFile{}
	for index := 0; index < domain.ClassroomSaveItemsPage+5; index++ {
		items = append(items, domain.ShareFile{Path: fmt.Sprintf("f%d", index), Size: 1})
	}
	id := offerTestSave(t, broker, 1000, items, &released)
	_, first, total, err := broker.SaveItems(1000, id, 0)
	if err != nil || len(first) != domain.ClassroomSaveItemsPage || total != len(items) {
		t.Fatalf("first page = %d %d %v", len(first), total, err)
	}
	_, second, _, err := broker.SaveItems(1000, id, len(first))
	if err != nil || len(second) != 5 || second[0].Path != fmt.Sprintf("f%d", domain.ClassroomSaveItemsPage) {
		t.Fatalf("second page = %+v %v", second, err)
	}
	long := []domain.ShareFile{}
	for index := 0; index < 20; index++ {
		long = append(long, domain.ShareFile{Path: fmt.Sprintf("%08000d", index), Size: 1})
	}
	id = offerTestSave(t, broker, 1000, long, &released)
	if _, page, _, _ := broker.SaveItems(1000, id, 0); len(page) == 0 || len(page) >= 20 {
		t.Fatalf("a page of long names has %d items", len(page))
	}
	if _, _, _, err := broker.SaveItems(1000, id, 21); err == nil {
		t.Fatal("a page past the end was returned")
	}
}

func TestSavesGoToTheUsersHelper(t *testing.T) {
	broker := newDesktopBroker()
	released := 0
	items := []domain.ShareFile{{Path: "pc01.jpg", Size: 3}}
	// Without a helper the save stays for its user.
	id := offerTestSave(t, broker, 1000, items, &released)
	if broker.Available(1000) {
		t.Fatal("a helper was reported without one")
	}
	if _, err := broker.Deliver(context.Background(), id, time.Second, time.Second); !errors.Is(err, errNoDesktopHelper) {
		t.Fatalf("no helper = %v", err)
	}
	broker.Withdraw(id)
	if released != 1 {
		t.Fatal("a withdrawn save was not released")
	}
	// Another user's helper never gets the job; the user's own does.
	id = offerTestSave(t, broker, 1000, items, &released)
	go func() {
		if job := broker.Wait(context.Background(), 1001); job.id != "" {
			t.Errorf("another user's helper got %+v", job)
		}
	}()
	go func() {
		job := broker.Wait(context.Background(), 1000)
		if job.id != id || job.kind != domain.DesktopSaveScreenshots {
			t.Errorf("job = %+v", job)
		}
		broker.SaveDone(1000, job.id, []string{"Nixorium/Screenshots/pc01.jpg"}, "")
	}()
	time.Sleep(50 * time.Millisecond)
	if !broker.Available(1000) {
		t.Fatal("the waiting helper was not seen")
	}
	outcome, err := broker.Deliver(context.Background(), id, 2*time.Second, 5*time.Second)
	if err != nil || len(outcome.names) != 1 || outcome.names[0] != "Nixorium/Screenshots/pc01.jpg" || released != 2 {
		t.Fatalf("outcome = %+v, %v", outcome, err)
	}
	// An older helper answers a save job as a file choice with a message.
	id = offerTestSave(t, broker, 1000, items, &released)
	go func() {
		job := broker.Wait(context.Background(), 1000)
		broker.Ready(1000, job.id, "", "The page asked for an unknown choice.", false)
	}()
	time.Sleep(50 * time.Millisecond)
	if outcome, err := broker.Deliver(context.Background(), id, 2*time.Second, 5*time.Second); err != nil || outcome.message == "" || len(outcome.names) != 0 {
		t.Fatalf("older helper = %+v, %v", outcome, err)
	}
}

func TestUnusedSavesAreReleased(t *testing.T) {
	broker := newDesktopBroker()
	released := 0
	id := offerTestSave(t, broker, 1000, []domain.ShareFile{{Path: "a", Size: 1}}, &released)
	broker.saves[id].used = time.Now().Add(-saveIdle - time.Minute)
	broker.pruneSaves()
	if released != 1 {
		t.Fatal("an unused save was kept")
	}
	if _, err := broker.SaveRead(1000, id, 0, 0); err == nil {
		t.Fatal("an expired save was read")
	}
	for range saveLimit {
		offerTestSave(t, broker, 1000, []domain.ShareFile{{Path: "a", Size: 1}}, &released)
	}
	if _, err := broker.Offer(1000, domain.DesktopSaveScreenshots, nil, nil, func() {}); err == nil {
		t.Fatal("more saves than allowed")
	}
}

// helperCopy copies a save into folder like the desktop helper does.
func helperCopy(broker *desktopBroker, uid int, id, folder string) ([]string, error) {
	if err := classroomview.EnsureFolder(folder); err != nil {
		return nil, err
	}
	_, items, _, err := broker.SaveItems(uid, id, 0)
	if err != nil {
		return nil, err
	}
	entries := []classroomview.FileEntry{}
	for _, item := range items {
		entries = append(entries, classroomview.FileEntry{Path: item.Path, Size: item.Size, Dir: item.Dir})
	}
	writer, err := classroomview.NewTreeWriter(folder, entries)
	if err != nil {
		return nil, err
	}
	for index, entry := range entries {
		for offset := int64(0); offset < entry.Size && !entry.Dir; {
			data, err := broker.SaveRead(uid, id, index, offset)
			if err != nil {
				writer.Abort()
				return nil, err
			}
			if err := writer.Write(index, data); err != nil {
				writer.Abort()
				return nil, err
			}
			offset += int64(len(data))
		}
	}
	return writer.Finish()
}

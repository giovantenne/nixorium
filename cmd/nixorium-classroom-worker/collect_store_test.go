package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/app"
	"github.com/giovantenne/nixorium/internal/classroomview"
)

func testCollectStore(t *testing.T) *collectStore {
	t.Helper()
	store := newCollectStore(t.TempDir())
	store.reserve = 0
	store.free = func(string) (int64, error) { return 1 << 40, nil }
	return store
}

var collectedEntries = []classroomview.FileEntry{{Path: "Homework", Dir: true}, {Path: "Homework/essay.odt", Size: 5}, {Path: "empty.txt"}, {Path: "notes.txt", Size: 3}}

func fillPieces(pieces ...struct {
	index int
	data  string
}) func(func(int, []byte) error) error {
	return func(write func(int, []byte) error) error {
		for _, piece := range pieces {
			if err := write(piece.index, []byte(piece.data)); err != nil {
				return err
			}
		}
		return nil
	}
}

type piece = struct {
	index int
	data  string
}

func TestCollectStoreKeepsCompleteComputersByPosition(t *testing.T) {
	store := testCollectStore(t)
	id, err := store.Begin("2026-10-15 10.30")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Store(id, "pc02", collectedEntries, fillPieces(piece{1, "ess"}, piece{1, "ay"}, piece{3, "abc"})); err != nil {
		t.Fatal(err)
	}
	if err := store.Store(id, "pc01", []classroomview.FileEntry{{Path: "a.txt", Size: 1}}, fillPieces(piece{0, "x"})); err != nil {
		t.Fatal(err)
	}
	// Files are stored by position, never under the names a computer sent.
	directory := store.collections[id].directory
	if _, err := os.Stat(filepath.Join(directory, "pc02", "1")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, "pc02", "Homework")); !os.IsNotExist(err) {
		t.Fatal("a file was stored under its own name")
	}
	items, err := store.Items(id)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, item := range items {
		got = append(got, fmt.Sprintf("%s:%d:%t", item.Path, item.Size, item.Dir))
	}
	want := "[2026-10-15 10.30:0:true 2026-10-15 10.30/pc01:0:true 2026-10-15 10.30/pc01/a.txt:1:false 2026-10-15 10.30/pc02:0:true 2026-10-15 10.30/pc02/Homework:0:true 2026-10-15 10.30/pc02/Homework/essay.odt:5:false 2026-10-15 10.30/pc02/empty.txt:0:false 2026-10-15 10.30/pc02/notes.txt:3:false]"
	if fmt.Sprint(got) != want {
		t.Fatalf("items = %v", got)
	}
	if data, err := store.Read(id, items[5], 2, 3); err != nil || string(data) != "say" {
		t.Fatalf("read = %q, %v", data, err)
	}
	for name, item := range map[string]collectedItem{"past the end": items[5], "a folder": items[4], "outside": {file: "/etc/passwd"}} {
		if _, err := store.Read(id, item, 4, 3); err == nil {
			t.Fatalf("%s was read", name)
		}
	}
	store.Remove(id)
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("a removed collection kept its files")
	}
	if _, err := store.Items(id); err == nil {
		t.Fatal("a removed collection is still listed")
	}
}

func TestCollectStoreKeepsNothingOfABrokenComputer(t *testing.T) {
	store := testCollectStore(t)
	id, err := store.Begin("2026-10-15 10.30")
	if err != nil {
		t.Fatal(err)
	}
	for name, fill := range map[string]func(func(int, []byte) error) error{
		"incomplete":   fillPieces(piece{1, "essay"}),
		"too long":     fillPieces(piece{1, "essay!"}),
		"out of order": fillPieces(piece{3, "abc"}, piece{1, "essay"}),
		"a folder":     fillPieces(piece{0, "x"}),
		"interrupted":  func(func(int, []byte) error) error { return errors.New("connection lost") },
	} {
		if err := store.Store(id, "pc01", collectedEntries, fill); err == nil {
			t.Fatalf("%s was kept", name)
		}
		if _, err := os.Stat(filepath.Join(store.collections[id].directory, "pc01")); !os.IsNotExist(err) {
			t.Fatalf("%s left files behind", name)
		}
	}
	for name, entries := range map[string][]classroomview.FileEntry{
		"traversal": {{Path: "../../x", Size: 1}},
		"absolute":  {{Path: "/etc/x", Size: 1}},
		"backslash": {{Path: `..\x`, Size: 1}},
		"too large": {{Path: "a", Size: classroomview.MaxFileBytes + 1}},
	} {
		if err := store.Store(id, "pc01", entries, fillPieces()); err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	if err := store.Store(id, "../pc01", collectedEntries, fillPieces()); err == nil {
		t.Fatal("an unsafe computer name was accepted")
	}
	if _, err := store.Items(id); err == nil {
		t.Fatal("a collection with nothing kept is listed")
	}
	// The controller keeps its reserve of free space.
	store.reserve = 100
	store.free = func(string) (int64, error) { return 110, nil }
	if err := store.Store(id, "pc01", collectedEntries, fillPieces(piece{1, "essay"}, piece{3, "abc"})); !errors.Is(err, app.ErrCollectNoSpace) {
		t.Fatalf("no space = %v", err)
	}
}

func TestCollectStoreLimitsAndExpiry(t *testing.T) {
	store := testCollectStore(t)
	if _, err := store.Begin("../x"); err == nil {
		t.Fatal("an unsafe folder name was accepted")
	}
	ids := []string{}
	for range collectSets {
		id, err := store.Begin("2026-10-15 10.30")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if _, err := store.Begin("2026-10-15 10.30"); err == nil {
		t.Fatal("more collections than allowed")
	}
	directory := store.collections[ids[0]].directory
	store.now = func() time.Time { return time.Now().Add(collectIdle + time.Minute) }
	if _, err := store.Begin("2026-10-15 10.30"); err != nil {
		t.Fatalf("unused collections were not removed: %v", err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("an expired collection kept its files")
	}
}

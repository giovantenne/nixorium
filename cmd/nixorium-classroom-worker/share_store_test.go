package main

import (
	"io"
	"os"
	"testing"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
)

func TestShareStorePreparesFilesInOrder(t *testing.T) {
	store := newShareStore(t.TempDir())
	entries := []classroomview.FileEntry{{Path: "Lesson", Dir: true}, {Path: "Lesson/a.txt", Size: 5}, {Path: "empty"}}
	id, err := store.Begin(entries)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Files(id); err == nil {
		t.Fatal("an incomplete transfer was listed")
	}
	for name, err := range map[string]error{
		"folder":       store.Write(id, 0, 0, []byte("x")),
		"wrong offset": store.Write(id, 1, 2, []byte("x")),
		"too large":    store.Write(id, 1, 0, []byte("123456")),
		"unknown":      store.Write("nope", 1, 0, []byte("x")),
	} {
		if err == nil {
			t.Fatalf("%s was accepted", name)
		}
	}
	if err := store.Write(id, 1, 0, []byte("he")); err != nil {
		t.Fatal(err)
	}
	if err := store.Write(id, 1, 2, []byte("llo")); err != nil {
		t.Fatal(err)
	}
	files, err := store.Files(id)
	if err != nil || len(files) != 3 || files[1].SHA256 != "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824" || files[2].SHA256 != emptyDigest || files[0].SHA256 != "" {
		t.Fatalf("files = %+v, %v", files, err)
	}
	reader, err := store.Open(id, 1)
	if err != nil {
		t.Fatal(err)
	}
	content, _ := io.ReadAll(reader)
	_ = reader.Close()
	if string(content) != "hello" {
		t.Fatalf("content = %q", content)
	}
	directory := store.transfers[id].directory
	store.Remove(id)
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatal("a removed transfer kept its files")
	}
}

func TestShareStoreLimitsAndExpiry(t *testing.T) {
	store := newShareStore(t.TempDir())
	if _, err := store.Begin([]classroomview.FileEntry{{Path: "../x", Size: 1}}); err == nil {
		t.Fatal("an unsafe name was accepted")
	}
	entries := []classroomview.FileEntry{{Path: "a", Size: 1}}
	ids := []string{}
	for range shareTransfers {
		id, err := store.Begin(entries)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if _, err := store.Begin(entries); err == nil {
		t.Fatal("more transfers than allowed")
	}
	store.now = func() time.Time { return time.Now().Add(shareIdle + time.Minute) }
	if _, err := store.Begin(entries); err != nil {
		t.Fatalf("expired transfers were not pruned: %v", err)
	}
	if _, err := store.Files(ids[0]); err == nil {
		t.Fatal("an expired transfer is still listed")
	}
}

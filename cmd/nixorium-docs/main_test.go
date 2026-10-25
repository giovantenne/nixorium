package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGalleryRenderingIsDeterministicAndSemantic(t *testing.T) {
	first := renderGallery()
	second := renderGallery()
	if first != second {
		t.Fatal("identical documentation fixtures produced different bytes")
	}
	for _, expected := range []string{
		"[F3] Search",
		"Profile review",
		"4 add · 0 already configured · 1 excluded",
		"Opened from a saved software change",
		"5 computers updated",
		"Enter starts network installation mode",
		"in use: those sessions will be shut down",
		"Logged in, not in use",
		"Restricted teacher dashboard",
	} {
		if !strings.Contains(first, expected) {
			t.Fatalf("generated gallery omits %q", expected)
		}
	}
	for _, forbidden := range []string{"/home/", "file://", "2026-09-23T"} {
		if strings.Contains(first, forbidden) {
			t.Fatalf("generated gallery contains environment data %q", forbidden)
		}
	}
}

func TestGalleryCheckIsNonMutatingAndWriteRepairsOnlyGeneratedRegion(t *testing.T) {
	repository := t.TempDir()
	path := filepath.Join(repository, galleryPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "# Manual title\n\nKeep this prose.\n\n" + generatedStart + "\nstale\n" + generatedEnd + "\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := updateGallery(repository, false)
	if err != nil || !changed {
		t.Fatalf("stale check = changed %t, err %v", changed, err)
	}
	afterCheck, err := os.ReadFile(path)
	if err != nil || string(afterCheck) != original {
		t.Fatal("check mode modified the gallery")
	}
	changed, err = updateGallery(repository, true)
	if err != nil || !changed {
		t.Fatalf("write = changed %t, err %v", changed, err)
	}
	afterWrite, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(afterWrite), "Keep this prose.") || !strings.Contains(string(afterWrite), "## Additive software profile review") {
		t.Fatalf("write replaced manual prose or omitted generated content:\n%s", afterWrite)
	}
	changed, err = updateGallery(repository, false)
	if err != nil || changed {
		t.Fatalf("fresh check = changed %t, err %v", changed, err)
	}
}

func TestGalleryRejectsAmbiguousMarkersAndSymlink(t *testing.T) {
	repository := t.TempDir()
	path := filepath.Join(repository, galleryPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(generatedStart+"\n"+generatedStart+"\n"+generatedEnd), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := updateGallery(repository, false); err == nil {
		t.Fatal("duplicate generated markers were accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(repository, "target.md")
	if err := os.WriteFile(target, []byte(generatedStart+"\n"+generatedEnd), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := updateGallery(repository, false); err == nil {
		t.Fatal("symlink gallery was accepted")
	}
}

func TestMenuMapIsDeterministicAndListsKnownEntries(t *testing.T) {
	first := renderMenuMap()
	if first != renderMenuMap() {
		t.Fatal("identical menu definitions produced different bytes")
	}
	for _, expected := range []string{
		"## Administrator menu",
		"## Teacher dashboard",
		"Maintenance → Update Nixorium (u): ",
		"Computers → Classroom view (v): ",
		"Maintenance → Change settings → Network (n): ",
		"Maintenance → Change settings → Accounts (a): ",
		"Maintenance → Change settings → Accounts → Change account passwords (p): ",
		"Maintenance → Change settings → Student workspace (w): ",
	} {
		if !strings.Contains(first, expected) {
			t.Fatalf("generated menu map omits %q", expected)
		}
	}
	teacher := first[strings.Index(first, "## Teacher dashboard"):]
	if !strings.Contains(teacher, "Computers → Classroom view (v): ") || strings.Contains(teacher, "Maintenance") {
		t.Fatal("teacher section must list only classroom Computers entries")
	}
}

func TestMenuMapCheckFailsWhenFileDiffers(t *testing.T) {
	repository := t.TempDir()
	path := filepath.Join(repository, menuMapPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	changed, err := updateMenuMap(repository, false)
	if err != nil || !changed {
		t.Fatalf("missing file check = changed %t, err %v", changed, err)
	}
	if err := os.WriteFile(path, []byte("stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err = updateMenuMap(repository, false)
	if err != nil || !changed {
		t.Fatalf("stale check = changed %t, err %v", changed, err)
	}
	if data, _ := os.ReadFile(path); string(data) != "stale\n" {
		t.Fatal("check mode modified the menu map")
	}
	if changed, err = updateMenuMap(repository, true); err != nil || !changed {
		t.Fatalf("write = changed %t, err %v", changed, err)
	}
	if changed, err = updateMenuMap(repository, false); err != nil || changed {
		t.Fatalf("check after write = changed %t, err %v", changed, err)
	}
}

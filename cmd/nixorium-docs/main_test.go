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
		"[F3] Search packages",
		"Profile review",
		"5 add · 0 keep scope · 1 excluded",
		"Opened from a saved software change",
		"Deployment completed and verified",
		"Type START to continue",
		"Active user sessions will be shut down",
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

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

func TestCollectArguments(t *testing.T) {
	for _, args := range [][]string{
		{"collect", "plan", "--on", "@lab"},
		{"collect", "plan", "--on", "pc01,pc02", "--into", "/home/teacher/Homework", "--json"},
		{"collect", "apply", "--on", "@lab", "--expect", "sha256:test"},
		{"collect", "apply", "--on", "pc01", "--into", "Homework", "--expect", "sha256:test", "--yes"},
	} {
		options, err := parseArguments(args)
		if err != nil || options.command != "collect" {
			t.Fatal(args, options, err)
		}
	}
	options, _ := parseArguments([]string{"collect", "plan", "--on", "pc01", "--into", "Homework"})
	if options.subcommand != "plan" || options.on != "pc01" || options.collectInto != "Homework" {
		t.Fatalf("options = %+v", options)
	}
	for _, args := range [][]string{
		{"collect"}, {"collect", "plan"},
		{"collect", "plan", "apply", "--on", "@lab"},
		{"collect", "apply", "--on", "@lab"},
		{"collect", "plan", "--on", "@lab", "--yes"},
		{"collect", "plan", "--on", "@lab", "--expect", "sha256:test"},
		{"collect", "plan", "--on", "@lab", "--into"},
		{"collect", "plan", "--on", "@lab", "--file", "x"},
		{"collect", "plan", "--on", "@lab", "--repo", "/srv/lab"},
		{"collect", "plan", "--on", "@lab", "--action", "lock"},
		{"collect", "status"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
	if options, err := parseArguments([]string{"collect", "--help"}); err != nil || !options.help {
		t.Fatalf("help = %+v, %v", options, err)
	}
}

// collectService answers collect plan and apply like the classroom
// service, then the save of the collected files.
func collectService(items []domain.ShareFile, data map[int]string) *fakeSaveService {
	return &fakeSaveService{kind: domain.DesktopSaveCollected, items: items, data: data, answer: func(request domain.ClassroomRequest) (domain.ClassroomResponse, error) {
		response := domain.ClassroomResponse{State: "completed"}
		switch request.Operation {
		case domain.ClassroomCollectPlanOperation:
			response.CollectPlan = &domain.CollectPlan{State: "ready", Requested: request.Requested, Destination: request.CollectFolder, ReviewToken: "sha256:" + request.CollectFolder, Message: "Ready."}
		case domain.ClassroomCollectApplyOperation:
			if request.CollectPlan == nil || request.CollectPlan.ReviewToken != "sha256:"+request.CollectPlan.Destination {
				return domain.ClassroomResponse{State: "failed", Message: "wrong plan"}, errors.New("wrong plan")
			}
			response.CollectReport = &domain.CollectReport{State: "completed", Save: "s1", Folder: "2026-10-15 10.30", Message: "Collected from 1 of 1 selected computers.", Targets: []domain.CollectOutcome{{Name: "pc01", State: domain.CollectCollected}}}
		default:
			return domain.ClassroomResponse{State: "failed", Message: "unexpected"}, errors.New("unexpected")
		}
		return response, nil
	}}
}

func TestCollectApplySavesIntoTheFolderAsTheUser(t *testing.T) {
	service := collectService(collectedItems, map[int]string{2: "essay", 4: "abc"})
	replaceSaveRequest(t, service)
	folder := t.TempDir()
	if err := os.Mkdir(filepath.Join(folder, "2026-10-15 10.30"), 0o755); err != nil {
		t.Fatal(err)
	}
	options, err := parseArguments([]string{"collect", "apply", "--on", "pc01", "--into", folder, "--expect", "sha256:" + folder, "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runCollectCommand(context.Background(), options, &stdout, &stderr); code != 0 {
		t.Fatalf("code %d: %s %s", code, stdout.String(), stderr.String())
	}
	// An existing folder of the same minute is kept; the new one is "(2)".
	saved := filepath.Join(folder, "2026-10-15 10.30 (2)")
	if !strings.Contains(stdout.String(), "Saved in "+saved+".") || len(service.done) != 1 || service.done[0] != "2026-10-15 10.30 (2)" {
		t.Fatalf("stdout = %s, done = %v", stdout.String(), service.done)
	}
	if content, err := os.ReadFile(filepath.Join(saved, "pc01", "essay.odt")); err != nil || string(content) != "essay" {
		t.Fatalf("essay = %q, %v", content, err)
	}
	if info, err := os.Stat(filepath.Join(saved, "pc01", "notes.txt")); err != nil || info.Mode() != 0o644 {
		t.Fatalf("notes = %v, %v", info, err)
	}
	// A changed folder needs a fresh review.
	options.expect = "sha256:/elsewhere"
	if code := runCollectCommand(context.Background(), options, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "create a fresh plan") {
		t.Fatalf("stale token accepted: %s", stderr.String())
	}
}

func TestCollectWithoutIntoUsesTheHomeFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	service := collectService(collectedItems, map[int]string{2: "essay", 4: "abc"})
	replaceSaveRequest(t, service)
	destination := filepath.Join(home, "Nixorium", "Collected")
	options, err := parseArguments([]string{"collect", "apply", "--on", "pc01", "--expect", "sha256:" + destination, "--yes"})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runCollectCommand(context.Background(), options, &stdout, &stderr); code != 0 {
		t.Fatalf("code %d: %s %s", code, stdout.String(), stderr.String())
	}
	if content, err := os.ReadFile(filepath.Join(destination, "2026-10-15 10.30", "pc01", "notes.txt")); err != nil || string(content) != "abc" {
		t.Fatalf("notes = %q, %v", content, err)
	}
	if info, err := os.Stat(filepath.Join(home, "Nixorium")); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("Nixorium folder = %v, %v", info, err)
	}
	// An explicit folder must exist.
	plan := planCollect(context.Background(), "pc01", filepath.Join(home, "missing"))
	if !plan.HasErrors() || !strings.Contains(plan.Message, "not an existing folder") {
		t.Fatalf("missing folder = %+v", plan)
	}
}

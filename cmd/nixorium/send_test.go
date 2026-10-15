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

func TestSendArguments(t *testing.T) {
	for _, args := range [][]string{
		{"send", "plan", "--file", "/home/teacher/lesson", "--on", "@lab"},
		{"send", "apply", "--file", "notes.txt", "--on", "pc01", "--expect", "sha256:test", "--yes"},
	} {
		if _, err := parseArguments(args); err != nil {
			t.Fatal(args, err)
		}
	}
	for _, args := range [][]string{
		{"send"}, {"send", "plan"},
		{"send", "plan", "--on", "@lab"},
		{"send", "apply", "--file", "notes.txt", "--on", "pc01", "--yes"},
		{"send", "plan", "--file", "notes.txt", "--on", "pc01", "--action", "lock"},
		{"send", "plan", "--file", "notes.txt", "--on", "pc01", "--yes"},
	} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
}

func TestClassroomViewArguments(t *testing.T) {
	if options, err := parseArguments([]string{"classroom-view"}); err != nil || options.command != "classroom-view" {
		t.Fatalf("classroom-view = %+v, %v", options, err)
	}
	for _, args := range [][]string{{"classroom-view", "plan"}, {"classroom-view", "--yes"}, {"classroom-view", "--on", "pc01"}} {
		if _, err := parseArguments(args); err == nil {
			t.Fatal("unsafe arguments accepted", args)
		}
	}
}

func TestDeployQueueArguments(t *testing.T) {
	for args, want := range map[string]string{
		"deploy queue status":           "queue-status",
		"deploy queue run --json":       "queue-run",
		"deploy queue cancel --on pc01": "queue-cancel",
		"deploy queue cancel --on @all": "queue-cancel",
	} {
		options, err := parseArguments(strings.Fields(args))
		if err != nil || options.subcommand != want {
			t.Fatalf("%s = %+v, %v", args, options.subcommand, err)
		}
	}
	options, err := parseArguments(strings.Fields("deploy apply --on @lab --expect " + strings.Repeat("a", 40) + " --queue-unreachable"))
	if err != nil || !options.queueUnreachable {
		t.Fatalf("queue flag = %+v, %v", options, err)
	}
	for _, args := range []string{
		"deploy queue", "deploy queue cancel", "deploy queue status --on pc01",
		"deploy plan --on @lab --queue-unreachable", "shutdown queue status", "deploy queue reboot",
	} {
		if _, err := parseArguments(strings.Fields(args)); err == nil {
			t.Fatal("accepted", args)
		}
	}
}

// noDeployment leaves no deployment repository to find, as for the teacher.
func noDeployment(t *testing.T) {
	t.Helper()
	empty := t.TempDir()
	t.Chdir(empty)
	t.Setenv("HOME", empty)
	t.Setenv("NIXORIUM_REPO", "")
	if _, err := resolveRepository(""); err == nil {
		t.Fatal("a deployment repository was found")
	}
}

func TestSendWorksWithoutTheDeploymentRepository(t *testing.T) {
	noDeployment(t)
	file := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	received := ""
	service := &fakeSaveService{answer: func(request domain.ClassroomRequest) (domain.ClassroomResponse, error) {
		response := domain.ClassroomResponse{State: "completed"}
		switch request.Operation {
		case domain.ClassroomShareBeginOperation:
			if len(request.ShareFiles) != 1 || request.ShareFiles[0].Path != "notes.txt" {
				return domain.ClassroomResponse{State: "failed", Message: "unexpected files"}, errors.New("unexpected files")
			}
			response.ShareTransfer = "t1"
		case domain.ClassroomShareChunkOperation:
			received += string(request.ShareData)
		case domain.ClassroomSharePlanOperation:
			response.SharePlan = &domain.SharePlan{State: "ready", Requested: request.Requested, Transfer: request.ShareTransfer, ReviewToken: "sha256:share", Message: "1 item (5 bytes) go to the desktop of 1 of 1 computers.", Targets: []domain.ShareTarget{{HostMeta: domain.HostMeta{Name: "pc01"}, Eligible: true}}}
		case domain.ClassroomShareApplyOperation:
			response.ShareReport = &domain.ShareReport{State: "completed", Message: "Delivered to 1 of 1 selected computers.", Targets: []domain.ShareOutcome{{Name: "pc01", State: "delivered"}}}
		default:
			return domain.ClassroomResponse{State: "failed", Message: "unexpected"}, errors.New("unexpected")
		}
		return response, nil
	}}
	replaceSaveRequest(t, service)
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"send", "plan", "--file", file, "--on", "pc01"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "Review token: sha256:share") || strings.Contains(stderr.String(), "repository") {
		t.Fatalf("plan = %d %s %s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if code := run(context.Background(), []string{"send", "apply", "--file", file, "--on", "pc01", "--expect", "sha256:share", "--yes"}, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "Delivered to 1 of 1") {
		t.Fatalf("apply = %d %s %s", code, stdout.String(), stderr.String())
	}
	if received != "hellohello" {
		t.Fatalf("uploaded = %q", received)
	}
}

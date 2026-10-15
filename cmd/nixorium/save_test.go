package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/giovantenne/nixorium/internal/domain"
)

// fakeSaveService answers like the classroom service for one save; answer
// handles the other operations.
type fakeSaveService struct {
	kind     string
	items    []domain.ShareFile
	data     map[int]string
	done     []string
	message  string
	requests []domain.ClassroomOperation
	answer   func(domain.ClassroomRequest) (domain.ClassroomResponse, error)
}

func (service *fakeSaveService) request(_ context.Context, operation domain.ClassroomOperation, configure func(*domain.ClassroomRequest)) (domain.ClassroomResponse, error) {
	request := domain.ClassroomRequest{Operation: operation}
	if configure != nil {
		configure(&request)
	}
	service.requests = append(service.requests, operation)
	response := domain.ClassroomResponse{State: "completed"}
	switch operation {
	case domain.ClassroomSaveItemsOperation:
		if request.DesktopJob != "s1" {
			return domain.ClassroomResponse{State: "failed", Message: "unknown"}, errors.New("unknown")
		}
		// Two items per page, as a long list would come.
		end := min(len(service.items), request.ShareIndex+2)
		response.DesktopKind, response.SaveItems, response.SaveTotal = service.kind, service.items[request.ShareIndex:end], len(service.items)
	case domain.ClassroomSaveReadOperation:
		data := service.data[request.ShareIndex][request.ShareOffset:]
		response.SaveData = []byte(data[:min(len(data), 2)])
	case domain.ClassroomSaveDoneOperation:
		service.done, service.message = request.SaveNames, request.DesktopError
	default:
		if service.answer == nil {
			return domain.ClassroomResponse{State: "failed", Message: "unexpected"}, errors.New("unexpected")
		}
		return service.answer(request)
	}
	return response, nil
}

func replaceSaveRequest(t *testing.T, service *fakeSaveService) {
	t.Helper()
	previous := classroomCall
	classroomCall = service.request
	t.Cleanup(func() { classroomCall = previous })
}

var collectedItems = []domain.ShareFile{
	{Path: "2026-10-15 10.30", Dir: true},
	{Path: "2026-10-15 10.30/pc01", Dir: true},
	{Path: "2026-10-15 10.30/pc01/essay.odt", Size: 5},
	{Path: "2026-10-15 10.30/pc01/empty.txt"},
	{Path: "2026-10-15 10.30/pc01/notes.txt", Size: 3},
}

func TestSavesRefuseUnsafeItemsAndOtherKinds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for name, items := range map[string][]domain.ShareFile{
		"traversal": {{Path: "../../.bashrc", Size: 2}},
		"absolute":  {{Path: "/etc/passwd", Size: 2}},
		"backslash": {{Path: `..\..\x`, Size: 2}},
		"control":   {{Path: "pc01\n.jpg", Size: 2}},
	} {
		service := &fakeSaveService{kind: domain.DesktopSaveScreenshots, items: items, data: map[int]string{0: "xx"}}
		replaceSaveRequest(t, service)
		saveDesktopJob(context.Background(), "s1", domain.DesktopSaveScreenshots)
		if len(service.done) != 0 || service.message == "" {
			t.Fatalf("%s was saved: %v", name, service.done)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
		t.Fatal("a file was written outside the folder")
	}
	// The page asked for screenshots; other files are refused.
	service := &fakeSaveService{kind: domain.DesktopSaveCollected, items: collectedItems, data: map[int]string{2: "essay", 4: "abc"}}
	replaceSaveRequest(t, service)
	saveDesktopJob(context.Background(), "s1", domain.DesktopSaveScreenshots)
	if service.message == "" {
		t.Fatal("a collection was saved as screenshots")
	}
	// Screenshots go to ~/Nixorium/Screenshots, never replacing a picture.
	pictures := []domain.ShareFile{{Path: "pc03-2026-10-15-10.30.12.jpg", Size: 4}}
	for attempt := 1; attempt <= 2; attempt++ {
		service = &fakeSaveService{kind: domain.DesktopSaveScreenshots, items: pictures, data: map[int]string{0: "\xff\xd8\xff\xd9"}}
		replaceSaveRequest(t, service)
		saveDesktopJob(context.Background(), "s1", domain.DesktopSaveScreenshots)
		want := "Nixorium/Screenshots/pc03-2026-10-15-10.30.12.jpg"
		if attempt == 2 {
			want = "Nixorium/Screenshots/pc03-2026-10-15-10.30.12 (2).jpg"
		}
		if fmt.Sprint(service.done) != "["+want+"]" || service.message != "" {
			t.Fatalf("attempt %d = %v %q", attempt, service.done, service.message)
		}
		if info, err := os.Stat(filepath.Join(home, filepath.FromSlash(want))); err != nil || info.Mode() != 0o644 {
			t.Fatalf("picture = %v, %v", info, err)
		}
	}
}

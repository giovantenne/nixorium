package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Files the classroom service holds for the user who runs Nixorium
// (screenshots, collected desktops) are copied into a folder of that user,
// as that user, through the classroom socket.

// classroomCall reaches the classroom service for sending, collecting and
// saving files; tests replace it.
var classroomCall = classroomRequest

// saveFolders are the folders of the user's home that the classroom view
// page's saves go to.
var saveFolders = map[string]string{
	domain.DesktopSaveScreenshots: "Nixorium/Screenshots",
	domain.DesktopSaveCollected:   "Nixorium/Collected",
}

// saveDesktopJob answers a save job of the classroom view page: the files
// go to the job's folder of the home, and the service learns where.
func saveDesktopJob(ctx context.Context, id, kind string) {
	names, message := []string{}, ""
	placed, err := saveIntoHome(ctx, id, kind)
	if err != nil {
		message = strings.ToUpper(err.Error()[:1]) + err.Error()[1:] + "."
	}
	for _, name := range placed {
		names = append(names, saveFolders[kind]+"/"+name)
	}
	_, _ = classroomCall(ctx, domain.ClassroomSaveDoneOperation, func(request *domain.ClassroomRequest) {
		request.DesktopJob, request.SaveNames, request.DesktopError = id, names, message
	})
}

func saveIntoHome(ctx context.Context, id, kind string) ([]string, error) {
	folder, known := saveFolders[kind]
	if !known {
		return nil, errors.New("the page asked for an unknown save")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	destination := filepath.Join(home, filepath.FromSlash(folder))
	if err := classroomview.EnsureFolder(destination); err != nil {
		return nil, err
	}
	received, placed, err := receiveSave(ctx, id, destination)
	if err == nil && received != kind {
		err = errors.New("the service sent other files than the page asked for")
	}
	return placed, err
}

// receiveSave copies a save of the classroom service into destination, an
// existing folder, and returns its kind and the names its top-level items
// got there (an existing name is kept and the new item becomes
// "name (2)"). Every name is checked again here.
func receiveSave(ctx context.Context, id, destination string) (string, []string, error) {
	kind, items, err := saveItems(ctx, id)
	if err != nil {
		return "", nil, err
	}
	entries := make([]classroomview.FileEntry, len(items))
	for index, item := range items {
		entries[index] = classroomview.FileEntry{Path: item.Path, Size: item.Size, Dir: item.Dir}
	}
	writer, err := classroomview.NewTreeWriter(destination, entries)
	if err != nil {
		return kind, nil, err
	}
	for index, entry := range entries {
		if entry.Dir || entry.Size == 0 {
			continue
		}
		for offset := int64(0); offset < entry.Size; {
			response, requestErr := classroomCall(ctx, domain.ClassroomSaveReadOperation, func(request *domain.ClassroomRequest) {
				request.DesktopJob, request.ShareIndex, request.ShareOffset = id, index, offset
			})
			if requestErr != nil || len(response.SaveData) == 0 {
				writer.Abort()
				return kind, nil, errors.New(classroomMessage(response, requestErr, "the files could not be read from the classroom service"))
			}
			if err := writer.Write(index, response.SaveData); err != nil {
				writer.Abort()
				return kind, nil, err
			}
			offset += int64(len(response.SaveData))
		}
	}
	placed, err := writer.Finish()
	return kind, placed, err
}

// saveItems reads the complete list of a save, one page at a time.
func saveItems(ctx context.Context, id string) (string, []domain.ShareFile, error) {
	kind, items := "", []domain.ShareFile{}
	for {
		response, err := classroomCall(ctx, domain.ClassroomSaveItemsOperation, func(request *domain.ClassroomRequest) {
			request.DesktopJob, request.ShareIndex = id, len(items)
		})
		if err != nil {
			return "", nil, errors.New(classroomMessage(response, err, "the files to save are not available"))
		}
		if kind == "" {
			kind = response.DesktopKind
		}
		if response.DesktopKind != kind || response.SaveTotal > len(items)+len(response.SaveItems) && len(response.SaveItems) == 0 {
			return "", nil, errors.New("the list of files to save is inconsistent")
		}
		items = append(items, response.SaveItems...)
		if len(items) >= response.SaveTotal {
			if len(items) != response.SaveTotal {
				return "", nil, errors.New("the list of files to save is inconsistent")
			}
			return kind, items, nil
		}
	}
}

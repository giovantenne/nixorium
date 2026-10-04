package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
)

// Send desktop from the terminal: the files and folders on the desktop of
// whoever runs Nixorium (teacher or administrator) are prepared through the
// classroom service, which alone reaches the students' computers.

func shareFailure(message string) domain.SharePlan {
	return domain.SharePlan{SchemaVersion: domain.SchemaVersion, Operation: "share-plan", State: "blocked", Message: message, Files: []domain.ShareFile{}, Targets: []domain.ShareTarget{}, Issues: []domain.ValidationIssue{{Field: "share", Message: message}}}
}

func classroomMessage(response domain.ClassroomResponse, err error, fallback string) string {
	switch {
	case response.Message != "":
		return response.Message
	case err != nil:
		return err.Error()
	}
	return fallback
}

// prepareDesktop uploads the caller's desktop and returns the transfer and
// the folder it came from.
func prepareDesktop(ctx context.Context) (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	folder, err := classroomview.DesktopDirectory(home)
	if err != nil {
		return "", "", err
	}
	entries, err := classroomview.CollectFolder(folder)
	if errors.Is(err, os.ErrNotExist) || (err == nil && len(entries) == 0) {
		return "", folder, fmt.Errorf("your desktop folder %s is empty", folder)
	}
	if err == nil {
		err = classroomview.ValidateFileEntries(entries)
	}
	if err != nil {
		return "", folder, fmt.Errorf("your desktop cannot be sent: %w", err)
	}
	files := make([]domain.ShareFile, len(entries))
	for index, entry := range entries {
		files[index] = domain.ShareFile{Path: entry.Path, Size: entry.Size, Dir: entry.Dir}
	}
	response, err := classroomRequest(ctx, domain.ClassroomShareBeginOperation, func(request *domain.ClassroomRequest) {
		request.ShareFiles = files
	})
	if err != nil || response.ShareTransfer == "" {
		return "", folder, errors.New(classroomMessage(response, err, "The files could not be prepared."))
	}
	transfer := response.ShareTransfer
	buffer := make([]byte, domain.ClassroomShareChunkBytes)
	for index, entry := range entries {
		if entry.Dir || entry.Size == 0 {
			continue
		}
		if err := uploadFile(ctx, transfer, index, filepath.Join(folder, filepath.FromSlash(entry.Path)), entry.Size, buffer); err != nil {
			return "", folder, err
		}
	}
	return transfer, folder, nil
}

func uploadFile(ctx context.Context, transfer string, index int, name string, size int64, buffer []byte) error {
	file, err := os.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	for offset := int64(0); offset < size; {
		count, err := io.ReadFull(file, buffer[:min(int64(len(buffer)), size-offset)])
		if count == 0 {
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
			return fmt.Errorf("%s changed while it was read: %w", name, err)
		}
		data := buffer[:count]
		response, requestErr := classroomRequest(ctx, domain.ClassroomShareChunkOperation, func(request *domain.ClassroomRequest) {
			request.ShareTransfer, request.ShareIndex, request.ShareOffset, request.ShareData = transfer, index, offset, data
		})
		if requestErr != nil || response.State == "failed" {
			return errors.New(classroomMessage(response, requestErr, "The files could not be prepared."))
		}
		offset += int64(count)
	}
	return nil
}

func planDesktopShare(ctx context.Context, requested string) domain.SharePlan {
	transfer, folder, err := prepareDesktop(ctx)
	if err != nil {
		return shareFailure(err.Error() + ".")
	}
	response, err := classroomRequest(ctx, domain.ClassroomSharePlanOperation, func(request *domain.ClassroomRequest) {
		request.Requested, request.ShareTransfer = requested, transfer
	})
	if err != nil || response.SharePlan == nil {
		return shareFailure(classroomMessage(response, err, "Sending files is unavailable."))
	}
	plan := *response.SharePlan
	plan.Message = "From " + folder + ": " + plan.Message
	return plan
}

func applyDesktopShare(ctx context.Context, plan domain.SharePlan) domain.ShareReport {
	response, err := classroomRequest(ctx, domain.ClassroomShareApplyOperation, func(request *domain.ClassroomRequest) {
		request.SharePlan = &plan
	})
	if err != nil || response.ShareReport == nil {
		return domain.ShareReport{SchemaVersion: domain.SchemaVersion, Operation: "share-apply", State: "blocked", Targets: []domain.ShareOutcome{}, Message: classroomMessage(response, err, "The reviewed sending could not be applied.")}
	}
	return *response.ShareReport
}

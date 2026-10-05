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

// Send files: a file or folder chosen by whoever runs Nixorium (teacher or
// administrator) is prepared through the classroom service, which alone
// reaches the students' computers, and lands on their desktops.

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

// prepareShare uploads one file, or one folder with its contents, and
// returns the transfer.
func prepareShare(ctx context.Context, item string) (string, error) {
	item, err := filepath.Abs(item)
	if err != nil {
		return "", err
	}
	parent, entries, err := classroomview.CollectItem(item)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%s does not exist", item)
	}
	if err == nil {
		err = classroomview.ValidateFileEntries(entries)
	}
	if err != nil {
		return "", fmt.Errorf("%s cannot be sent: %w", item, err)
	}
	files := make([]domain.ShareFile, len(entries))
	for index, entry := range entries {
		files[index] = domain.ShareFile{Path: entry.Path, Size: entry.Size, Dir: entry.Dir}
	}
	response, err := classroomRequest(ctx, domain.ClassroomShareBeginOperation, func(request *domain.ClassroomRequest) {
		request.ShareFiles = files
	})
	if err != nil || response.ShareTransfer == "" {
		return "", errors.New(classroomMessage(response, err, "The files could not be prepared."))
	}
	transfer := response.ShareTransfer
	buffer := make([]byte, domain.ClassroomShareChunkBytes)
	for index, entry := range entries {
		if entry.Dir || entry.Size == 0 {
			continue
		}
		if err := uploadFile(ctx, transfer, index, filepath.Join(parent, filepath.FromSlash(entry.Path)), entry.Size, buffer); err != nil {
			return "", err
		}
	}
	return transfer, nil
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

func planShare(ctx context.Context, requested, item string) domain.SharePlan {
	transfer, err := prepareShare(ctx, item)
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
	plan.Message = "Sending " + filepath.Base(filepath.Clean(item)) + ": " + plan.Message
	return plan
}

func applyShare(ctx context.Context, plan domain.SharePlan) domain.ShareReport {
	response, err := classroomRequest(ctx, domain.ClassroomShareApplyOperation, func(request *domain.ClassroomRequest) {
		request.SharePlan = &plan
	})
	if err != nil || response.ShareReport == nil {
		return domain.ShareReport{SchemaVersion: domain.SchemaVersion, Operation: "share-apply", State: "blocked", Targets: []domain.ShareOutcome{}, Message: classroomMessage(response, err, "The reviewed sending could not be applied.")}
	}
	return *response.ShareReport
}

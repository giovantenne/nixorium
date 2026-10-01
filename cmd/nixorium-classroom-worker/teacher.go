package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/giovantenne/nixorium/internal/adapters"
	"github.com/giovantenne/nixorium/internal/domain"
)

// teacherMessage rewrites administrative blockers in words a teacher can act
// on: try again later, or ask the administrator and quote a short code.
// Other messages are already about the selected computers and stay unchanged.
func teacherMessage(message string) string {
	switch {
	case strings.Contains(message, adapters.OperationBusyMessage):
		holder := ""
		if _, detail, found := strings.Cut(message, adapters.OperationBusyMessage+": "); found {
			holder = " (" + strings.TrimSuffix(detail, ".") + ")"
		}
		return "The administrator is working on the computers" + holder + ". Try again in a few minutes. Code: OP-BUSY."
	case strings.Contains(message, "unfinished client deployment"):
		return "An interrupted computer update must be finished by the administrator first. Ask the administrator. Code: DEPLOY-PENDING."
	case strings.Contains(message, "USB installation"):
		return "A computer installation is in progress. Ask the administrator. Code: USB-RESERVED."
	case strings.Contains(message, "network installation"), strings.Contains(message, "installation mode"):
		return "Network installation is running in the laboratory. Try again later or ask the administrator. Code: PXE-ACTIVE."
	}
	return message
}

// Ready plans carry no issues; an empty list is returned unchanged so review
// tokens that cover the issue list stay valid.
func teacherIssues(issues []domain.ValidationIssue) []domain.ValidationIssue {
	if len(issues) == 0 {
		return issues
	}
	result := make([]domain.ValidationIssue, len(issues))
	for index, issue := range issues {
		issue.Message = teacherMessage(issue.Message)
		result[index] = issue
	}
	return result
}

// classroomFailure keeps the technical cause in the service journal for the
// administrator and gives the teacher a short, actionable message.
func classroomFailure(err error) domain.ClassroomResponse {
	message := teacherMessage(err.Error())
	if message == err.Error() {
		fmt.Fprintln(os.Stderr, "nixorium classroom worker:", err)
		message = "The classroom service could not read the laboratory configuration. Ask the administrator. Code: CLASSROOM-LOAD."
	}
	return domain.ClassroomResponse{State: "failed", Message: message}
}

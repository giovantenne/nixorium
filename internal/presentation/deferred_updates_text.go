package presentation

import (
	"fmt"
	"io"

	"github.com/giovantenne/nixorium/internal/domain"
)

func DeferredUpdateStatusText(w io.Writer, s domain.DeferredUpdateStatus) {
	fmt.Fprintln(w, s.Message)
	for _, entry := range s.Updates {
		state := "waits for the computer"
		if entry.Stale {
			state = "configuration changed since; review the update again"
		} else if entry.LastError != "" {
			state = fmt.Sprintf("last attempt %s failed: %s", entry.LastAttempt.Local().Format("2006-01-02 15:04"), entry.LastError)
		}
		fmt.Fprintf(w, "  %-10s %-15s since %s · %s\n", entry.Host, entry.IP, entry.QueuedAt.Local().Format("2006-01-02 15:04"), state)
	}
	for _, issue := range s.Issues {
		fmt.Fprintf(w, "  ERROR %s: %s\n", issue.Field, issue.Message)
	}
}

func DeferredUpdateRunText(w io.Writer, r domain.DeferredUpdateRunReport) {
	fmt.Fprintln(w, r.Message)
	for _, result := range r.Results {
		fmt.Fprintf(w, "  %-10s %-8s %s\n", result.Host, result.State, result.Detail)
	}
	for _, issue := range r.Issues {
		fmt.Fprintf(w, "  ERROR %s: %s\n", issue.Field, issue.Message)
	}
}

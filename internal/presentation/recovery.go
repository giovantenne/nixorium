package presentation

import (
	"fmt"
	"io"

	"github.com/giovantenne/nixorium/internal/domain"
)

func RecoveryText(writer io.Writer, report domain.RecoveryReport) {
	if len(report.Conditions) == 0 {
		fmt.Fprintln(writer, "Nothing blocks Nixorium operations.")
		return
	}
	fmt.Fprintf(writer, "%d item(s) need attention:\n", len(report.Conditions))
	for _, condition := range report.Conditions {
		fmt.Fprintf(writer, "\n- %s\n", condition.Title)
		if condition.Detail != "" {
			fmt.Fprintf(writer, "  %s\n", condition.Detail)
		}
		if condition.Blocks != "" {
			fmt.Fprintf(writer, "  Blocks: %s\n", condition.Blocks)
		}
		fmt.Fprintf(writer, "  %s\n", condition.Next.Line())
	}
}

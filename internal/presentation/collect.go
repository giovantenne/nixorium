package presentation

import (
	"fmt"
	"io"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func CollectPlanText(w io.Writer, p domain.CollectPlan) {
	fmt.Fprintf(w, "Collect from students' desktops (%s)\n%s\n", p.State, p.Message)
	for _, t := range p.Targets {
		outcome := t.Detail
		if !t.Eligible {
			outcome += " Not collected."
		}
		fmt.Fprintf(w, "  %s  %s  %s\n", t.Name, t.IP, strings.TrimSpace(outcome))
	}
	if p.ReviewToken != "" {
		fmt.Fprintf(w, "Review token: %s\n", p.ReviewToken)
	}
}

func CollectReportText(w io.Writer, r domain.CollectReport) {
	fmt.Fprintln(w, r.Message)
	for _, t := range r.Targets {
		fmt.Fprintf(w, "  %s  %s  %s\n", t.Name, t.State, t.Detail)
	}
}

func ConfirmCollect(input io.Reader, output io.Writer, p domain.CollectPlan) (bool, error) {
	return confirmYesNo(input, output, func(w io.Writer) {
		CollectPlanText(w, p)
		fmt.Fprint(w, "Collect these desktops? [y/N] ")
	})
}

package presentation

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

func CollectPlanText(w io.Writer, p domain.CollectPlan) {
	fmt.Fprintf(w, "Collect from desktop (%s)\n%s\n", p.State, p.Message)
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
	CollectPlanText(output, p)
	fmt.Fprint(output, "Collect these desktops? [y/N] ")
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}

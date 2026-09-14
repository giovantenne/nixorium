package presentation

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/giovantenne/nixorium/internal/domain"
)

func IsInteractive(input *os.File) bool {
	return input != nil && term.IsTerminal(input.Fd())
}

func ConfirmControllerApply(input io.Reader, output io.Writer, controller string) (bool, error) {
	fmt.Fprintf(output, "Controller apply review\n")
	fmt.Fprintf(output, "Machine: %s (this controller only)\n", controller)
	fmt.Fprintln(output, "Action: validate, build, and activate the reviewed Git configuration")
	fmt.Fprintln(output, "Impact: services and networking may restart; this terminal connection may be interrupted")
	fmt.Fprintln(output, "Reboot: not normally required")
	fmt.Fprint(output, "Type APPLY to continue: ")
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == "APPLY", nil
}

func ConfirmPXEStart(input io.Reader, output io.Writer, report domain.PXELifecycleReport) (bool, error) {
	fmt.Fprintln(output, "PXE installation mode review")
	fmt.Fprintf(output, "Interface: %s\n", report.Interface)
	fmt.Fprintf(output, "Temporary change: remove %s while installation mode is active\n", report.StaticCIDR)
	fmt.Fprintf(output, "Service address: %s (institutional DHCP remains authoritative)\n", report.DHCPAddress)
	fmt.Fprintln(output, "Services: ProxyDHCP, TFTP, HTTP, and the local binary cache")
	fmt.Fprintln(output, "Recovery: `nixorium pxe stop` restores normal addressing; reboot recovery is enabled")
	fmt.Fprint(output, "Type START PXE to continue: ")
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == "START PXE", nil
}

func ConfirmDeploymentApply(input io.Reader, output io.Writer, report domain.DeploymentPlanReport) (bool, error) {
	fmt.Fprintln(output, "Client deployment review")
	fmt.Fprintf(output, "Revision: %s\n", report.Revision)
	fmt.Fprintf(output, "Targets: %s (%d computer(s))\n", report.ColmenaSelector, len(report.Targets))
	fmt.Fprintln(output, "Action: build every selected configuration, then apply it with Colmena")
	fmt.Fprintln(output, "Impact: target services may restart; offline or failed targets will be reported")
	fmt.Fprintln(output, "Retry: safe; Nixorium revalidates the revision and rebuilds before every apply")
	fmt.Fprintf(output, "Type DEPLOY %s to continue: ", report.ColmenaSelector)
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == "DEPLOY "+report.ColmenaSelector, nil
}

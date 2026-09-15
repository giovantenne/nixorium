package presentation

import (
	"bufio"
	"errors"
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

func ConfirmControllerRebuild(input io.Reader, output io.Writer, report domain.ControllerRebuildPlanReport) (bool, error) {
	fmt.Fprintln(output, "Controller rebuild review")
	fmt.Fprintf(output, "Machine: %s (this controller only)\n", report.Controller)
	fmt.Fprintf(output, "Revision: %s\n", report.Revision)
	fmt.Fprintln(output, "Action: validate, build, activate, and verify the reviewed Git configuration")
	fmt.Fprintln(output, "Impact: services and networking may restart; this terminal connection may be interrupted")
	fmt.Fprintln(output, "Retry: safe after inspecting the systemd unit journal and creating a fresh plan")
	fmt.Fprintf(output, "Type %s to continue: ", report.Confirmation)
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == report.Confirmation, nil
}

func ConfirmServiceRestart(input io.Reader, output io.Writer, service domain.ManagedService) (bool, error) {
	fmt.Fprintln(output, "Service restart review")
	fmt.Fprintf(output, "Service: %s\n", service.Name)
	fmt.Fprintln(output, "Impact: the binary cache will be briefly unavailable; active PXE clients may retry downloads")
	fmt.Fprintln(output, "Safety: PXE networking and listeners are not controlled by this action")
	fmt.Fprint(output, "Type RESTART CACHE to continue: ")
	reader := bufio.NewReader(input)
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return strings.TrimSpace(value) == "RESTART CACHE", nil
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

func ConfirmGitCommit(input io.Reader, output io.Writer, report domain.GitCommitPlanReport) (bool, error) {
	fmt.Fprintln(output, "Local Git commit review")
	fmt.Fprintf(output, "HEAD: %s\n", report.Revision)
	fmt.Fprintf(output, "Paths: %s\n", strings.Join(report.Paths, ", "))
	fmt.Fprintf(output, "Message: %s\n", report.CommitMessage)
	fmt.Fprintln(output, "Action: create one local commit containing only the reviewed path versions")
	fmt.Fprintln(output, "Safety: unrelated index/worktree changes remain; no hook, signing action, remote, or push is used")
	fmt.Fprintf(output, "Type %s to continue: ", report.Confirmation)
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == report.Confirmation, nil
}

func ConfirmUpdate(input io.Reader, output io.Writer, report domain.UpdatePlanReport) (bool, error) {
	fmt.Fprintln(output, "Nixorium release update review")
	fmt.Fprintf(output, "Current: %s (%s)\n", report.CurrentRef, report.CurrentRev)
	fmt.Fprintf(output, "Target: %s (%s)\n", report.Target, report.TargetChannel)
	fmt.Fprintln(output, "Action: replace only flake.nix and flake.lock with the validated proposal")
	fmt.Fprintln(output, "Safety: no branch, commit, push, activation, PXE action, or deployment is performed")
	fmt.Fprintln(output, "Afterward: review and commit the two files separately before deploying")
	fmt.Fprintf(output, "Type %s to continue: ", report.Confirmation)
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == report.Confirmation, nil
}

func ConfirmSoftwareChange(input io.Reader, output io.Writer, report domain.SoftwareChangePlanReport) (bool, error) {
	fmt.Fprintln(output, "Software declaration review")
	fmt.Fprintf(output, "Package: %s\n", report.Request.Package)
	fmt.Fprintf(output, "Configuration scope: %s (%d client(s))\n", report.Request.Scope.Kind, len(report.AffectedClients))
	fmt.Fprintln(output, "Action: atomically update only lab-software.json")
	fmt.Fprintln(output, "Safety: no commit, build, controller activation, PXE action, or client deployment is performed")
	fmt.Fprintln(output, "Afterward: review and commit the managed file, then prepare or distribute the system separately")
	fmt.Fprintf(output, "Type %s to continue: ", report.Confirmation)
	value, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(value) == 0 {
		return false, err
	}
	return strings.TrimSpace(value) == report.Confirmation, nil
}

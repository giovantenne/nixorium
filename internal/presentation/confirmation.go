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

// continuePrompt is the question of every review whose confirmation is a
// yes/no answer: only an action that destroys something or cannot be undone
// asks for a typed word (ADR 0026, amended 2026-10-19).
const continuePrompt = "Continue?"

func IsInteractive(input *os.File) bool {
	return input != nil && term.IsTerminal(input.Fd())
}

func ConfirmControllerApply(input io.Reader, output io.Writer, controller string) (bool, error) {
	fmt.Fprintf(output, "Controller apply review\n")
	fmt.Fprintf(output, "Machine: %s (this controller only)\n", controller)
	fmt.Fprintln(output, "Action: validate, build, and activate the reviewed Git configuration")
	fmt.Fprintln(output, "Impact: services and networking may restart; this terminal connection may be interrupted")
	fmt.Fprintln(output, "Reboot: not normally required")
	return ConfirmYesNo(input, output, continuePrompt)
}

func ConfirmControllerRebuild(input io.Reader, output io.Writer, report domain.ControllerRebuildPlanReport) (bool, error) {
	fmt.Fprintln(output, "Apply to controller: review")
	fmt.Fprintf(output, "Machine: %s (this controller only)\n", report.Controller)
	fmt.Fprintf(output, "Revision: %s\n", report.Revision)
	fmt.Fprintln(output, "Action: validate, build, activate, and verify the reviewed Git configuration")
	fmt.Fprintln(output, "Impact: services and networking may restart; this terminal connection may be interrupted")
	fmt.Fprintln(output, "Retry: safe after inspecting the systemd unit journal and creating a fresh plan")
	return ConfirmYesNo(input, output, continuePrompt)
}

func ConfirmServiceRestart(input io.Reader, output io.Writer, service domain.ManagedService) (bool, error) {
	fmt.Fprintln(output, "Service restart review")
	fmt.Fprintf(output, "Service: %s\n", service.Name)
	fmt.Fprintln(output, "Impact: the binary cache will be briefly unavailable; active PXE clients may retry downloads")
	fmt.Fprintln(output, "Safety: PXE networking and listeners are not controlled by this action")
	return ConfirmYesNo(input, output, continuePrompt)
}

func ConfirmPXEStart(input io.Reader, output io.Writer, report domain.PXELifecycleReport) (bool, error) {
	fmt.Fprintln(output, "PXE installation mode review")
	fmt.Fprintf(output, "Interface: %s\n", report.Interface)
	fmt.Fprintf(output, "Temporary change: remove %s while installation mode is active\n", report.StaticCIDR)
	fmt.Fprintf(output, "Service address: %s (institutional DHCP remains authoritative)\n", report.DHCPAddress)
	fmt.Fprintln(output, "Services: ProxyDHCP, TFTP, HTTP, and the local binary cache")
	fmt.Fprintln(output, "Recovery: `nixorium pxe stop` restores normal addressing; reboot recovery is enabled")
	return ConfirmYesNo(input, output, continuePrompt)
}

func ConfirmDeploymentApply(input io.Reader, output io.Writer, report domain.DeploymentPlanReport) (bool, error) {
	fmt.Fprintln(output, "Update computers: review")
	fmt.Fprintf(output, "Revision: %s\n", report.Revision)
	fmt.Fprintf(output, "Targets: %s (%s)\n", report.ColmenaSelector, countNoun(len(report.Targets), "computer"))
	fmt.Fprintln(output, "Action: build every selected system, then copy and activate it on each computer")
	fmt.Fprintln(output, "Impact: target services may restart; offline or failed targets will be reported")
	fmt.Fprintln(output, "Retry: safe; Nixorium revalidates the revision and rebuilds before every apply")
	return ConfirmYesNo(input, output, continuePrompt)
}

func ConfirmGitCommit(input io.Reader, output io.Writer, report domain.GitCommitPlanReport) (bool, error) {
	fmt.Fprintln(output, "Local Git commit review")
	fmt.Fprintf(output, "HEAD: %s\n", report.Revision)
	fmt.Fprintf(output, "Paths: %s\n", strings.Join(report.Paths, ", "))
	fmt.Fprintf(output, "Message: %s\n", report.CommitMessage)
	fmt.Fprintln(output, "Action: create one local commit containing only the reviewed path versions")
	fmt.Fprintln(output, "Safety: unrelated index/worktree changes remain; no hook, signing action, remote, or push is used")
	return ConfirmYesNo(input, output, continuePrompt)
}

func ConfirmUpdate(input io.Reader, output io.Writer, report domain.UpdatePlanReport) (bool, error) {
	if report.Kind == "package-base" {
		fmt.Fprintln(output, "System and package update review (runtime compatibility unverified)")
	} else {
		fmt.Fprintln(output, "Nixorium release update review")
	}
	fmt.Fprintf(output, "Current: %s (%s)\n", report.CurrentRef, report.CurrentRev)
	fmt.Fprintf(output, "Target: %s (%s)\n", report.Target, report.TargetChannel)
	fmt.Fprintln(output, "Action: replace only flake.nix and flake.lock with the validated proposal")
	fmt.Fprintln(output, "Safety: no branch, commit, push, activation, PXE action, or deployment is performed")
	fmt.Fprintln(output, "Afterward: review and commit the two files separately before deploying")
	return ConfirmYesNo(input, output, continuePrompt)
}

func ConfirmSoftwareChange(input io.Reader, output io.Writer, report domain.SoftwareChangePlanReport) (bool, error) {
	fmt.Fprintln(output, "Software declaration review")
	fmt.Fprintf(output, "Package: %s\n", report.Request.Package)
	fmt.Fprintf(output, "Configuration scope: %s (%s)\n", report.Request.Scope.Kind, countNoun(len(report.AffectedClients), "client"))
	if report.AffectedController != "" {
		fmt.Fprintf(output, "Controller configuration: %s (not activated by saving)\n", report.AffectedController)
	}
	fmt.Fprintln(output, "Action: atomically update only lab-software.json")
	fmt.Fprintln(output, "Safety: no commit, build, controller activation, PXE action, or client deployment is performed")
	fmt.Fprintln(output, "Afterward: review and commit the managed file, then prepare or distribute the system separately")
	return ConfirmYesNo(input, output, continuePrompt)
}

func ConfirmSoftwarePreset(input io.Reader, output io.Writer, report domain.SoftwarePresetPlanReport) (bool, error) {
	fmt.Fprintln(output, "Software profile declaration review")
	fmt.Fprintf(output, "Profile: %s (%s)\n", report.Preset.Label, report.Request.Preset)
	fmt.Fprintf(output, "New declarations: %d with scope %s\n", len(report.Additions), softwareScopeText(report.Request.Scope))
	if len(report.Existing) > 0 {
		fmt.Fprintf(output, "Existing declarations: %d keep their current scopes\n", len(report.Existing))
	}
	fmt.Fprintln(output, "Action: atomically update only lab-software.json")
	fmt.Fprintln(output, "Safety: no commit, build, controller activation, PXE action, or client deployment is performed")
	fmt.Fprintln(output, "Afterward: review the complete current configuration before preparing or distributing systems")
	return ConfirmYesNo(input, output, continuePrompt)
}

func ConfirmShutdown(input io.Reader, output io.Writer, report domain.ShutdownPlanReport) (bool, error) {
	label := "shutdown"
	if report.Action == domain.ClientRestart {
		label = "restart"
	}
	fmt.Fprintf(output, "Client %s review\n", label)
	fmt.Fprintf(output, "Targets: %d eligible of %s selected\n", report.Eligible, countNoun(len(report.Targets), "computer"))
	fmt.Fprintln(output, "Controller: always excluded")
	fmt.Fprintf(output, "Session safety: %s\n", shutdownPolicyLabel(report.Policy))
	fmt.Fprintln(output, "Impact: unsaved user work may be lost; checks run again before dispatch")
	if active := shutdownActiveCount(report); active > 0 {
		fmt.Fprintf(output, "Explicit confirmation: %s authorizes %s of %s with an active user session\n", report.Action.Confirmation(), label, countNoun(active, "computer"))
	}
	if unknown := shutdownUnknownCount(report); unknown > 0 {
		fmt.Fprintf(output, "Explicit confirmation: %s authorizes %s of %s whose session state is unknown\n", report.Action.Confirmation(), label, countNoun(unknown, "computer"))
	}
	if unused := shutdownUnusedCount(report); unused > 0 {
		fmt.Fprintf(output, "Logged in but not in use: %s without recent keyboard or mouse input\n", countNoun(unused, "computer"))
	}
	fmt.Fprintln(output, "Outcome: acceptance confirms only that the operating system received the request, not physical power state")
	fmt.Fprintln(output, "Retry: do not retry an unconfirmed request blindly")
	if !report.InterruptsSessions() {
		return ConfirmYesNo(input, output, continuePrompt)
	}
	return ConfirmWord(input, output, report.Action.Confirmation())
}

// confirmYesNo renders a plan and its prompt, then reads one line; "y" in any
// case approves, anything else declines.
func confirmYesNo(input io.Reader, output io.Writer, render func(io.Writer)) (bool, error) {
	render(output)
	line, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && len(line) == 0 {
		return false, err
	}
	return strings.EqualFold(strings.TrimSpace(line), "y"), nil
}

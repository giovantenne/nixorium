package presentation

import (
	"fmt"
	"io"

	"github.com/giovantenne/nixorium/internal/domain"
)

// RemoteInstallCloseNeedsWord reports whether closing this installation asks
// for the typed CLOSE: only a record that never reached the disk closes with
// a yes/no answer (ADR 0026).
func RemoteInstallCloseNeedsWord(response domain.RemoteInstallResponse) bool {
	return remoteInstallCloseNeedsWord(response)
}

// RemoteInstallCloseExplanation says in one sentence what closing means.
func RemoteInstallCloseExplanation(response domain.RemoteInstallResponse) string {
	if remoteInstallCloseNeedsWord(response) {
		return "CLOSE stops following this installation and frees the controller. The disk may have been written, and Nixorium does not check that the computer starts correctly: check it, or install it again."
	}
	return "The disk was not touched: closing discards this installation's local access and frees the computer, without confirming that the temporary access was removed from the computer started from the USB stick. A new installation needs a new review."
}

func RemoteInstallResponseText(writer io.Writer, response domain.RemoteInstallResponse) {
	RemoteInstallFindingsText(writer, response)
	fmt.Fprintf(writer, "USB SSH installation: %s\n", response.State)
	if response.OperationID != "" {
		fmt.Fprintf(writer, "Operation ID:        %s\n", response.OperationID)
	}
	if response.Message != "" {
		fmt.Fprintf(writer, "Detail:              %s\n", response.Message)
	}
	if response.Session != nil {
		session := response.Session
		if session.Artifacts != nil {
			fmt.Fprintf(writer, "Prepared host:       %s (%s, %s)\n", session.Artifacts.HostName, session.Artifacts.HostInterface, session.Artifacts.HostStaticIP)
			fmt.Fprintf(writer, "Prepared revision:   %s\n", session.Artifacts.DeploymentRevision)
			fmt.Fprintf(writer, "Prepared system:     %s\n", session.Artifacts.SystemPath)
		}
		if session.Bootstrap != nil {
			fmt.Fprintf(writer, "Verified live ISO:   %s at %s\n", session.Bootstrap.Host, session.Bootstrap.Address)
			fmt.Fprintf(writer, "Host fingerprint:    %s\n", session.Bootstrap.HostFingerprint)
		}
		if session.Preparation != nil {
			fmt.Fprintf(writer, "Static identity:     %s\n", session.Preparation.Host.StaticIP)
			fmt.Fprintf(writer, "Cache endpoint:      %s\n", session.Preparation.Cache.URL)
			if session.Preparation.KnownHostConflict {
				fmt.Fprintln(writer, "Known host:          CONFLICT — a separate rotation review is required")
			}
			fmt.Fprintln(writer, "Observed disks:")
			for _, disk := range session.Preparation.Facts.Disks {
				status := "eligible"
				if !disk.Eligible {
					status = "excluded"
				}
				fmt.Fprintf(writer, "  %s  %d bytes  %s  %s  [%s]\n", disk.Path, disk.SizeBytes, unescapeUdev(disk.Model), disk.Serial, status)
				for _, reason := range disk.ExclusionReasons {
					fmt.Fprintf(writer, "    - %s\n", reason)
				}
			}
		}
	}
	if response.Plan != nil {
		plan := response.Plan
		fmt.Fprintf(writer, "Reviewed host:       %s (%s -> %s)\n", plan.Host.Name, plan.Host.LiveIP, plan.Host.StaticIP)
		fmt.Fprintf(writer, "Reviewed disk:       %s (%d bytes, serial %s, WWN %s)\n", plan.Disk.Path, plan.Disk.SizeBytes, plan.Disk.Serial, plan.Disk.WWN)
		fmt.Fprintf(writer, "Reviewed revision:   %s\n", plan.Revision)
		fmt.Fprintf(writer, "Host-key rotation:   %t\n", plan.HostKeyRotation)
		fmt.Fprintf(writer, "Review token:        %s\n", plan.ReviewToken)
		fmt.Fprintf(writer, "Confirmation:        %s\n", plan.Confirmation)
	}
	if response.Execution != nil {
		report := response.Execution
		fmt.Fprintf(writer, "Phase:               %s\n", report.Phase)
		fmt.Fprintf(writer, "Disk may be modified: %t\n", report.DiskMayBeModified)
		fmt.Fprintf(writer, "Installed:           %t\n", report.Installed)
		fmt.Fprintf(writer, "Reboot requested:    %t\n", report.RebootRequested)
		fmt.Fprintf(writer, "Boot verified:       %t\n", report.BootVerified)
		if report.LogID != "" {
			fmt.Fprintf(writer, "Operation log:       %s\n", report.LogID)
		}
	}
}

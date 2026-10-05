package domain

import (
	"fmt"
	"strings"
)

// SettingImpact says what a reviewed settings change means after saving.
// Warning impacts need particular care before applying.
type SettingImpact struct {
	Kind    string `json:"kind"`
	Detail  string `json:"detail"`
	Warning bool   `json:"warning,omitempty"`
}

// Fields that do not reach client computers.
var controllerOnlySettings = map[string]bool{
	"lab.masterDhcpIp": true, "lab.controllerIfaceName": true, "lab.adminGitName": true, "lab.adminGitEmail": true,
}

// SettingsTransitionIssues protects the management connection while clients are
// configured. Use the saved count, including when the candidate removes clients.
// This is a transition rule, not a restriction on valid initial configurations.
func SettingsTransitionIssues(base, candidate LabSettingsFile) []ValidationIssue {
	if base.Lab.PCCount <= 0 {
		return nil
	}
	var issues []ValidationIssue
	for _, change := range DiffLabSettings(base, candidate) {
		switch change.Field {
		case "lab.networkBase", "lab.networkPrefixLength", "lab.masterHostNumber":
			issues = append(issues, ValidationIssue{Field: change.Field, Message: fmt.Sprintf(
				"Cannot change laboratory addressing while %d client computers are configured. They would keep their old network settings and could lose contact with the controller. Keep the saved value; guided network migration is not available yet.", base.Lab.PCCount)})
		}
	}
	return issues
}

// SettingsImpacts classifies reviewed changes into what happens next.
func SettingsImpacts(base, candidate LabSettingsFile, changes []SettingChange) []SettingImpact {
	if len(changes) == 0 {
		return nil
	}
	changed := map[string]bool{}
	clients, passwords, accounts := false, false, false
	for _, change := range changes {
		changed[change.Field] = true
		if !controllerOnlySettings[change.Field] {
			clients = true
		}
		switch change.Field {
		case "lab.adminPassword", "lab.teacherPassword", "lab.studentPassword":
			passwords = true
		case "lab.teacherUser", "lab.studentUser":
			accounts = true
		}
	}
	impacts := []SettingImpact{{Kind: "controller-apply", Detail: "Apply to controller so this controller uses the saved settings."}}
	if clients && candidate.Lab.DeploymentMode != "controller" {
		impacts = append(impacts, SettingImpact{Kind: "client-update", Detail: "Update computers so the clients use the saved settings; offline computers keep the old ones until updated."})
	}
	if changed["lab.networkBase"] || changed["lab.networkPrefixLength"] || changed["lab.masterHostNumber"] {
		impacts = append(impacts, SettingImpact{Kind: "client-addresses", Warning: true,
			Detail: "Applying these settings changes the laboratory addresses or routing. Check the proposed subnet and controller address before applying locally; the current connection may close."})
	}
	if changed["lab.masterDhcpIp"] {
		impacts = append(impacts, SettingImpact{Kind: "pxe-address", Detail: "This changes only the controller address hint for PXE preparation. It does not change the DHCP lease or the static addresses used to manage installed computers. Prepare network installation again before using PXE."})
	}
	for _, change := range changes {
		var detail string
		switch change.Field {
		case "lab.controllerIfaceName":
			detail = "Applying to the controller can move its laboratory connection to another network device and disconnect clients or this session. An empty value uses the shared fallback."
		case "lab.clientIfaceName":
			detail = "Updating clients can move their laboratory connection to another network device and make them unreachable. An empty value uses the shared fallback; per-computer overrides still take priority."
		case "lab.ifaceName":
			detail = "Changing the shared fallback can disconnect the controller and clients that have no interface override when their settings are applied."
		case "lab.hostIfaceNames":
			detail = "Changing per-computer overrides can disconnect the affected computers when their settings are applied. Overrides take priority over controller, client and shared settings."
		}
		if detail != "" {
			impacts = append(impacts, SettingImpact{Kind: "network-interface", Warning: true,
				Detail: detail + " Verify the old and new device names, cabling and access to each affected computer's local console before applying."})
		}
	}
	if candidate.Lab.PCCount < base.Lab.PCCount {
		impacts = append(impacts, SettingImpact{Kind: "computers-removed", Warning: true,
			Detail: fmt.Sprintf("Computers %s are no longer managed; they keep running their last configuration.", computerRange(candidate.Lab.PCCount+1, base.Lab.PCCount))})
	}
	if candidate.Lab.PCCount > base.Lab.PCCount {
		impacts = append(impacts, SettingImpact{Kind: "computers-added",
			Detail: fmt.Sprintf("Install the new computers %s after preparing network installation again.", computerRange(base.Lab.PCCount+1, candidate.Lab.PCCount))})
	}
	if passwords {
		impacts = append(impacts, SettingImpact{Kind: "passwords", Detail: "Passwords change on this controller when applied and on each computer when it is updated."})
	}
	if accounts {
		impacts = append(impacts, SettingImpact{Kind: "accounts", Warning: true, Detail: "A renamed account gets a new, empty home; files of the old account are not moved."})
	}
	if clients && candidate.Lab.DeploymentMode != "controller" {
		impacts = append(impacts, SettingImpact{Kind: "pxe-prepare", Detail: "Prepare network installation again before installing more computers."})
	}
	return impacts
}

func computerRange(first, last int) string {
	if first == last {
		return fmt.Sprintf("pc%02d", first)
	}
	return strings.Join([]string{fmt.Sprintf("pc%02d", first), fmt.Sprintf("pc%02d", last)}, "–")
}

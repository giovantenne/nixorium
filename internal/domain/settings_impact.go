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
//
// The laboratory network (base and prefix) may change only as a change of
// laboratory network: the candidate records the saved network as the previous
// one, so the controller keeps both addresses and the clients accept both
// controller addresses until the record is removed. The controller number
// stays locked.
func SettingsTransitionIssues(base, candidate LabSettingsFile) []ValidationIssue {
	if base.Lab.PCCount <= 0 {
		return nil
	}
	migration := movesFromSavedNetwork(base, candidate) || undoesSavedChange(base, candidate)
	var issues []ValidationIssue
	for _, change := range DiffLabSettings(base, candidate) {
		switch change.Field {
		case "lab.networkBase", "lab.networkPrefixLength":
			if !migration {
				issues = append(issues, BlockedIssue(change.Field, "NETWORK-LOCKED", NetworkLockedMessage(base.Lab.PCCount)))
			}
		case "lab.networkTransition":
			// Removing the record finishes a change; any other edit must
			// come with the network it describes.
			if candidate.Lab.NetworkTransition != nil && !migration {
				issues = append(issues, BlockedIssue(change.Field, "NETWORK-LOCKED", NetworkLockedMessage(base.Lab.PCCount)))
			}
		case "lab.masterHostNumber":
			issues = append(issues, BlockedIssue(change.Field, "NETWORK-LOCKED", ControllerNumberLockedMessage(base.Lab.PCCount)))
		}
	}
	return issues
}

// movesFromSavedNetwork reports whether the candidate is a change of
// laboratory network away from the saved one: it records the saved base and
// prefix as the previous network. While a change is already in progress only
// going back to its previous network is accepted, because computers still on
// that network would otherwise lose the controller.
func movesFromSavedNetwork(base, candidate LabSettingsFile) bool {
	transition := candidate.Lab.NetworkTransition
	if transition == nil || transition.PreviousBase != base.Lab.NetworkBase || transition.PreviousPrefixLength != base.Lab.NetworkPrefix {
		return false
	}
	if saved := base.Lab.NetworkTransition; saved != nil {
		return candidate.Lab.NetworkBase == saved.PreviousBase && candidate.Lab.NetworkPrefix == saved.PreviousPrefixLength
	}
	return true
}

// undoesSavedChange reports whether the candidate undoes the saved change of
// laboratory network: it restores the previous network and removes the
// record. Until the controller is applied it keeps both addresses, and
// clients keep the previous address during the change, so every computer
// stays reachable on the restored network.
func undoesSavedChange(base, candidate LabSettingsFile) bool {
	saved := base.Lab.NetworkTransition
	return saved != nil && candidate.Lab.NetworkTransition == nil &&
		candidate.Lab.NetworkBase == saved.PreviousBase && candidate.Lab.NetworkPrefix == saved.PreviousPrefixLength
}

func configuredComputers(computers int) string {
	if computers == 1 {
		return "1 computer is"
	}
	return fmt.Sprintf("%d computers are", computers)
}

// NetworkLockedMessage is the refusal for an addressing change while
// computers are configured (situation NETWORK-LOCKED).
func NetworkLockedMessage(computers int) string {
	return fmt.Sprintf("The laboratory network cannot be changed while %s configured: they would keep their old addresses and lose the controller. To move them to a new network use Maintenance → Change network addressing; here keep the saved value.", configuredComputers(computers))
}

// ControllerNumberLockedMessage is the refusal for a controller number change
// while computers are configured (situation NETWORK-LOCKED).
func ControllerNumberLockedMessage(computers int) string {
	return fmt.Sprintf("The controller number cannot be changed while %s configured: it sets the controller's laboratory address, so they would lose the controller. Keep the saved value.", configuredComputers(computers))
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
	if (changed["lab.clientIfaceName"] || changed["lab.ifaceName"] || changed["lab.hostIfaceNames"]) && candidate.Lab.DeploymentMode != "controller" {
		impacts = append(impacts, SettingImpact{Kind: "network-interface-order",
			Detail: "Update one computer first and check it answers before updating the others."})
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

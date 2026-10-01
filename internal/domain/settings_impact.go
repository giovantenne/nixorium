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
			Detail: "This renumbers the laboratory. Installed computers keep their old addresses and cannot be updated at the new ones; there is no guided address change yet. Keep the current network unless you will reinstall the computers."})
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

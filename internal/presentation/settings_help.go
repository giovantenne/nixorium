package presentation

import (
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

type settingHelp struct{ description, example string }

var settingsHelp = map[string]settingHelp{
	"lab.controllerIfaceName": {"Network device on this controller; empty uses the shared fallback.", "enp1s0"},
	"lab.clientIfaceName":     {"Network device used to connect client computers to the laboratory.", "enp1s0"},
	"lab.ifaceName":           {"Device used when no controller, client or host override is set.", "enp1s0"},
	"lab.masterDhcpIp":        {"PXE address hint only; changing it does not change the DHCP lease or static addresses.", "192.168.1.20"},
	"lab.networkBase":         {"Static lab network; must not contain the controller DHCP address.", "10.0.0.0"},
	"lab.networkPrefixLength": {"Network size in CIDR bits; /24 contains 254 usable host addresses.", "24"},
	"lab.pcCount":             {"Clients are numbered from pc01; do not include the controller.", "20"},
	"lab.masterHostNumber":    {"Controller number/address offset; must be above all client numbers.", "99"},
	"lab.teacherUser":         {"Teacher account name, distinct from administrator and student.", "teacher"},
	"lab.studentUser":         {"Shared student account whose home is reset at computer startup.", "student"},
	"lab.timeZone":            {"Local clock and daylight-saving rules for laboratory computers.", "Europe/Rome"},
	"lab.keyboardLayout":      {"Keyboard layout used for typing; also selects a known console keymap.", "it"},
	"lab.homepageUrl":         {"Initial browser homepage in the student environment.", "https://school.example/"},
	"lab.studentGitName":      {"Author name recorded by Git commits made as the student.", "Student"},
	"lab.studentGitEmail":     {"Author email recorded by Git commits made as the student.", "student@example.org"},
	"lab.adminGitName":        {"Author name for the administrator's Git commits.", "Lab Administrator"},
	"lab.adminGitEmail":       {"Author email for the administrator's Git commits.", "admin@example.org"},
}

func settingLabel(id string) string {
	for _, field := range settingsFields {
		if field.id == id {
			return field.label
		}
	}
	for _, group := range routineSettingsGroups {
		for _, field := range group.fields {
			if field.id == id {
				return field.label
			}
		}
	}
	switch id {
	case "lab.adminPassword":
		return "Administrator password"
	case "lab.teacherPassword":
		return "Teacher password"
	case "lab.studentPassword":
		return "Student password"
	case "lab.network":
		return "Laboratory network"
	case "lab.consoleKeyMap":
		return "Console keyboard layout"
	case "lab.defaultLocale":
		return "Desktop language"
	case "lab.extraLocale":
		return "Regional formats"
	case "lab.deploymentMode":
		return "Laboratory mode"
	}
	return id
}

func (model settingsWizardModel) networkPreview() []string {
	field := model.fields[model.index].id
	if !strings.HasPrefix(field, "lab.network") && field != "lab.masterDhcpIp" && field != "lab.pcCount" && field != "lab.masterHostNumber" {
		return nil
	}
	candidate := model.settings
	for index, field := range model.fields {
		next, err := setSettingField(candidate, field.id, model.drafts[index])
		if err != nil {
			return []string{"Address preview unavailable until the draft values are valid."}
		}
		candidate = next
	}
	controller, err := domain.ControllerStaticAddress(candidate.Lab)
	if err != nil {
		return []string{"Address preview unavailable until network and host number are valid."}
	}
	lines := []string{tuiFieldDetail("Controller static address", controller, model.isDark)}
	if candidate.Lab.PCCount > 0 {
		lab := candidate.Lab
		lab.MasterHostNumber = 1
		first, e1 := domain.ControllerStaticAddress(lab)
		lab.MasterHostNumber = lab.PCCount
		last, e2 := domain.ControllerStaticAddress(lab)
		if e1 == nil && e2 == nil {
			lines = append(lines, tuiFieldDetail("Client addresses", fmt.Sprintf("%s – %s", first, last), model.isDark))
		}
	}
	for _, issue := range candidate.Validate() {
		if issue.Field == "lab.network" {
			lines = append(lines, "! DHCP overlaps the static lab network; saving is blocked.")
		}
	}
	return lines
}

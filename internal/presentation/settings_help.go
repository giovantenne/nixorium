package presentation

import (
	"fmt"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

type settingHelp struct{ description, example string }

var settingsHelp = map[string]settingHelp{
	"lab.controllerIfaceName": {"Network device on this controller; empty uses the shared fallback.", "enp1s0"},
	"lab.clientIfaceName":     {"Leave empty (recommended) to use each client computer's wired network card whatever its name. Set a name only when every client has the same card name; use a per-computer override for other hardware.", "enp1s0"},
	"lab.ifaceName":           {"Device used by the controller when no controller override is set; clients ignore it unless a client or computer name is set.", "enp1s0"},
	"lab.masterDhcpIp":        {"PXE address hint only; changing it does not change the DHCP lease or static addresses.", "192.168.1.20"},
	"lab.networkBase":         {"Static lab network; must not contain the controller DHCP address.", "10.0.0.0"},
	"lab.networkPrefixLength": {"Network size in CIDR bits; /24 contains 254 usable host addresses.", "24"},
	"lab.pcCount":             {"Clients are numbered from pc01; do not include the controller.", "20"},
	"lab.masterHostNumber":    {"Controller number/address offset; must be above all client numbers.", "99"},
	"lab.teacherUser":         {"Teacher account name, distinct from administrator and student.", "teacher"},
	"lab.studentUser":         {"Shared student account whose home is reset at computer startup.", "student"},
	"lab.timeZone":            {"Local clock and daylight-saving rules for laboratory computers.", "Europe/Rome"},
	"lab.keyboardLayout":      {"Keyboard layout used for typing at the login screen, in the desktop and on the text console.", "it"},
	"lab.homepageUrl":         {"Initial browser homepage in the student environment.", "https://school.example/"},
	"lab.studentGitName":      {"Author name recorded by Git commits made as the student.", "Student"},
	"lab.studentGitEmail":     {"Author email recorded by Git commits made as the student.", "student@example.org"},
	"lab.adminGitName":        {"Author name for the administrator's Git commits.", "Lab Administrator"},
	"lab.classroomView":       {"On (the default) gives the teacher the classroom view: screen previews, taking control, locking screens, messages, showing the teacher's screen, sending and collecting files. Off removes it from the controller and from the student computers; Internet pause, switching computers on and off and the schedule keep working.", "on"},
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
	case "lab.networkTransition":
		return "Change of laboratory network"
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

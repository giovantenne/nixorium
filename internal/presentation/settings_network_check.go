package presentation

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/giovantenne/nixorium/internal/domain"
)

// loadSettingsMsg reads the settings for the editor and, read-only, the
// controller's current addresses offered for its DHCP address.
func (model dashboardModel) loadSettingsMsg(ctx context.Context) tea.Msg {
	settings, err := model.actions.LoadSettings(ctx)
	message := dashboardSettingsMsg{settings: settings, err: err, repair: model.settingsRepairIssues()}
	if err == nil && model.actions.LoadControllerAddresses != nil {
		message.addresses = model.actions.LoadControllerAddresses(ctx, settings)
	}
	return message
}

// newSettingsEditor opens a settings form sized for this screen. When the
// controller's addresses were read, the DHCP address becomes a choice among
// them, with Other… for a typed value.
func (model dashboardModel) newSettingsEditor(settings domain.LabSettingsFile, fields []settingsField, title string) settingsWizardModel {
	editor := newSettingsEditorModel(settings, withDetectedDHCPChoices(fields, model.settings.addresses), title)
	editor.width, editor.height, editor.isDark = model.width, model.height, model.isDark
	editor.prepareCurrentField()
	return editor
}

func withDetectedDHCPChoices(fields []settingsField, addresses domain.ControllerAddresses) []settingsField {
	if !addresses.Detected {
		return fields
	}
	updated := append([]settingsField(nil), fields...)
	for index, field := range updated {
		if field.id == "lab.controllerIfaceName" && len(addresses.Wired) > 0 {
			// The controller's wired cards, with what they carry now.
			field.choices = []settingsChoice{}
			field.detected = []string{}
			for _, device := range addresses.Wired {
				carried := []string{}
				for _, address := range device.Addresses {
					if !strings.HasPrefix(address.Address, "fe80:") {
						carried = append(carried, address.Address)
					}
				}
				description := "Wired card without an address now"
				if len(carried) > 0 {
					description = "Wired card · " + strings.Join(carried, ", ")
				}
				field.choices = append(field.choices, settingsChoice{value: device.Name, label: device.Name, description: description})
				field.detected = append(field.detected, device.Name)
			}
			field.other = "Other…"
			updated[index] = field
			continue
		}
		if field.id != "lab.masterDhcpIp" {
			continue
		}
		field.choices = []settingsChoice{}
		field.detected = []string{}
		for _, choice := range addresses.Choices {
			field.choices = append(field.choices, settingsChoice{value: choice.Address, label: choice.Address, description: "Current address on " + choice.Device, device: choice.Device})
			field.detected = append(field.detected, choice.Address)
		}
		field.other = "Other…"
		updated[index] = field
	}
	return updated
}

// settingsNetworkCheckLines shows the review's read-only network check: the
// controller's devices and, per affected client, whether its device exists.
func (model dashboardModel) settingsNetworkCheckLines() []string {
	preflight := model.settings.plan.NetworkPreflight
	if preflight == nil {
		return nil
	}
	lines := []string{"", tuiSection("Network check", model.isDark)}
	if check := preflight.Controller; check != nil {
		kind := tuiStatusSuccess
		switch {
		case check.Blocking:
			kind = tuiStatusFailure
		case check.State != domain.DevicePresent || (check.DHCPDevice != "" && check.DHCPDevice != check.Interface):
			kind = tuiStatusAttention
		}
		lines = append(lines, "  "+tuiStatus(check.Message, kind, model.isDark))
	}
	if check := preflight.DHCPAddress; check != nil && !check.Found {
		lines = append(lines, "  "+tuiStatus(check.Message, tuiStatusAttention, model.isDark))
	}
	if len(preflight.Clients) > 0 {
		lines = append(lines, "  Client computers, checked now without changing them:")
	}
	for _, client := range preflight.Clients {
		switch client.State {
		case domain.DevicePresent:
			lines = append(lines, "  "+tuiStatus(fmt.Sprintf("%s  device present (%s)", client.Name, client.Interface), tuiStatusSuccess, model.isDark))
		case domain.DeviceMissing:
			devices := "none"
			if len(client.Devices) > 0 {
				devices = strings.Join(client.Devices, ", ")
			}
			kind := tuiStatusFailure
			if model.settings.saveAnyway {
				kind = tuiStatusAttention
			}
			lines = append(lines, "  "+tuiStatus(fmt.Sprintf("%s  device missing: no %s (its devices: %s)", client.Name, client.Interface, devices), kind, model.isDark))
		default:
			lines = append(lines, "  "+tuiStatus(client.Name+"  not reachable: check it at its screen before updating it", tuiStatusNeutral, model.isDark))
		}
	}
	if missing := preflight.Missing(); len(missing) > 0 {
		if model.settings.saveAnyway {
			lines = append(lines, "  "+tuiStatus("Saving anyway for "+strings.Join(missing, ", ")+": updated with this name, a computer without the device loses the laboratory network.", tuiStatusAttention, model.isDark))
		} else {
			lines = append(lines, "  Correct the name, or press c to save anyway for the computers listed.")
		}
	}
	if len(lines) == 2 {
		// Only the heading: nothing to report, so no heading either.
		return nil
	}
	return lines
}

// reviewedSettingsPlan is the reviewed plan with the computers acknowledged
// on this screen; the save checks them again.
func (model dashboardModel) reviewedSettingsPlan() domain.ConfigPlanReport {
	plan := model.settings.plan
	if plan.NetworkPreflight != nil {
		preflight := *plan.NetworkPreflight
		preflight.Acknowledged = nil
		if model.settings.saveAnyway {
			preflight.Acknowledged = preflight.Missing()
		}
		plan.NetworkPreflight = &preflight
	}
	return plan
}

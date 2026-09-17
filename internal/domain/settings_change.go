package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
)

var ErrSettingsConflict = errors.New("settings changed since review")

type SettingChange struct {
	Field     string `json:"field"`
	Before    any    `json:"before"`
	After     any    `json:"after"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

type ConfigPlanReport struct {
	SchemaVersion        int               `json:"schemaVersion"`
	Operation            string            `json:"operation"`
	State                string            `json:"state"`
	Repository           string            `json:"repository"`
	File                 string            `json:"file"`
	BaseFingerprint      string            `json:"baseFingerprint,omitempty"`
	CandidateFingerprint string            `json:"candidateFingerprint,omitempty"`
	Changes              []SettingChange   `json:"changes"`
	Issues               []ValidationIssue `json:"issues"`
}

func (r ConfigPlanReport) HasErrors() bool {
	return len(r.Issues) > 0
}

type ConfigApplyReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	File          string            `json:"file"`
	Changes       []SettingChange   `json:"changes"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r ConfigApplyReport) HasErrors() bool {
	return r.State == "invalid" || r.State == "conflict"
}

func SettingsFingerprint(data []byte) string {
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func DiffLabSettings(before, after LabSettingsFile) []SettingChange {
	changes := make([]SettingChange, 0)
	add := func(field string, oldValue, newValue any) {
		if !reflect.DeepEqual(oldValue, newValue) {
			changes = append(changes, SettingChange{Field: field, Before: oldValue, After: newValue})
		}
	}
	addSecret := func(field, oldValue, newValue string) {
		if oldValue != newValue {
			changes = append(changes, SettingChange{Field: field, Before: "configured", After: "updated", Sensitive: true})
		}
	}

	add("lab.deploymentMode", before.Lab.DeploymentMode, after.Lab.DeploymentMode)
	add("lab.masterDhcpIp", before.Lab.MasterDHCPIP, after.Lab.MasterDHCPIP)
	add("lab.networkBase", before.Lab.NetworkBase, after.Lab.NetworkBase)
	add("lab.networkPrefixLength", before.Lab.NetworkPrefix, after.Lab.NetworkPrefix)
	add("lab.pcCount", before.Lab.PCCount, after.Lab.PCCount)
	add("lab.masterHostNumber", before.Lab.MasterHostNumber, after.Lab.MasterHostNumber)
	add("lab.ifaceName", before.Lab.InterfaceName, after.Lab.InterfaceName)
	add("lab.controllerIfaceName", before.Lab.ControllerInterfaceName, after.Lab.ControllerInterfaceName)
	add("lab.clientIfaceName", before.Lab.ClientInterfaceName, after.Lab.ClientInterfaceName)
	add("lab.hostIfaceNames", before.Lab.HostInterfaceNames, after.Lab.HostInterfaceNames)
	add("lab.teacherUser", before.Lab.TeacherUser, after.Lab.TeacherUser)
	add("lab.studentUser", before.Lab.StudentUser, after.Lab.StudentUser)
	addSecret("lab.teacherPassword", before.Lab.TeacherPassword, after.Lab.TeacherPassword)
	addSecret("lab.studentPassword", before.Lab.StudentPassword, after.Lab.StudentPassword)
	addSecret("lab.adminPassword", before.Lab.AdminPassword, after.Lab.AdminPassword)
	add("lab.homepageUrl", before.Lab.HomepageURL, after.Lab.HomepageURL)
	add("lab.studentGitName", before.Lab.StudentGitName, after.Lab.StudentGitName)
	add("lab.studentGitEmail", before.Lab.StudentGitEmail, after.Lab.StudentGitEmail)
	add("lab.adminGitName", before.Lab.AdminGitName, after.Lab.AdminGitName)
	add("lab.adminGitEmail", before.Lab.AdminGitEmail, after.Lab.AdminGitEmail)
	add("lab.timeZone", before.Lab.TimeZone, after.Lab.TimeZone)
	add("lab.defaultLocale", before.Lab.DefaultLocale, after.Lab.DefaultLocale)
	add("lab.extraLocale", before.Lab.ExtraLocale, after.Lab.ExtraLocale)
	add("lab.keyboardLayout", before.Lab.KeyboardLayout, after.Lab.KeyboardLayout)
	add("lab.consoleKeyMap", before.Lab.ConsoleKeyMap, after.Lab.ConsoleKeyMap)
	add("lab.veyonNativeHosts", before.Lab.VeyonNativeHosts, after.Lab.VeyonNativeHosts)
	return changes
}

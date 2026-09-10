package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

const (
	SettingsSchemaVersion = 1
	MasterDHCPPlaceholder = "MASTER_DHCP_IP"
	DefaultPasswordHash   = "$6$t.4PBRDwSMnGbuzA$fLuu1n700q.Mvj0ivauGLPQJcfT6XnFMkDh6T0GMWH/hzlSNuzxfh0bxh2iQR027y7PSdzuIvWoO3NgRbM/gV0"
)

var (
	interfaceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,14}$`)
	userNamePattern      = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,30}$`)
	passwordHashPattern  = regexp.MustCompile(`^\$6\$[^$]+\$[^$]+$`)
)

type LabSettingsFile struct {
	SchemaVersion int         `json:"schemaVersion"`
	Lab           LabSettings `json:"lab"`
}

type LabSettings struct {
	MasterDHCPIP     string   `json:"masterDhcpIp"`
	NetworkBase      string   `json:"networkBase"`
	NetworkPrefix    int      `json:"networkPrefixLength"`
	PCCount          int      `json:"pcCount"`
	MasterHostNumber int      `json:"masterHostNumber"`
	InterfaceName    string   `json:"ifaceName"`
	TeacherUser      string   `json:"teacherUser"`
	StudentUser      string   `json:"studentUser"`
	TeacherPassword  string   `json:"teacherPassword"`
	StudentPassword  string   `json:"studentPassword"`
	AdminPassword    string   `json:"adminPassword"`
	HomepageURL      string   `json:"homepageUrl"`
	StudentGitName   string   `json:"studentGitName"`
	StudentGitEmail  string   `json:"studentGitEmail"`
	AdminGitName     string   `json:"adminGitName"`
	AdminGitEmail    string   `json:"adminGitEmail"`
	TimeZone         string   `json:"timeZone"`
	DefaultLocale    string   `json:"defaultLocale"`
	ExtraLocale      string   `json:"extraLocale"`
	KeyboardLayout   string   `json:"keyboardLayout"`
	ConsoleKeyMap    string   `json:"consoleKeyMap"`
	VeyonNativeHosts []string `json:"veyonNativeHosts"`
}

type ValidationIssue struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type ConfigValidationReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	Operation     string            `json:"operation"`
	State         string            `json:"state"`
	Repository    string            `json:"repository"`
	File          string            `json:"file"`
	Issues        []ValidationIssue `json:"issues"`
}

func (r ConfigValidationReport) HasErrors() bool {
	return len(r.Issues) > 0
}

func DecodeLabSettings(data []byte) (LabSettingsFile, []ValidationIssue) {
	var settings LabSettingsFile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return settings, []ValidationIssue{{Field: "$", Message: fmt.Sprintf("invalid settings JSON: %v", err)}}
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return settings, []ValidationIssue{{Field: "$", Message: "settings file contains trailing data"}}
	}
	return settings, settings.Validate()
}

func (s LabSettingsFile) Validate() []ValidationIssue {
	issues := make([]ValidationIssue, 0)
	add := func(field, message string) {
		issues = append(issues, ValidationIssue{Field: field, Message: message})
	}
	if s.SchemaVersion != SettingsSchemaVersion {
		add("schemaVersion", fmt.Sprintf("must be %d", SettingsSchemaVersion))
	}

	lab := s.Lab
	if lab.MasterDHCPIP != MasterDHCPPlaceholder {
		if address, err := netip.ParseAddr(lab.MasterDHCPIP); err != nil || !address.Is4() {
			add("lab.masterDhcpIp", "must be an IPv4 address or MASTER_DHCP_IP")
		}
	}
	prefix, prefixErr := netip.ParsePrefix(fmt.Sprintf("%s/%d", lab.NetworkBase, lab.NetworkPrefix))
	if prefixErr != nil || !prefix.Addr().Is4() || lab.NetworkPrefix < 1 || lab.NetworkPrefix > 30 {
		add("lab.networkBase", "must be an IPv4 network address with a prefix length from 1 to 30")
	} else if prefix.Masked() != prefix {
		add("lab.networkBase", fmt.Sprintf("must be aligned to /%d", lab.NetworkPrefix))
	}
	if lab.PCCount < 1 || lab.PCCount > 253 {
		add("lab.pcCount", "must be between 1 and 253")
	}
	if lab.MasterHostNumber < 1 || lab.MasterHostNumber > 254 {
		add("lab.masterHostNumber", "must be between 1 and 254")
	} else {
		if lab.MasterHostNumber <= lab.PCCount {
			add("lab.masterHostNumber", "must be greater than pcCount")
		}
		if prefixErr == nil && lab.NetworkPrefix >= 1 && lab.NetworkPrefix <= 30 {
			hostCapacity := 1 << (32 - lab.NetworkPrefix)
			if lab.MasterHostNumber >= hostCapacity-1 {
				add("lab.masterHostNumber", "does not fit in the configured subnet")
			}
		}
	}
	if !interfaceNamePattern.MatchString(lab.InterfaceName) {
		add("lab.ifaceName", "must be a valid Linux interface name of at most 15 characters")
	}
	validateUser := func(field, value string) {
		if !userNamePattern.MatchString(value) {
			add(field, "must be a valid Unix user name")
		} else if value == "root" || value == "admin" {
			add(field, "must not be root or admin")
		}
	}
	validateUser("lab.teacherUser", lab.TeacherUser)
	validateUser("lab.studentUser", lab.StudentUser)
	if lab.TeacherUser != "" && lab.TeacherUser == lab.StudentUser {
		add("lab.studentUser", "must be different from teacherUser")
	}
	for _, password := range []struct {
		field string
		value string
	}{
		{"lab.teacherPassword", lab.TeacherPassword},
		{"lab.studentPassword", lab.StudentPassword},
		{"lab.adminPassword", lab.AdminPassword},
	} {
		if !passwordHashPattern.MatchString(password.value) {
			add(password.field, "must be a SHA-512 crypt hash beginning with $6$")
		}
	}
	parsedURL, urlErr := url.Parse(lab.HomepageURL)
	if urlErr != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		add("lab.homepageUrl", "must be an absolute http:// or https:// URL")
	}
	for _, required := range []struct {
		field string
		value string
	}{
		{"lab.studentGitName", lab.StudentGitName},
		{"lab.studentGitEmail", lab.StudentGitEmail},
		{"lab.adminGitName", lab.AdminGitName},
		{"lab.adminGitEmail", lab.AdminGitEmail},
		{"lab.timeZone", lab.TimeZone},
		{"lab.defaultLocale", lab.DefaultLocale},
		{"lab.extraLocale", lab.ExtraLocale},
		{"lab.keyboardLayout", lab.KeyboardLayout},
		{"lab.consoleKeyMap", lab.ConsoleKeyMap},
	} {
		if strings.TrimSpace(required.value) == "" {
			add(required.field, "must not be empty")
		}
	}
	validHosts := map[string]bool{fmt.Sprintf("pc%02d", lab.MasterHostNumber): true}
	for number := 1; number <= lab.PCCount; number++ {
		validHosts[fmt.Sprintf("pc%02d", number)] = true
	}
	for index, host := range lab.VeyonNativeHosts {
		if !validHosts[host] {
			add(fmt.Sprintf("lab.veyonNativeHosts[%d]", index), "must name a configured client or controller")
		}
	}
	return issues
}

func MarshalLabSettings(settings LabSettingsFile) ([]byte, error) {
	if issues := settings.Validate(); len(issues) > 0 {
		return nil, fmt.Errorf("settings validation failed: %s: %s", issues[0].Field, issues[0].Message)
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode settings: %w", err)
	}
	return append(data, '\n'), nil
}

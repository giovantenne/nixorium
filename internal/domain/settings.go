package domain

import (
	"bytes"
	"encoding/json"
	"errors"
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
	DeploymentMode          string            `json:"deploymentMode,omitempty"`
	MasterDHCPIP            string            `json:"masterDhcpIp"`
	NetworkBase             string            `json:"networkBase"`
	NetworkPrefix           int               `json:"networkPrefixLength"`
	PCCount                 int               `json:"pcCount"`
	MasterHostNumber        int               `json:"masterHostNumber"`
	InterfaceName           string            `json:"ifaceName"`
	ControllerInterfaceName string            `json:"controllerIfaceName,omitempty"`
	ClientInterfaceName     string            `json:"clientIfaceName,omitempty"`
	HostInterfaceNames      map[string]string `json:"hostIfaceNames,omitempty"`
	TeacherUser             string            `json:"teacherUser"`
	StudentUser             string            `json:"studentUser"`
	TeacherPassword         string            `json:"teacherPassword"`
	StudentPassword         string            `json:"studentPassword"`
	AdminPassword           string            `json:"adminPassword"`
	HomepageURL             string            `json:"homepageUrl"`
	StudentGitName          string            `json:"studentGitName"`
	StudentGitEmail         string            `json:"studentGitEmail"`
	AdminGitName            string            `json:"adminGitName"`
	AdminGitEmail           string            `json:"adminGitEmail"`
	TimeZone                string            `json:"timeZone"`
	DefaultLocale           string            `json:"defaultLocale"`
	ExtraLocale             string            `json:"extraLocale"`
	KeyboardLayout          string            `json:"keyboardLayout"`
	ConsoleKeyMap           string            `json:"consoleKeyMap"`
	VeyonNativeHosts        []string          `json:"veyonNativeHosts"`
}

func (l LabSettings) ControllerInterface() string {
	if l.ControllerInterfaceName != "" {
		return l.ControllerInterfaceName
	}
	return l.InterfaceName
}

func ControllerStaticAddress(lab LabSettings) (string, error) {
	prefix, err := netip.ParsePrefix(fmt.Sprintf("%s/%d", lab.NetworkBase, lab.NetworkPrefix))
	if err != nil || !prefix.Addr().Is4() || prefix.Masked() != prefix || prefix.Bits() < 1 || prefix.Bits() > 30 {
		return "", errors.New("laboratory network is not a canonical IPv4 prefix")
	}
	hostCapacity := uint64(1) << (32 - prefix.Bits())
	if lab.MasterHostNumber < 1 || uint64(lab.MasterHostNumber) >= hostCapacity-1 {
		return "", errors.New("controller host number does not fit in the laboratory prefix")
	}
	octets := prefix.Addr().As4()
	base := uint32(octets[0])<<24 | uint32(octets[1])<<16 | uint32(octets[2])<<8 | uint32(octets[3])
	value := base + uint32(lab.MasterHostNumber)
	candidate := netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
	return candidate.String(), nil
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
	if lab.DeploymentMode != "" && lab.DeploymentMode != "laboratory" && lab.DeploymentMode != "controller" {
		add("lab.deploymentMode", "must be laboratory or controller")
	}
	if lab.DeploymentMode == "controller" {
		if lab.PCCount != 0 {
			add("lab.pcCount", "must be zero in controller mode")
		}
	} else if lab.PCCount < 1 || lab.PCCount > 253 {
		add("lab.pcCount", "must be between 1 and 253 in laboratory mode")
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
	for _, networkInterface := range []struct {
		field string
		value string
	}{
		{"lab.controllerIfaceName", lab.ControllerInterfaceName},
		{"lab.clientIfaceName", lab.ClientInterfaceName},
	} {
		if networkInterface.value != "" && !interfaceNamePattern.MatchString(networkInterface.value) {
			add(networkInterface.field, "must be a valid Linux interface name of at most 15 characters")
		}
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
	for host, interfaceName := range lab.HostInterfaceNames {
		if !validHosts[host] {
			add("lab.hostIfaceNames."+host, "must name a configured client or controller")
		} else if !interfaceNamePattern.MatchString(interfaceName) {
			add("lab.hostIfaceNames."+host, "must be a valid Linux interface name of at most 15 characters")
		}
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

func IsPasswordHash(value string) bool {
	return passwordHashPattern.MatchString(value)
}

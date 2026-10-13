package domain

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
)

var ErrHomeFilesConflict = errors.New("initial home files changed since review")
var ErrHomeFilesDurability = errors.New("initial home file storage durability is unconfirmed")

type HomeFilesDraft struct {
	Profile    HomeFilesProfile `json:"profile"`
	CaptureIDs []string         `json:"captureIds,omitempty"`
	// A draft opened by the TUI retains its original manifest identity.
	BaseFingerprint string `json:"baseFingerprint,omitempty"`
}

type HomeFilesSnapshot struct {
	Revision           string `json:"revision"`
	SourceFingerprint  string `json:"sourceFingerprint"`
	PinFingerprint     string `json:"pinFingerprint"`
	BaseFingerprint    string `json:"baseFingerprint"`
	BaseExists         bool   `json:"baseExists"`
	PayloadFingerprint string `json:"payloadFingerprint"`
}

type HomeFilesEntry struct {
	Destination string `json:"destination"`
	SHA256      string `json:"sha256,omitempty"`
	Size        int64  `json:"size"`
	Executable  bool   `json:"executable"`
	Directory   bool   `json:"directory,omitempty"`
}

type HomeFilesChange struct {
	Destination      string `json:"destination"`
	Kind             string `json:"kind"`
	Directory        bool   `json:"directory,omitempty"`
	BeforeDirectory  bool   `json:"beforeDirectory,omitempty"`
	AfterDirectory   bool   `json:"afterDirectory,omitempty"`
	BeforeSize       int64  `json:"beforeSize"`
	AfterSize        int64  `json:"afterSize"`
	BeforeExecutable bool   `json:"beforeExecutable"`
	AfterExecutable  bool   `json:"afterExecutable"`
}

func ValidateHomeFilesInspection(i HomeFilesInspection, p HomeFilesProfile) error {
	s := i.Snapshot
	if !workspaceRevisionPattern.MatchString(s.Revision) || !workspaceFingerprintPattern.MatchString(s.SourceFingerprint) || !workspaceFingerprintPattern.MatchString(s.PinFingerprint) || !workspaceFingerprintPattern.MatchString(s.BaseFingerprint) || !workspaceFingerprintPattern.MatchString(s.PayloadFingerprint) || !userNamePattern.MatchString(i.StudentUser) || len(i.Targets) == 0 || i.Targets[0].Role != "controller" {
		return errors.New("incomplete initial-file inspection identity")
	}
	names := map[string]bool{}
	for index, t := range i.Targets {
		if !workspaceHostPattern.MatchString(t.Name) || names[t.Name] || (index > 0 && t.Role != "client") {
			return errors.New("invalid initial-file destinations")
		}
		names[t.Name] = true
	}
	validate := func(profile HomeFilesProfile, entries []HomeFilesEntry) error {
		if _, err := MarshalHomeFilesProfile(profile); err != nil {
			return err
		}
		wanted := map[string]HomeFile{}
		dirs := map[string]bool{}
		for _, f := range profile.Files {
			wanted[f.Destination] = f
		}
		for _, d := range profile.Directories {
			dirs[d] = true
		}
		if len(entries) != len(wanted)+len(dirs) {
			return errors.New("initial-file inspection omitted entries")
		}
		seen := map[string]bool{}
		total := int64(0)
		for _, e := range entries {
			if seen[e.Destination] {
				return errors.New("duplicate inspected destination")
			}
			seen[e.Destination] = true
			if e.Directory {
				if !dirs[e.Destination] || e.Size != 0 || e.SHA256 != "" || e.Executable {
					return errors.New("unexpected inspected directory")
				}
				continue
			}
			f, ok := wanted[e.Destination]
			if !ok || e.Executable != f.Executable || e.Size < 0 || e.Size > HomeFilesMaxFileBytes || len(e.SHA256) != 64 {
				return errors.New("unexpected inspected file")
			}
			if f.Text != nil && (e.Size != int64(len(*f.Text)) || e.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(*f.Text)))) {
				return errors.New("inspected text differs from declaration")
			}
			if f.Source != nil && e.SHA256 != strings.TrimPrefix(*f.Source, "imports/") {
				return errors.New("inspected source differs from declaration")
			}
			total += e.Size
			if total > HomeFilesMaxTotalBytes {
				return errors.New("inspected files exceed size limit")
			}
		}
		return nil
	}
	if err := validate(p, i.Entries); err != nil {
		return err
	}
	return validate(i.Base, i.BaseEntries)
}

type HomeFilesInspection struct {
	Snapshot    HomeFilesSnapshot `json:"snapshot"`
	Base        HomeFilesProfile  `json:"base"`
	Entries     []HomeFilesEntry  `json:"entries"`
	BaseEntries []HomeFilesEntry  `json:"baseEntries"`
	StudentUser string            `json:"studentUser"`
	Targets     []WorkspaceTarget `json:"targets"`
}

type HomeFilesCaptureReport struct {
	SchemaVersion int               `json:"schemaVersion"`
	State         string            `json:"state"`
	CaptureID     string            `json:"captureId,omitempty"`
	Destination   string            `json:"destination"`
	Files         []HomeFile        `json:"files"`
	Directories   []string          `json:"directories"`
	Entries       []HomeFilesEntry  `json:"entries"`
	Issues        []ValidationIssue `json:"issues"`
	Message       string            `json:"message"`
}

func (r HomeFilesCaptureReport) HasErrors() bool { return r.State != "captured" || len(r.Issues) != 0 }

type HomeFilesPlanReport struct {
	SchemaVersion int                `json:"schemaVersion"`
	Operation     string             `json:"operation"`
	State         string             `json:"state"`
	Repository    string             `json:"repository"`
	Candidate     *HomeFilesDraft    `json:"candidate,omitempty"`
	Snapshot      *HomeFilesSnapshot `json:"snapshot,omitempty"`
	Entries       []HomeFilesEntry   `json:"entries"`
	Changes       []HomeFilesChange  `json:"changes"`
	StudentUser   string             `json:"studentUser"`
	Targets       []WorkspaceTarget  `json:"targets"`
	ReviewToken   string             `json:"reviewToken,omitempty"`
	Confirmation  string             `json:"confirmation,omitempty"`
	Issues        []ValidationIssue  `json:"issues"`
	Message       string             `json:"message"`
}

func (r HomeFilesPlanReport) HasErrors() bool {
	return (r.State != "ready" && r.State != "unchanged") || len(r.Issues) != 0
}

type HomeFilesApplyReport struct {
	SchemaVersion    int               `json:"schemaVersion"`
	Operation        string            `json:"operation"`
	State            string            `json:"state"`
	Repository       string            `json:"repository"`
	Recorded         bool              `json:"recorded,omitempty"`
	Revision         string            `json:"revision,omitempty"`
	RecoveryRequired bool              `json:"recoveryRequired,omitempty"`
	StudentUser      string            `json:"studentUser,omitempty"`
	Targets          []WorkspaceTarget `json:"targets"`
	Issues           []ValidationIssue `json:"issues"`
	Message          string            `json:"message"`
}

func (r HomeFilesApplyReport) HasErrors() bool {
	return (r.State != "saved" && r.State != "unchanged" && r.State != "cancelled") || len(r.Issues) != 0
}

package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const TemplateResetSchemaVersion = 1

// TemplateFile is private proposal material: never render these bytes or
// include them in JSON reports.
// Its mode is a Git tree mode, including confined template discovery symlinks.
type TemplateFile struct {
	Mode string
	Data []byte
}

type TemplateResetChange struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

type TemplateResetCatalog struct {
	Repository       string                `json:"repository"`
	UpstreamRevision string                `json:"upstreamRevision"`
	Catalog          SoftwarePresetCatalog `json:"catalog"`
	Fingerprint      string                `json:"fingerprint"`
	Error            string                `json:"error,omitempty"`
}

// TemplateResetProposal binds the complete source and candidate, including
// private settings bytes. Only its digest and typed summary may be displayed.
type TemplateResetProposal struct {
	Revision         string
	Branch           string
	UpstreamRevision string
	SourceIdentity   string
	UntrackedPaths   []string
	Original         map[string]TemplateFile
	Candidate        map[string]TemplateFile
}

type TemplateResetPlan struct {
	SchemaVersion    int                   `json:"schemaVersion"`
	State            string                `json:"state"`
	Repository       string                `json:"repository"`
	Revision         string                `json:"revision"`
	UpstreamRevision string                `json:"upstreamRevision"`
	Preset           SoftwarePreset        `json:"preset"`
	PreviousSoftware []SoftwareDeclaration `json:"previousSoftware"`
	Changes          []TemplateResetChange `json:"changes"`
	Preserved        []string              `json:"preserved"`
	Checks           []UpdateCheck         `json:"checks"`
	ReviewToken      string                `json:"reviewToken,omitempty"`
	Confirmation     string                `json:"confirmation,omitempty"`
	Message          string                `json:"message"`
	Proposal         TemplateResetProposal `json:"-"`
}

func (p TemplateResetPlan) HasErrors() bool { return p.State != "ready" }

type TemplateResetResult struct {
	State            string `json:"state"`
	Revision         string `json:"revision,omitempty"`
	BackupRef        string `json:"backupRef,omitempty"`
	RecoveryRequired bool   `json:"recoveryRequired"`
	Message          string `json:"message"`
}

var templateGitRevision = regexp.MustCompile(`^[0-9a-f]{40}$`)

func TemplateResetPreserves(name string) bool {
	return name == "nixorium-recovery.age" || name == "lab-settings.json" || name == "flake.lock" ||
		strings.HasPrefix(name, "keys/") || path.Base(name) == ".gitignore"
}

func ValidTemplatePath(name string) bool {
	if name == "" || len(name) > 1024 || path.IsAbs(name) || path.Clean(name) != name || name == "." ||
		!utf8.ValidString(name) || strings.Contains(name, "\\") || strings.HasPrefix(name, "../") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".git" || part == ".." || part == "" || strings.ContainsAny(part, "\x1b") {
			return false
		}
	}
	return true
}

func ValidateTemplateFiles(files map[string]TemplateFile) error {
	if len(files) == 0 || len(files) > 2000 {
		return errors.New("template tree must contain between 1 and 2000 files")
	}
	total := 0
	for name, file := range files {
		if !ValidTemplatePath(name) || (file.Mode != "100644" && file.Mode != "100755" && file.Mode != "120000") {
			return errors.New("unsupported template file path or mode")
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, exists := files[parent]; exists {
				return errors.New("template file traverses another file or symlink")
			}
		}
		total += len(file.Data)
		if len(file.Data) > 16*1024*1024 || total > 128*1024*1024 {
			return errors.New("template tree exceeds the size limit")
		}
		if file.Mode == "120000" {
			target := string(file.Data)
			resolved := path.Clean(path.Join(path.Dir(name), target))
			if target == "" || path.IsAbs(target) || !ValidTemplatePath(resolved) ||
				!utf8.ValidString(target) || strings.Contains(target, "\\") || strings.IndexFunc(target, unicode.IsControl) >= 0 {
				return errors.New("template symlink must stay inside the deployment")
			}
		}
	}
	return nil
}

func TemplateResetToken(plan TemplateResetPlan) (string, error) {
	p := plan.Proposal
	if !templateGitRevision.MatchString(p.Revision) || !templateGitRevision.MatchString(p.UpstreamRevision) ||
		p.Revision != plan.Revision || p.UpstreamRevision != plan.UpstreamRevision || p.SourceIdentity == "" ||
		!strings.HasPrefix(p.Branch, "refs/heads/") || !softwarePresetIDPattern.MatchString(plan.Preset.ID) ||
		len(p.Original) == 0 || len(p.Candidate) == 0 || len(p.Original) > 2000 || len(p.Candidate) > 2000 {
		return "", errors.New("incomplete template reset proposal")
	}
	for _, files := range []map[string]TemplateFile{p.Original, p.Candidate} {
		if err := ValidateTemplateFiles(files); err != nil {
			return "", err
		}
	}
	for name, original := range p.Original {
		if TemplateResetPreserves(name) {
			candidate, ok := p.Candidate[name]
			if !ok || original.Mode != candidate.Mode || !slices.Equal(original.Data, candidate.Data) {
				return "", errors.New("template reset must preserve settings, lock, keys and ignore rules")
			}
		}
	}
	for _, name := range []string{"lab-settings.json", "flake.lock"} {
		if file, ok := p.Original[name]; !ok || file.Mode == "120000" {
			return "", errors.New("template reset requires tracked settings and lock")
		}
	}
	for _, name := range p.UntrackedPaths {
		if !ValidTemplatePath(name) {
			return "", errors.New("unsupported untracked path")
		}
		for proposed := range p.Candidate {
			if proposed == name || strings.HasPrefix(proposed, name+"/") || strings.HasPrefix(name, proposed+"/") {
				return "", errors.New("template reset collides with an untracked or ignored path")
			}
		}
	}
	// Do not hash the public JSON projection: it intentionally omits the source
	// bytes that must authorize this destructive, multi-file operation.
	data, err := json.Marshal(struct {
		Repository       string
		Preset           SoftwarePreset
		Proposal         TemplateResetProposal
		Changes          []TemplateResetChange
		Preserved        []string
		PreviousSoftware []SoftwareDeclaration
		Checks           []UpdateCheck
	}{plan.Repository, plan.Preset, p, plan.Changes, plan.Preserved, plan.PreviousSoftware, plan.Checks})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

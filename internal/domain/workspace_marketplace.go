package domain

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// WorkspaceMarketplaceCandidate describes one Marketplace version that was
// downloaded into the controller store and inspected. It is a proposal for
// the profile draft, not a selection, save or installation.
type WorkspaceMarketplaceCandidate struct {
	Entry         WorkspaceMarketplaceExtension `json:"entry"`
	DisplayName   string                        `json:"displayName"`
	Description   string                        `json:"description"`
	Engine        string                        `json:"engine"`
	EditorVersion string                        `json:"editorVersion"`
	Dependencies  []string                      `json:"dependencies"`
	Pack          []string                      `json:"pack"`
	// Native reports executables or native modules. Marketplace binaries are
	// not adapted to NixOS and often fail to start there.
	Native    bool   `json:"native"`
	StorePath string `json:"storePath"`
}

type WorkspaceMarketplaceReport struct {
	SchemaVersion int                            `json:"schemaVersion"`
	Operation     string                         `json:"operation"`
	State         string                         `json:"state"`
	ID            string                         `json:"id"`
	Candidate     *WorkspaceMarketplaceCandidate `json:"candidate,omitempty"`
	Issues        []ValidationIssue              `json:"issues"`
	Message       string                         `json:"message"`
}

func (r WorkspaceMarketplaceReport) HasErrors() bool {
	return len(r.Issues) != 0 || r.State != "ready" || r.Candidate == nil
}

var workspaceMarketplaceID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*\.[a-z0-9][a-z0-9-]*$`)

// NormalizeWorkspaceMarketplaceID accepts publisher.name in any case.
func NormalizeWorkspaceMarketplaceID(id string) (string, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if len(id) > 128 || !workspaceMarketplaceID.MatchString(id) {
		return "", errors.New("enter the Marketplace identifier as publisher.name, for example platformio.platformio-ide")
	}
	return id, nil
}

var workspaceEngineVersion = regexp.MustCompile(`^([0-9]+)\.([0-9]+|x)\.([0-9]+|x)(?:-[0-9A-Za-z.-]+)?$`)

func parseEngineVersion(value string) ([3]int, bool) {
	var result [3]int
	match := workspaceEngineVersion.FindStringSubmatch(value)
	if match == nil {
		return result, false
	}
	for index, part := range match[1:4] {
		if part == "x" {
			continue
		}
		number, err := strconv.Atoi(part)
		if err != nil {
			return result, false
		}
		result[index] = number
	}
	return result, true
}

// VSCodeEngineAccepts applies the editor's own engine rule to a stable
// editor version: "*", "^1.2.3", ">=1.2.3" or "1.2.3". A caret keeps the
// major version; a pre-release suffix of the requirement is ignored.
func VSCodeEngineAccepts(requirement, editor string) (bool, error) {
	requirement = strings.TrimSpace(requirement)
	if requirement == "*" {
		return true, nil
	}
	current, ok := parseEngineVersion(editor)
	if !ok {
		return false, errors.New("the pinned editor version is not a release number")
	}
	caret := strings.HasPrefix(requirement, "^")
	minimum, ok := parseEngineVersion(strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(requirement, "^"), ">=")))
	if !ok {
		return false, errors.New("the extension declares an editor requirement Nixorium cannot interpret: " + requirement)
	}
	if caret && minimum[0] > 0 && current[0] != minimum[0] {
		return false, nil
	}
	for index := range current {
		if current[index] != minimum[index] {
			return current[index] > minimum[index], nil
		}
	}
	return true, nil
}

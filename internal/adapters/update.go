package adapters

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
)

var managedNixoriumInput = regexp.MustCompile(`(?m)^([ \t]*inputs\.nixorium\.url[ \t]*=[ \t]*")([^"\r\n]+)("[ \t]*;[ \t]*)$`)
var githubNixoriumSource = regexp.MustCompile(`^github:([^/]+)/([^/]+)/([^/]+)$`)

type flakeLockDocument struct {
	Root  string `json:"root"`
	Nodes map[string]struct {
		Inputs map[string]any `json:"inputs"`
		Locked struct {
			Rev string `json:"rev"`
		} `json:"locked"`
	} `json:"nodes"`
}

func (Local) InspectUpdateInput(repository string) (domain.UpdateInputSnapshot, error) {
	flake, _, err := readRegularFileNoFollowLimit(filepath.Join(repository, "flake.nix"), 1024*1024)
	if err != nil {
		return domain.UpdateInputSnapshot{}, fmt.Errorf("read flake.nix: %w", err)
	}
	matches := managedNixoriumInput.FindAllSubmatch(flake, -1)
	if len(matches) != 1 {
		return domain.UpdateInputSnapshot{}, errors.New("flake.nix must contain exactly one simple inputs.nixorium.url string assignment")
	}
	sourceURL := string(matches[0][2])
	source := githubNixoriumSource.FindStringSubmatch(sourceURL)
	if source == nil {
		return domain.UpdateInputSnapshot{}, fmt.Errorf("managed update supports a github:OWNER/REPOSITORY/REFERENCE nixorium input; found %q", sourceURL)
	}
	snapshot := domain.UpdateInputSnapshot{
		SourceURL:    sourceURL,
		SourcePrefix: strings.Join(source[1:3], "/"),
		CurrentRef:   source[3],
		FlakeContent: append([]byte(nil), flake...),
	}
	lock, _, lockErr := readRegularFileNoFollowLimit(filepath.Join(repository, "flake.lock"), 4*1024*1024)
	if os.IsNotExist(lockErr) {
		return snapshot, nil
	}
	if lockErr != nil {
		return domain.UpdateInputSnapshot{}, fmt.Errorf("read flake.lock: %w", lockErr)
	}
	snapshot.HasLock = true
	snapshot.LockContent = append([]byte(nil), lock...)
	var document flakeLockDocument
	if err := json.Unmarshal(lock, &document); err != nil {
		return domain.UpdateInputSnapshot{}, fmt.Errorf("decode flake.lock: %w", err)
	}
	root, ok := document.Nodes[document.Root]
	if !ok {
		return domain.UpdateInputSnapshot{}, errors.New("flake.lock root node is missing")
	}
	input, ok := root.Inputs["nixorium"].(string)
	if !ok || input == "" {
		return domain.UpdateInputSnapshot{}, errors.New("flake.lock root nixorium input is not a direct node")
	}
	node, ok := document.Nodes[input]
	if !ok || node.Locked.Rev == "" {
		return domain.UpdateInputSnapshot{}, errors.New("flake.lock nixorium revision is missing")
	}
	snapshot.CurrentRev = node.Locked.Rev
	return snapshot, nil
}

func ProposedUpdateFlake(snapshot domain.UpdateInputSnapshot, target string) ([]byte, error) {
	targetURL := "github:" + snapshot.SourcePrefix + "/" + target
	matches := managedNixoriumInput.FindAllSubmatchIndex(snapshot.FlakeContent, -1)
	if len(matches) != 1 {
		return nil, errors.New("managed nixorium input changed after inspection")
	}
	indices := matches[0]
	result := make([]byte, 0, len(snapshot.FlakeContent)-len(snapshot.SourceURL)+len(targetURL))
	result = append(result, snapshot.FlakeContent[:indices[4]]...)
	result = append(result, targetURL...)
	result = append(result, snapshot.FlakeContent[indices[5]:]...)
	return result, nil
}

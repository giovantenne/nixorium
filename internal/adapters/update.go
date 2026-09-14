package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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

func (Local) PrepareUpdate(ctx context.Context, repository, target string) (domain.UpdateProposal, error) {
	if err := ensurePrivateFilesUntracked(ctx, repository); err != nil {
		return domain.UpdateProposal{}, err
	}
	snapshot, err := (Local{}).InspectUpdateInput(repository)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	proposedFlake, err := ProposedUpdateFlake(snapshot, target)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	targetURL := "github:" + snapshot.SourcePrefix + "/" + target
	lockFile, err := os.CreateTemp("", "nixorium-update-lock-*.json")
	if err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("create candidate lock path: %w", err)
	}
	lockPath := lockFile.Name()
	if err := lockFile.Close(); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("close candidate lock path: %w", err)
	}
	if err := os.Remove(lockPath); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("initialize candidate lock path: %w", err)
	}
	defer os.Remove(lockPath)
	flake, err := deploymentFlakeReference(repository)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	if _, err := runBoundedNix(ctx, 256*1024, "flake", "lock", flake, "--override-input", "nixorium", targetURL, "--output-lock-file", lockPath); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("generate candidate flake.lock: %w", err)
	}
	proposedLock, _, err := readRegularFileNoFollowLimit(lockPath, 4*1024*1024)
	if err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("read candidate flake.lock: %w", err)
	}
	common := []string{"--override-input", "nixorium", targetURL, "--reference-lock-file", lockPath, "--no-write-lock-file"}
	metaOutput, err := runBoundedNix(ctx, 1024*1024, append([]string{"eval", flake + "#labMeta", "--json"}, common...)...)
	if err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("evaluate candidate labMeta: %w", err)
	}
	var meta domain.LabMeta
	if err := json.Unmarshal([]byte(metaOutput), &meta); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("decode candidate labMeta: %w", err)
	}
	if meta.Controller.Name == "" || len(meta.Clients.Hosts) == 0 || meta.Clients.Hosts[0].Name == "" {
		return domain.UpdateProposal{}, errors.New("candidate labMeta does not contain a controller and at least one client")
	}
	statusOutput, err := runBoundedNix(ctx, 1024*1024, append([]string{"eval", flake + "#deploymentStatus", "--json"}, common...)...)
	if err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("evaluate candidate deploymentStatus: %w", err)
	}
	var status domain.DeploymentStatus
	if err := json.Unmarshal([]byte(statusOutput), &status); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("decode candidate deploymentStatus: %w", err)
	}
	if !status.Ready {
		return domain.UpdateProposal{}, fmt.Errorf("candidate deployment is not ready: %s", strings.Join(status.Issues, "; "))
	}
	builds := []struct {
		id        string
		attribute string
	}{
		{"client", "nixosConfigurations." + meta.Clients.Hosts[0].Name + ".config.system.build.toplevel"},
		{"controller", "nixosConfigurations." + meta.Controller.Name + ".config.system.build.toplevel"},
		{"netboot", "nixosConfigurations.netboot.config.system.build.netbootRamdisk"},
		{"pxe-firmware", "pxeFirmware"},
		{"installer-bundle", "installerBundle"},
	}
	checks := []domain.UpdateCheck{
		{ID: "lab-meta", State: "passed", Message: "candidate laboratory metadata evaluated"},
		{ID: "deployment-status", State: "passed", Message: "candidate deployment is ready"},
	}
	for _, build := range builds {
		arguments := append([]string{"build", flake + "#" + build.attribute, "--no-link"}, common...)
		if _, err := runBoundedNix(ctx, 256*1024, arguments...); err != nil {
			return domain.UpdateProposal{}, fmt.Errorf("build candidate %s: %w", build.id, err)
		}
		checks = append(checks, domain.UpdateCheck{ID: build.id, State: "passed", Message: "candidate output built without a result link"})
	}
	diff, err := updateProposalDiff(ctx, snapshot.FlakeContent, proposedFlake, snapshot.LockContent, proposedLock)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	return domain.UpdateProposal{
		FlakeContent: proposedFlake,
		LockContent:  proposedLock,
		Diff:         domain.GitDiff{Scope: "nixorium-update", Content: diff},
		Checks:       checks,
	}, nil
}

func runBoundedNix(ctx context.Context, limit int, arguments ...string) (string, error) {
	command := exec.CommandContext(ctx, "nix", append([]string{"--extra-experimental-features", "nix-command flakes"}, arguments...)...)
	stdout := &boundedCommandBuffer{limit: limit}
	stderr := &boundedCommandBuffer{limit: limit}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.buffer.String())
		if stderr.truncated {
			message += "\n<output truncated>"
		}
		if message == "" {
			message = err.Error()
		}
		return stdout.buffer.String(), fmt.Errorf("nix: %s", sanitizeOperationLog([]byte(message)))
	}
	if stdout.truncated {
		return stdout.buffer.String(), errors.New("nix output exceeds the safety limit")
	}
	return stdout.buffer.String(), nil
}

func updateProposalDiff(ctx context.Context, oldFlake, newFlake, oldLock, newLock []byte) (string, error) {
	temporary, err := os.MkdirTemp("", "nixorium-update-diff-*")
	if err != nil {
		return "", fmt.Errorf("create update diff directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	var combined bytes.Buffer
	for _, file := range []struct {
		name       string
		oldContent []byte
		newContent []byte
	}{{"flake.nix", oldFlake, newFlake}, {"flake.lock", oldLock, newLock}} {
		oldPath := filepath.Join(temporary, "old-"+file.name)
		newPath := filepath.Join(temporary, "new-"+file.name)
		if err := os.WriteFile(oldPath, file.oldContent, 0600); err != nil {
			return "", fmt.Errorf("write old %s diff input: %w", file.name, err)
		}
		if err := os.WriteFile(newPath, file.newContent, 0600); err != nil {
			return "", fmt.Errorf("write new %s diff input: %w", file.name, err)
		}
		command := exec.CommandContext(ctx, "git", "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--unified=3", "--", oldPath, newPath)
		output := &boundedCommandBuffer{limit: maximumGitDiffBytes - combined.Len()}
		stderr := &boundedCommandBuffer{limit: 16 * 1024}
		command.Stdout = output
		command.Stderr = stderr
		runErr := command.Run()
		var exitErr *exec.ExitError
		if runErr != nil && (!errors.As(runErr, &exitErr) || exitErr.ExitCode() != 1) {
			return "", fmt.Errorf("render %s update diff: %s", file.name, sanitizeOperationLog(stderr.buffer.Bytes()))
		}
		if output.truncated {
			return "", errors.New("update diff exceeds the 256 KiB safety limit")
		}
		content := output.buffer.String()
		oldDisplay := filepath.ToSlash(strings.TrimPrefix(oldPath, string(filepath.Separator)))
		newDisplay := filepath.ToSlash(strings.TrimPrefix(newPath, string(filepath.Separator)))
		content = strings.ReplaceAll(content, "a/"+oldDisplay, "a/"+file.name)
		content = strings.ReplaceAll(content, "b/"+newDisplay, "b/"+file.name)
		content = strings.ReplaceAll(content, oldPath, "a/"+file.name)
		content = strings.ReplaceAll(content, newPath, "b/"+file.name)
		combined.WriteString(content)
	}
	if combined.Len() == 0 {
		return "", errors.New("target release does not change flake.nix or flake.lock")
	}
	return sanitizeOperationLog(combined.Bytes()), nil
}

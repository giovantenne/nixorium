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
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
)

// The exact four-space compatibility alternative keeps deployments generated
// by the short-lived nested-inputs template updateable. The canonical template
// uses the explicit top-level inputs.nixorium.url assignment.
var managedNixoriumInput = regexp.MustCompile(`(?m)^((?:[ \t]*inputs\.nixorium| {4}nixorium)\.url[ \t]*=[ \t]*")([^"\r\n]+)("[ \t]*;[ \t]*)$`)
var githubNixoriumSource = regexp.MustCompile(`^github:([^/]+)/([^/]+)/([^/]+)$`)
var safeGitHubSourcePrefix = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}/[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
var remoteGitObjectID = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

const (
	updateDiscoveryTimeout = 15 * time.Second
	updateDiscoveryBytes   = 256 * 1024
)

type flakeLockDocument struct {
	Root  string `json:"root"`
	Nodes map[string]struct {
		Inputs map[string]any `json:"inputs"`
		Locked struct {
			Rev string `json:"rev"`
		} `json:"locked"`
	} `json:"nodes"`
}

type rawFlakeLockDocument struct {
	Root  string                     `json:"root"`
	Nodes map[string]json.RawMessage `json:"nodes"`
}

func (Local) InspectUpdateInput(repository string) (domain.UpdateInputSnapshot, error) {
	flake, flakeMode, err := readRegularFileNoFollowLimit(filepath.Join(repository, "flake.nix"), 1024*1024)
	if err != nil {
		return domain.UpdateInputSnapshot{}, fmt.Errorf("read flake.nix: %w", err)
	}
	matches := managedNixoriumInput.FindAllSubmatch(flake, -1)
	if len(matches) != 1 {
		return domain.UpdateInputSnapshot{}, errors.New("flake.nix must contain exactly one simple nixorium input URL string assignment")
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
		FlakeMode:    flakeMode,
	}
	lock, lockMode, lockErr := readRegularFileNoFollowLimit(filepath.Join(repository, "flake.lock"), 4*1024*1024)
	if os.IsNotExist(lockErr) {
		return snapshot, nil
	}
	if lockErr != nil {
		return domain.UpdateInputSnapshot{}, fmt.Errorf("read flake.lock: %w", lockErr)
	}
	snapshot.HasLock = true
	snapshot.LockContent = append([]byte(nil), lock...)
	snapshot.LockMode = lockMode
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

func (Local) DiscoverUpdateReleases(ctx context.Context, sourcePrefix string) ([]domain.UpdateReleaseRef, error) {
	if !safeGitHubSourcePrefix.MatchString(sourcePrefix) {
		return nil, errors.New("configured GitHub upstream owner/repository is not safe for update discovery")
	}
	discoveryContext, cancel := context.WithTimeout(ctx, updateDiscoveryTimeout)
	defer cancel()
	upstream := "https://github.com/" + sourcePrefix + ".git"
	command := exec.CommandContext(discoveryContext, "git",
		"-c", "credential.helper=",
		"-c", "core.askPass=",
		"ls-remote", "--refs", "--exit-code", upstream, "refs/heads/master", "refs/tags/v*",
	)
	command.Env = updateDiscoveryEnvironment()
	stdout := &boundedCommandBuffer{limit: updateDiscoveryBytes}
	stderr := &boundedCommandBuffer{limit: 16 * 1024}
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		if errors.Is(discoveryContext.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("update discovery exceeded the %s time limit", updateDiscoveryTimeout)
		}
		message := strings.TrimSpace(stderr.buffer.String())
		if stderr.truncated {
			message += "\n<output truncated>"
		}
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("query configured public upstream: git: %s", sanitizeOperationLog([]byte(message)))
	}
	if stdout.truncated {
		return nil, fmt.Errorf("update discovery output exceeds the %d KiB safety limit", updateDiscoveryBytes/1024)
	}
	refs := make([]domain.UpdateReleaseRef, 0)
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(stdout.buffer.String()), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !remoteGitObjectID.MatchString(fields[0]) {
			return nil, errors.New("configured upstream returned a malformed release reference")
		}
		tag := ""
		switch {
		case fields[1] == "refs/heads/master":
			tag = "master"
		case strings.HasPrefix(fields[1], "refs/tags/"):
			tag = strings.TrimPrefix(fields[1], "refs/tags/")
		default:
			return nil, errors.New("configured upstream returned a malformed update reference")
		}
		if seen[tag] {
			continue
		}
		seen[tag] = true
		refs = append(refs, domain.UpdateReleaseRef{Tag: tag, ObjectID: fields[0]})
	}
	return refs, nil
}

func updateDiscoveryEnvironment() []string {
	blocked := map[string]bool{
		"GIT_TERMINAL_PROMPT": true,
		"GIT_ASKPASS":         true,
		"SSH_ASKPASS":         true,
		"GCM_INTERACTIVE":     true,
		"GIT_CONFIG_GLOBAL":   true,
		"GIT_CONFIG_NOSYSTEM": true,
		"GIT_CONFIG":          true,
	}
	environment := make([]string, 0, len(os.Environ())+6)
	for _, item := range os.Environ() {
		name, _, _ := strings.Cut(item, "=")
		if !blocked[name] && !strings.HasPrefix(name, "GIT_CONFIG_") {
			environment = append(environment, item)
		}
	}
	return append(environment,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"SSH_ASKPASS=",
		"GCM_INTERACTIVE=Never",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
	)
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
	return (Local{}).PrepareUpdateWithProgress(ctx, repository, target, nil)
}

func (Local) PrepareUpdateWithProgress(ctx context.Context, repository, target string, progress func(domain.UpdatePlanProgress)) (domain.UpdateProposal, error) {
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
	emitUpdateProgress(progress, domain.UpdatePlanPhaseLock, "Resolving the selected release and generating its candidate lock", 0, 0)
	if _, err := runBoundedNix(ctx, 256*1024, "flake", "lock", flake, "--override-input", "nixorium", targetURL, "--output-lock-file", lockPath); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("generate candidate flake.lock: %w", err)
	}
	proposedLock, _, err := readRegularFileNoFollowLimit(lockPath, 4*1024*1024)
	if err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("read candidate flake.lock: %w", err)
	}
	if err := preserveDeploymentInputNode(snapshot.LockContent, proposedLock, "nixpkgs"); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("validate candidate package-base pin: %w", err)
	}
	common := []string{"--override-input", "nixorium", targetURL, "--reference-lock-file", lockPath, "--no-write-lock-file"}
	emitUpdateProgress(progress, domain.UpdatePlanPhaseEvaluate, "Evaluating candidate laboratory metadata", 0, 0)
	metaOutput, err := runBoundedNix(ctx, 1024*1024, append([]string{"eval", flake + "#labMeta", "--json"}, common...)...)
	if err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("evaluate candidate labMeta: %w", err)
	}
	var meta domain.LabMeta
	if err := json.Unmarshal([]byte(metaOutput), &meta); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("decode candidate labMeta: %w", err)
	}
	emitUpdateProgress(progress, domain.UpdatePlanPhaseEvaluate, "Evaluating candidate deployment readiness", 0, 0)
	statusOutput, err := runBoundedNix(ctx, 1024*1024, append([]string{"eval", flake + "#deploymentStatus", "--json"}, common...)...)
	if err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("evaluate candidate deploymentStatus: %w", err)
	}
	var status domain.DeploymentStatus
	if err := json.Unmarshal([]byte(statusOutput), &status); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("decode candidate deploymentStatus: %w", err)
	}
	builds, checks, err := updateCandidateChecks(meta, status)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	for index, build := range builds {
		emitUpdateProgress(progress, domain.UpdatePlanPhaseBuild, "Building "+updateBuildLabel(build.id), index+1, len(builds))
		arguments := append([]string{"build", flake + "#" + build.attribute, "--no-link"}, common...)
		if _, err := runBoundedNix(ctx, 256*1024, arguments...); err != nil {
			return domain.UpdateProposal{}, fmt.Errorf("build candidate %s: %w", build.id, err)
		}
		checks = append(checks, domain.UpdateCheck{ID: build.id, State: "passed", Message: "candidate output built without a result link"})
	}
	emitUpdateProgress(progress, domain.UpdatePlanPhaseReview, "Preparing the bounded flake.nix and flake.lock review", 0, 0)
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

func emitUpdateProgress(progress func(domain.UpdatePlanProgress), phase domain.UpdatePlanPhase, detail string, current, total int) {
	if progress == nil {
		return
	}
	progress(domain.UpdatePlanProgress{Phase: phase, Detail: detail, Current: current, Total: total})
}

func updateBuildLabel(id string) string {
	switch id {
	case "client":
		return "the representative client"
	case "controller":
		return "the controller"
	case "netboot":
		return "the netboot environment"
	case "pxe-firmware":
		return "the PXE firmware"
	case "installer-bundle":
		return "the offline installer bundle"
	default:
		return "candidate output " + id
	}
}

func preserveDeploymentInputNode(before, after []byte, inputName string) error {
	beforeNode, present, err := directRootInputNode(before, inputName)
	if err != nil {
		return fmt.Errorf("inspect current %s input: %w", inputName, err)
	}
	if !present {
		return nil
	}
	afterNode, afterPresent, err := directRootInputNode(after, inputName)
	if err != nil {
		return fmt.Errorf("inspect candidate %s input: %w", inputName, err)
	}
	if !afterPresent || !bytes.Equal(beforeNode, afterNode) {
		return fmt.Errorf("framework update changed the deployment-owned %s lock node", inputName)
	}
	return nil
}

func directRootInputNode(content []byte, inputName string) ([]byte, bool, error) {
	if len(content) == 0 {
		return nil, false, nil
	}
	var document rawFlakeLockDocument
	if err := json.Unmarshal(content, &document); err != nil {
		return nil, false, fmt.Errorf("decode flake.lock: %w", err)
	}
	rootContent, ok := document.Nodes[document.Root]
	if !ok {
		return nil, false, errors.New("flake.lock root node is missing")
	}
	var root struct {
		Inputs map[string]json.RawMessage `json:"inputs"`
	}
	if err := json.Unmarshal(rootContent, &root); err != nil {
		return nil, false, fmt.Errorf("decode flake.lock root node: %w", err)
	}
	input, ok := root.Inputs[inputName]
	if !ok {
		return nil, false, nil
	}
	var nodeName string
	if err := json.Unmarshal(input, &nodeName); err != nil || nodeName == "" {
		return nil, false, fmt.Errorf("root %s input is not a direct node", inputName)
	}
	nodeContent, ok := document.Nodes[nodeName]
	if !ok {
		return nil, false, fmt.Errorf("%s lock node is missing", inputName)
	}
	var node any
	if err := json.Unmarshal(nodeContent, &node); err != nil {
		return nil, false, fmt.Errorf("decode %s lock node: %w", inputName, err)
	}
	canonical, err := json.Marshal(node)
	if err != nil {
		return nil, false, fmt.Errorf("normalize %s lock node: %w", inputName, err)
	}
	return canonical, true, nil
}

type updateCandidateBuild struct {
	id        string
	attribute string
}

func updateCandidateChecks(meta domain.LabMeta, status domain.DeploymentStatus) ([]updateCandidateBuild, []domain.UpdateCheck, error) {
	if meta.Controller.Name == "" {
		return nil, nil, errors.New("candidate labMeta does not contain a controller")
	}
	switch meta.DeploymentMode {
	case "controller":
		if meta.Clients.Count != 0 || len(meta.Clients.Hosts) != 0 {
			return nil, nil, errors.New("candidate controller mode contains client inventory")
		}
		if status.Controller == nil {
			return nil, nil, errors.New("candidate controller mode does not advertise controller readiness")
		}
		if !status.Controller.Ready {
			detail := strings.Join(status.Controller.Issues, "; ")
			if detail == "" {
				detail = "controller configuration is not ready"
			}
			return nil, nil, fmt.Errorf("candidate controller is not ready: %s", detail)
		}
		return []updateCandidateBuild{
				{"controller", "nixosConfigurations." + meta.Controller.Name + ".config.system.build.toplevel"},
			}, []domain.UpdateCheck{
				{ID: "lab-meta", State: "passed", Message: "candidate controller metadata evaluated"},
				{ID: "deployment-status", State: "passed", Message: "candidate controller is ready"},
			}, nil
	case "", "laboratory":
		if len(meta.Clients.Hosts) == 0 || meta.Clients.Hosts[0].Name == "" {
			return nil, nil, errors.New("candidate labMeta does not contain at least one client")
		}
		if !status.Ready {
			detail := strings.Join(status.Issues, "; ")
			if detail == "" {
				detail = "laboratory configuration is not ready"
			}
			return nil, nil, fmt.Errorf("candidate deployment is not ready: %s", detail)
		}
		return []updateCandidateBuild{
				{"client", "nixosConfigurations." + meta.Clients.Hosts[0].Name + ".config.system.build.toplevel"},
				{"controller", "nixosConfigurations." + meta.Controller.Name + ".config.system.build.toplevel"},
				{"netboot", "nixosConfigurations.netboot.config.system.build.netbootRamdisk"},
				{"pxe-firmware", "pxeFirmware"},
				{"installer-bundle", "installerBundle"},
			}, []domain.UpdateCheck{
				{ID: "lab-meta", State: "passed", Message: "candidate laboratory metadata evaluated"},
				{ID: "deployment-status", State: "passed", Message: "candidate deployment is ready"},
			}, nil
	default:
		return nil, nil, fmt.Errorf("candidate labMeta has unsupported deployment mode %q", meta.DeploymentMode)
	}
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

func (local Local) ApplyPreparedUpdate(ctx context.Context, repository, expectedRevision string, expected domain.UpdateInputSnapshot, proposal domain.UpdateProposal) (bool, error) {
	rootDescriptor, err := syscall.Open(repository, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return false, fmt.Errorf("open deployment root: %w", err)
	}
	root := os.NewFile(uintptr(rootDescriptor), repository)
	defer root.Close()
	if err := syscall.Flock(rootDescriptor, syscall.LOCK_EX); err != nil {
		return false, fmt.Errorf("lock deployment root: %w", err)
	}
	defer syscall.Flock(rootDescriptor, syscall.LOCK_UN)
	revision, err := local.GitRevision(ctx, repository)
	if err != nil || revision != expectedRevision {
		return false, errors.New("deployment HEAD changed after update review")
	}
	state, err := local.GitState(ctx, repository)
	if err != nil || state.Dirty {
		return false, errors.New("deployment worktree changed after update review")
	}
	current, err := local.InspectUpdateInput(repository)
	if err != nil {
		return false, fmt.Errorf("reinspect update input: %w", err)
	}
	if current.HasLock != expected.HasLock || current.FlakeMode != expected.FlakeMode || current.LockMode != expected.LockMode || !bytes.Equal(current.FlakeContent, expected.FlakeContent) || !bytes.Equal(current.LockContent, expected.LockContent) {
		return false, errors.New("flake.nix or flake.lock changed after update review")
	}
	if len(proposal.FlakeContent) == 0 || len(proposal.LockContent) == 0 {
		return false, errors.New("validated update proposal is incomplete")
	}
	lockMode := os.FileMode(expected.LockMode)
	if !expected.HasLock {
		lockMode = 0644
	}
	lockTemp, err := writeUpdateTemporary(repository, "flake.lock", proposal.LockContent, lockMode)
	if err != nil {
		return false, err
	}
	defer os.Remove(lockTemp)
	flakeTemp, err := writeUpdateTemporary(repository, "flake.nix", proposal.FlakeContent, os.FileMode(expected.FlakeMode))
	if err != nil {
		return false, err
	}
	defer os.Remove(flakeTemp)
	lockPath := filepath.Join(repository, "flake.lock")
	flakePath := filepath.Join(repository, "flake.nix")
	if err := os.Rename(lockTemp, lockPath); err != nil {
		return false, fmt.Errorf("replace flake.lock: %w", err)
	}
	if err := os.Rename(flakeTemp, flakePath); err != nil {
		rollbackErr := restoreUpdateLock(repository, expected)
		if rollbackErr != nil {
			return true, fmt.Errorf("replace flake.nix: %w; restoring flake.lock also failed: %v", err, rollbackErr)
		}
		return false, fmt.Errorf("replace flake.nix: %w; original flake.lock was restored", err)
	}
	if err := root.Sync(); err != nil {
		return true, fmt.Errorf("sync deployment root after update: %w", err)
	}
	return false, nil
}

func writeUpdateTemporary(repository, name string, content []byte, mode os.FileMode) (string, error) {
	file, err := os.CreateTemp(repository, "."+name+".nixorium-update-*")
	if err != nil {
		return "", fmt.Errorf("create %s update draft: %w", name, err)
	}
	path := file.Name()
	failed := true
	defer func() {
		if failed {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(mode.Perm()); err != nil {
		file.Close()
		return "", fmt.Errorf("set %s update draft mode: %w", name, err)
	}
	if _, err := file.Write(content); err != nil {
		file.Close()
		return "", fmt.Errorf("write %s update draft: %w", name, err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return "", fmt.Errorf("sync %s update draft: %w", name, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close %s update draft: %w", name, err)
	}
	failed = false
	return path, nil
}

func restoreUpdateLock(repository string, expected domain.UpdateInputSnapshot) error {
	path := filepath.Join(repository, "flake.lock")
	if !expected.HasLock {
		return os.Remove(path)
	}
	temporary, err := writeUpdateTemporary(repository, "flake.lock.rollback", expected.LockContent, os.FileMode(expected.LockMode))
	if err != nil {
		return err
	}
	defer os.Remove(temporary)
	return os.Rename(temporary, path)
}

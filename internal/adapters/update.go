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
	if mismatches, err := flakeLockOriginalMismatches(flake, lock); err == nil {
		for _, mismatch := range mismatches {
			if strings.HasPrefix(mismatch, "nixorium:") {
				snapshot.PinOutdated = mismatch
			}
		}
	}
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
	configureCommandCancellation(command)
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
	// The framework update owns only nixorium. A package-base pin that already
	// disagrees with flake.nix would be relocked here; refuse instead.
	mismatches, err := flakeLockOriginalMismatches(snapshot.FlakeContent, snapshot.LockContent)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	for _, mismatch := range mismatches {
		if strings.HasPrefix(mismatch, "nixpkgs:") {
			return domain.UpdateProposal{}, fmt.Errorf("flake.lock does not match flake.nix (%s); in the deployment folder run `nix flake lock`, save flake.lock in Maintenance → Review Git changes, then update again", mismatch)
		}
	}
	current, err := deploymentFlakeReference(ctx, repository)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	emitUpdateProgress(progress, domain.UpdatePlanPhaseLock, "Resolving the selected release and generating its candidate lock", 0, 0)
	proposedLock, err := lockUpdateCandidate(ctx, repository, proposedFlake, "nixorium", targetURL)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	if err := preserveDeploymentInputNode(snapshot.LockContent, proposedLock, "nixpkgs"); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("validate candidate package-base pin: %w", err)
	}
	return validateUpdateCandidate(ctx, repository, current, snapshot, proposedFlake, proposedLock, progress, false)
}

// lockUpdateCandidate locks the proposed flake.nix against the current
// flake.lock. Only the named input is resolved again (the override fetches
// targetURL even when the reference is a moving branch); Nix keeps every
// other node. Because the proposed flake.nix already declares targetURL, the
// recorded "original" matches the file that will be saved.
func lockUpdateCandidate(ctx context.Context, repository string, proposedFlake []byte, input, targetURL string, extra ...string) ([]byte, error) {
	source, err := candidateFlakeSource(ctx, repository, map[string][]byte{"flake.nix": proposedFlake})
	if err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "nixorium-update-lock-*")
	if err != nil {
		return nil, fmt.Errorf("create candidate lock path: %w", err)
	}
	defer os.RemoveAll(directory)
	lockPath := filepath.Join(directory, "flake.lock")
	arguments := append([]string{"flake", "lock", source, "--override-input", input, targetURL}, extra...)
	arguments = append(arguments, "--output-lock-file", lockPath)
	if _, err := runBoundedNix(ctx, 256*1024, arguments...); err != nil {
		return nil, fmt.Errorf("generate candidate flake.lock: %w", err)
	}
	lock, _, err := readRegularFileNoFollowLimit(lockPath, 4*1024*1024)
	if err != nil {
		return nil, fmt.Errorf("read candidate flake.lock: %w", err)
	}
	return lock, nil
}

// candidateLockMismatch is the refusal when a candidate flake.lock would
// not be accepted by Nix as-is next to the proposed flake.nix.
const candidateLockMismatch = "flake.lock does not match flake.nix in the candidate; the update was refused and nothing was changed"

// validateUpdateCandidate evaluates and builds exactly the proposed pair of
// files. The candidate is an immutable copy with the proposed flake.nix and
// flake.lock, used with --no-update-lock-file and never with
// --no-write-lock-file (which would let Nix silently relock in memory): a
// pair that Nix would want to change is refused before anything else.
func validateUpdateCandidate(ctx context.Context, repository, currentFlake string, snapshot domain.UpdateInputSnapshot, proposedFlake, proposedLock []byte, progress func(domain.UpdatePlanProgress), packageBase bool) (domain.UpdateProposal, error) {
	if mismatches, err := flakeLockOriginalMismatches(proposedFlake, proposedLock); err != nil || len(mismatches) > 0 {
		return domain.UpdateProposal{}, fmt.Errorf("%s: %s", candidateLockMismatch, strings.Join(mismatches, "; "))
	}
	replacements := map[string][]byte{"flake.nix": proposedFlake, "flake.lock": proposedLock}
	flake, err := candidateFlakeSource(ctx, repository, replacements)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	// The target release's changes to the deployment's own files, made by
	// its own migrator: the candidate is evaluated and built with them
	// (ADR 0034). A package-base update keeps the Nixorium revision and has
	// none.
	// An unreadable revision on either side counts as a change: migrations
	// run rather than being skipped silently.
	proposedRevision, currentRevision := lockedNixoriumRevision(proposedLock), lockedNixoriumRevision(snapshot.LockContent)
	revisionChanges := proposedRevision == "" || currentRevision == "" || proposedRevision != currentRevision
	if revisionChanges {
		emitUpdateProgress(progress, domain.UpdatePlanPhaseEvaluate, "Running the target release's changes to deployment files", 0, 0)
	}
	migrations, migratedFiles, migrationReport, migrator, err := planDeploymentMigrations(ctx, repository, flake, revisionChanges)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	if len(migratedFiles) > 0 {
		for _, file := range migratedFiles {
			replacements[file.Path] = file.After
			if file.Delete {
				replacements[file.Path] = nil
			}
		}
		if flake, err = candidateFlakeSource(ctx, repository, replacements); err != nil {
			return domain.UpdateProposal{}, err
		}
	}
	emitUpdateProgress(progress, domain.UpdatePlanPhaseEvaluate, "Checking that the candidate flake.lock matches flake.nix", 0, 0)
	if _, err := runBoundedNix(ctx, 64*1024, "flake", "lock", "--no-update-lock-file", flake); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("%s: %w", candidateLockMismatch, err)
	}
	common := []string{"--no-update-lock-file"}
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
	emitUpdateProgress(progress, domain.UpdatePlanPhaseEvaluate, "Comparing pinned student workspace dependencies", 0, 0)
	workspace, err := inspectWorkspaceUpdate(ctx, currentFlake, flake, common)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	if packageBase {
		output, err := runBoundedNix(ctx, 256*1024, append([]string{"eval", flake + "#nixoriumUpdateTargets", "--json"}, common...)...)
		if err != nil {
			return domain.UpdateProposal{}, fmt.Errorf("deployment needs the autonomous-update outputs; see the deployment migration guide: %w", err)
		}
		var targets []string
		if err := json.Unmarshal([]byte(output), &targets); err != nil || len(targets) == 0 || len(targets) > 256 {
			return domain.UpdateProposal{}, errors.New("invalid deployment update targets")
		}
		allowed := map[string]bool{meta.Controller.Name: true}
		for _, host := range meta.Clients.Hosts {
			allowed[host.Name] = true
		}
		seen := map[string]bool{meta.Controller.Name: true}
		if len(meta.Clients.Hosts) > 0 {
			seen[meta.Clients.Hosts[0].Name] = true
		}
		for _, host := range targets {
			if !allowed[host] {
				return domain.UpdateProposal{}, fmt.Errorf("unknown update target %q", host)
			}
			if !seen[host] {
				builds = append(builds, updateCandidateBuild{"host-" + host, "nixosConfigurations." + host + ".config.system.build.toplevel", false})
				seen[host] = true
			}
		}
		builds = append(builds, updateCandidateBuild{"offline-equivalence", "nixoriumOfflineCheck", false})
	}
	// One Nix invocation groups the complete reviewed output set. Do not mark individual checks passed until all succeed.
	evaluated := 0
	for _, build := range builds {
		if build.evaluateOnly {
			evaluated++
		}
	}
	detail := fmt.Sprintf("Building all %d required outputs together", len(builds))
	if evaluated > 0 {
		detail = fmt.Sprintf("Building %d systems and evaluating %d installation outputs", len(builds)-evaluated, evaluated)
	}
	current := 0
	if len(builds) == 1 {
		detail = "Building " + updateBuildLabel(builds[0].id)
		current = 1
	}
	emitUpdateProgress(progress, domain.UpdatePlanPhaseBuild, detail, current, len(builds))
	arguments := []string{"build", "--no-link"}
	evaluations := []string{"path-info", "--derivation"}
	for _, build := range builds {
		if build.evaluateOnly {
			evaluations = append(evaluations, flake+"#"+build.attribute)
		} else {
			arguments = append(arguments, flake+"#"+build.attribute)
		}
	}
	if len(evaluations) > 2 {
		if _, err := runBoundedNix(ctx, 256*1024, append(evaluations, common...)...); err != nil {
			return domain.UpdateProposal{}, fmt.Errorf("evaluate candidate installation outputs: %w", err)
		}
	}
	arguments = append(arguments, common...)
	if _, err := runBoundedNix(ctx, 256*1024, arguments...); err != nil {
		return domain.UpdateProposal{}, fmt.Errorf("build candidate systems: %w", err)
	}
	for _, build := range builds {
		message := "candidate output built without a result link"
		if build.evaluateOnly {
			message = "candidate output evaluated; it is built when network installation is prepared"
		}
		checks = append(checks, domain.UpdateCheck{ID: build.id, State: "passed", Message: message})
	}
	emitUpdateProgress(progress, domain.UpdatePlanPhaseReview, "Preparing the bounded flake.nix and flake.lock review", 0, 0)
	diff, err := updateProposalDiff(ctx, snapshot.FlakeContent, proposedFlake, snapshot.LockContent, proposedLock, migratedFiles...)
	if err != nil {
		return domain.UpdateProposal{}, err
	}
	return domain.UpdateProposal{
		FlakeContent:     proposedFlake,
		LockContent:      proposedLock,
		NixoriumRevision: proposedRevision,
		Diff:             domain.GitDiff{Scope: "nixorium-update", Content: diff},
		Checks:           checks,
		Workspace:        workspace,
		Migrations:       migrations,
		MigratedFiles:    migratedFiles,
		MigrationReport:  migrationReport,
		Migrator:         migrator,
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
	// evaluateOnly outputs are evaluated and instantiated, not built: the
	// network-installation outputs are built when PXE is prepared, and building
	// the netboot image here doubled the review time.
	evaluateOnly bool
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
				{"controller", "nixosConfigurations." + meta.Controller.Name + ".config.system.build.toplevel", false},
			}, []domain.UpdateCheck{
				{ID: "lab-meta", State: "passed", Message: "candidate controller metadata evaluated"},
				{ID: "deployment-status", State: "passed", Message: "candidate controller is ready"},
			}, nil
	case "laboratory":
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
				{"client", "nixosConfigurations." + meta.Clients.Hosts[0].Name + ".config.system.build.toplevel", false},
				{"controller", "nixosConfigurations." + meta.Controller.Name + ".config.system.build.toplevel", false},
				{"netboot", "nixosConfigurations.netboot.config.system.build.netbootRamdisk", true},
				{"pxe-firmware", "pxeFirmware", true},
				{"installer-bundle", "installerBundle", true},
			}, []domain.UpdateCheck{
				{ID: "lab-meta", State: "passed", Message: "candidate laboratory metadata evaluated"},
				{ID: "deployment-status", State: "passed", Message: "candidate deployment is ready"},
			}, nil
	default:
		return nil, nil, fmt.Errorf("candidate labMeta has unsupported deployment mode %q", meta.DeploymentMode)
	}
}

func runBoundedNix(ctx context.Context, limit int, arguments ...string) (string, error) {
	return runBoundedNixCommand(ctx, limit, exec.CommandContext(ctx, "nix", append([]string{"--extra-experimental-features", "nix-command flakes"}, arguments...)...))
}

// runIsolatedNix runs nix as runBoundedNix does, but in a network namespace of
// its own: an Assistant candidate is Nix code written by the model, and a
// fetch with a fixed hash in it would otherwise make the controller send
// requests while the change is only being reviewed. The file system, the
// Nix daemon and its store stay the same, so the result is too.
func runIsolatedNix(ctx context.Context, limit int, arguments ...string) (string, error) {
	sandbox := os.Getenv("NIXORIUM_NPM_BWRAP")
	if sandbox == "" {
		var err error
		if sandbox, err = exec.LookPath("bwrap"); err != nil {
			return "", errors.New("bubblewrap is needed to evaluate Assistant proposals")
		}
	}
	nix, err := exec.LookPath("nix")
	if err != nil {
		return "", err
	}
	return runBoundedNixCommand(ctx, limit, exec.CommandContext(ctx, sandbox, append([]string{
		"--dev-bind", "/", "/", "--unshare-net", "--die-with-parent", "--new-session",
		"--", nix, "--extra-experimental-features", "nix-command flakes"}, arguments...)...))
}

func runBoundedNixCommand(ctx context.Context, limit int, command *exec.Cmd) (string, error) {
	configureCommandCancellation(command)
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

func updateProposalDiff(ctx context.Context, oldFlake, newFlake, oldLock, newLock []byte, migrated ...domain.MigratedFile) (string, error) {
	temporary, err := os.MkdirTemp("", "nixorium-update-diff-*")
	if err != nil {
		return "", fmt.Errorf("create update diff directory: %w", err)
	}
	defer os.RemoveAll(temporary)
	var combined bytes.Buffer
	type diffFile struct {
		name       string
		oldContent []byte
		newContent []byte
	}
	files := []diffFile{{"flake.nix", oldFlake, newFlake}, {"flake.lock", oldLock, newLock}}
	for _, file := range migrated {
		files = append(files, diffFile{file.Path, file.Before, file.After})
	}
	for index, file := range files {
		oldPath := filepath.Join(temporary, fmt.Sprintf("old-%d", index))
		newPath := filepath.Join(temporary, fmt.Sprintf("new-%d", index))
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
	defer func() { _ = syscall.Flock(rootDescriptor, syscall.LOCK_UN) }()
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
	// The release's changes to the deployment's own files go first: they
	// work with the current release too, and a failure leaves the update
	// input untouched (ADR 0034).
	if partial, err := applyMigratedFiles(repository, proposal.MigratedFiles); err != nil {
		return partial, err
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
		if restoreErr := restoreMigratedFiles(repository, proposal.MigratedFiles); restoreErr != nil {
			return true, fmt.Errorf("replace flake.lock: %w; restoring migrated files also failed: %v", err, restoreErr)
		}
		return false, fmt.Errorf("replace flake.lock: %w", err)
	}
	if err := os.Rename(flakeTemp, flakePath); err != nil {
		rollbackErr := errors.Join(restoreUpdateLock(repository, expected), restoreMigratedFiles(repository, proposal.MigratedFiles))
		if rollbackErr != nil {
			return true, fmt.Errorf("replace flake.nix: %w; restoring flake.lock and migrated files also failed: %v", err, rollbackErr)
		}
		return false, fmt.Errorf("replace flake.nix: %w; original flake.lock and files were restored", err)
	}
	if err := root.Sync(); err != nil {
		return true, fmt.Errorf("sync deployment root after update: %w", err)
	}
	return false, nil
}

// applyMigratedFiles writes the reviewed migrations, only if every file is
// still exactly as reviewed; on a failure it restores those already written.
// partial reports that the restore failed too.
func applyMigratedFiles(repository string, files []domain.MigratedFile) (partial bool, err error) {
	for _, file := range files {
		if !validMigrationFile(file.Path) && file.Path != domain.DeploymentMigrationsFile {
			return false, fmt.Errorf("deployment migration names a file it may not change: %s", file.Path)
		}
		current, exists, err := readMigratedFile(repository, file.Path)
		switch {
		case err != nil:
			return false, fmt.Errorf("%s changed after update review", file.Path)
		case file.Create && exists:
			return false, fmt.Errorf("%s appeared after update review", file.Path)
		case !file.Create && (!exists || !bytes.Equal(current, file.Before)):
			return false, fmt.Errorf("%s changed after update review", file.Path)
		}
	}
	for index, file := range files {
		if err := writeMigratedState(repository, file, false); err != nil {
			restoreErr := restoreMigratedFiles(repository, files[:index])
			return restoreErr != nil, errors.Join(fmt.Errorf("write deployment migration: %w", err), restoreErr)
		}
	}
	return false, nil
}

// MigratedFilesSaved reports whether every migrated file on disk is exactly
// the reviewed result (present with its new content, or removed).
func (Local) MigratedFilesSaved(repository string, files []domain.MigratedFile) bool {
	for _, file := range files {
		current, exists, err := readMigratedFile(repository, file.Path)
		if err != nil || exists == file.Delete || (!file.Delete && !bytes.Equal(current, file.After)) {
			return false
		}
	}
	return true
}

// restoreMigratedFiles puts back the reviewed originals of migrated files.
// Files are restored in reverse order, undoing later writes first.
func restoreMigratedFiles(repository string, files []domain.MigratedFile) error {
	var problems []error
	for index := len(files) - 1; index >= 0; index-- {
		file := files[index]
		if err := writeMigratedState(repository, file, true); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", file.Path, err))
		}
	}
	return errors.Join(problems...)
}

// writeMigratedState writes a file's state after (or, restoring, before)
// its migrations: content, absence, and mode.
func writeMigratedState(repository string, file domain.MigratedFile, restoring bool) error {
	path := filepath.Join(repository, file.Path)
	absent, content := file.Delete, file.After
	if restoring {
		absent, content = file.Create, file.Before
	}
	if absent {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := writeUpdateTemporary(directory, filepath.Base(file.Path), content, os.FileMode(file.Mode))
	if err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func readMigratedFile(repository, name string) ([]byte, bool, error) {
	path := filepath.Join(repository, name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, true, errors.New("not a regular file")
	}
	content, err := os.ReadFile(path)
	return content, true, err
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

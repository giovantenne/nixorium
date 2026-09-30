package adapters

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

const resetMaximumBytes = 128 * 1024 * 1024
const resetPendingName = "nixorium-template-reset.json"

// TemplateReset is separate from ordinary additive software updates. Its only
// source is the deployment's locked framework; it never discovers a new ref.
type TemplateReset struct{}

func resetGit(ctx context.Context, repository string, input []byte, limit int, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repository,
		"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false",
		"-c", "core.fsync=committed,index,reference", "-c", "commit.gpgSign=false",
		"-c", "core.autocrlf=false", "-c", "core.fileMode=true", "-c", "core.symlinks=true",
		"-c", "core.sparseCheckout=false", "-c", "core.attributesFile=/dev/null"}, args...)...)
	configureCommandCancellation(command)
	command.Env = append(workspaceEnvironment(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_AUTHOR_NAME=Nixorium", "GIT_AUTHOR_EMAIL=nixorium@localhost",
		"GIT_COMMITTER_NAME=Nixorium", "GIT_COMMITTER_EMAIL=nixorium@localhost")
	command.Stdin = bytes.NewReader(input)
	out := &boundedCommandBuffer{limit: limit}
	command.Stdout = out
	command.Stderr = &boundedCommandBuffer{limit: 16 * 1024}
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("Git %s failed; inspect the local repository before retrying: %w", args[0], err)
	}
	if out.truncated {
		return "", errors.New("Git output exceeds the template reset safety limit")
	}
	return out.buffer.String(), nil
}

func resetNix(ctx context.Context, repository, expression string) ([]byte, error) {
	flake, err := deploymentFlakeReference(repository)
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, "nix", "--extra-experimental-features", "nix-command flakes", "eval",
		"--impure", "--json", "--no-write-lock-file", "--no-update-lock-file",
		"--option", "allow-import-from-derivation", "false", "--option", "accept-flake-config", "false",
		"--expr", `let f = builtins.getFlake (builtins.getEnv "NIXORIUM_DEPLOYMENT_FLAKE"); in `+expression)
	configureCommandCancellation(command)
	command.Env = append(workspaceEnvironment(), "NIXORIUM_DEPLOYMENT_FLAKE="+flake)
	out := &boundedCommandBuffer{limit: 2 * 1024 * 1024}
	command.Stdout, command.Stderr = out, &boundedCommandBuffer{limit: 64 * 1024}
	if err := command.Run(); err != nil {
		return nil, fmt.Errorf("template reset evaluation failed; inspect the pinned template and local deployment privately: %w", err)
	}
	if out.truncated {
		return nil, errors.New("template reset metadata exceeds the safety limit")
	}
	return out.buffer.Bytes(), nil
}

func resetFileDigest(value any) string {
	data, _ := json.Marshal(value)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

func inspectResetRepository(ctx context.Context, root *os.File) (domain.TemplateResetProposal, error) {
	p := domain.TemplateResetProposal{Original: map[string]domain.TemplateFile{}}
	identity, err := workspaceRootIdentity(root)
	if err != nil || identity.Uid != uint32(os.Geteuid()) || identity.Mode&0022 != 0 {
		return p, errors.New("template reset requires an owned, non-shared deployment directory")
	}
	gitDir, err := unix.Openat2(int(root.Fd()), ".git", &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
	if err != nil {
		return p, errors.New("template reset requires an ordinary Git repository, not a linked worktree")
	}
	defer unix.Close(gitDir)
	var gitInfo unix.Stat_t
	if unix.Fstat(gitDir, &gitInfo) != nil || gitInfo.Uid != uint32(os.Geteuid()) || gitInfo.Mode&0022 != 0 {
		return p, errors.New("unsafe Git directory ownership or permissions")
	}
	for _, name := range []string{resetPendingName, "index.lock", "HEAD.lock", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"} {
		var info unix.Stat_t
		if err := unix.Fstatat(gitDir, name, &info, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
			return p, errors.New("an unfinished Git or template reset operation blocks this reset; preserve recovery evidence")
		}
	}
	var attributes unix.Stat_t
	if err := unix.Fstatat(gitDir, "info/attributes", &attributes, unix.AT_SYMLINK_NOFOLLOW); err == nil || !errors.Is(err, unix.ENOENT) {
		return p, errors.New("repository-specific Git attributes require manual review before template reset")
	}
	top, err := resetGit(ctx, root.Name(), nil, 4096, "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(top) != root.Name() {
		return p, errors.New("template reset must run at the actual deployment repository root")
	}
	branch, err := resetGit(ctx, root.Name(), nil, 4096, "symbolic-ref", "HEAD")
	if err != nil {
		return p, errors.New("template reset requires an attached local branch")
	}
	p.Branch = strings.TrimSpace(branch)
	revision, err := resetGit(ctx, root.Name(), nil, 256, "rev-parse", "HEAD")
	if err != nil || !fullGitObjectIDPattern.MatchString(strings.TrimSpace(revision)) {
		return p, errors.New("template reset requires a committed deployment")
	}
	p.Revision = strings.TrimSpace(revision)
	manifest, err := resetGit(ctx, root.Name(), nil, 1024*1024, "ls-files", "--stage", "-z")
	if err != nil {
		return p, err
	}
	paths := []string{}
	blobs := map[string]string{}
	for _, entry := range strings.Split(strings.TrimSuffix(manifest, "\x00"), "\x00") {
		header, name, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || fields[2] != "0" || !domain.ValidTemplatePath(name) ||
			(fields[0] != "100644" && fields[0] != "100755" && fields[0] != "120000") || isPrivateDeploymentPath(name) || filepath.Base(name) == ".gitattributes" {
			return p, errors.New("unsupported tracked path, private key, submodule or conflicted index")
		}
		paths = append(paths, name)
		p.Original[name] = domain.TemplateFile{Mode: fields[0]}
		blobs[name] = fields[1]
	}
	if len(paths) == 0 || len(paths) > 2000 {
		return p, errors.New("template reset supports at most 2000 tracked files")
	}
	if err := resetCheckCandidateDestinations(root, p.Original); err != nil {
		return p, err
	}
	if err := rejectGitAttributes(ctx, root.Name(), paths); err != nil {
		return p, err
	}
	flags, err := resetGit(ctx, root.Name(), nil, 1024*1024, "ls-files", "-v", "-z")
	if err != nil {
		return p, err
	}
	for _, entry := range strings.Split(strings.TrimSuffix(flags, "\x00"), "\x00") {
		if !strings.HasPrefix(entry, "H ") {
			return p, errors.New("sparse, skip-worktree or assume-unchanged entries require manual review")
		}
	}
	// Clean tracked files make the original commit a complete recoverable backup;
	// untracked and ignored files are neither read nor added to that backup.
	if _, err := resetGit(ctx, root.Name(), nil, 1024, "diff", "--quiet", "--no-ext-diff", "--no-textconv", "HEAD", "--"); err != nil {
		return p, errors.New("commit or resolve tracked local changes before resetting; nothing was stashed or discarded")
	}
	total := 0
	for name, file := range p.Original {
		var info unix.Stat_t
		if file.Mode == "120000" {
			parent, err := resetOpenParent(root, name)
			if err != nil {
				return p, err
			}
			buffer := make([]byte, 4096)
			n, readErr := unix.Readlinkat(int(parent.Fd()), filepath.Base(name), buffer)
			statErr := unix.Fstatat(int(parent.Fd()), filepath.Base(name), &info, unix.AT_SYMLINK_NOFOLLOW)
			parent.Close()
			if readErr != nil || statErr != nil || n >= len(buffer) || info.Uid != uint32(os.Geteuid()) || info.Nlink != 1 {
				return p, errors.New("could not read a confined tracked symlink")
			}
			file.Data = buffer[:n]
		} else {
			file.Data, info, err = readWorkspaceFile(root, name, 16*1024*1024)
			if err != nil || info.Nlink != 1 || info.Uid != uint32(os.Geteuid()) || info.Mode&0022 != 0 {
				return p, fmt.Errorf("unsafe tracked regular file: %s", name)
			}
			if (info.Mode&0111 != 0) != (file.Mode == "100755") {
				return p, errors.New("tracked file mode differs from its Git index entry")
			}
			if privateKeyContent.Match(file.Data) {
				return p, errors.New("tracked private-key or token material blocks template reset")
			}
		}
		// Compare raw bytes, independently of stat caches, Git attributes and
		// index flags. SHA-1 here identifies a Git blob, not an authorization token.
		blob := sha1.New()
		fmt.Fprintf(blob, "blob %d\x00", len(file.Data))
		blob.Write(file.Data)
		if hex.EncodeToString(blob.Sum(nil)) != blobs[name] {
			return p, errors.New("tracked bytes changed or require Git content conversion; commit or review them manually")
		}
		total += len(file.Data)
		if total > resetMaximumBytes {
			return p, errors.New("tracked deployment exceeds the 128 MiB reset limit")
		}
		p.Original[name] = file
	}
	if err := domain.ValidateTemplateFiles(p.Original); err != nil {
		return p, err
	}
	others, err := resetGit(ctx, root.Name(), nil, 1024*1024, "ls-files", "--others", "-z")
	if err != nil {
		return p, err
	}
	for _, name := range strings.Split(others, "\x00") {
		if name != "" {
			p.UntrackedPaths = append(p.UntrackedPaths, strings.TrimSuffix(name, "/"))
		}
	}
	sort.Strings(p.UntrackedPaths)
	p.SourceIdentity = resetFileDigest(struct {
		Device, Inode uint64
		Index         string
	}{uint64(identity.Dev), identity.Ino, manifest})
	return p, nil
}

func resetOpenParent(root *os.File, name string) (*os.File, error) {
	fd, err := unix.Openat2(int(root.Fd()), filepath.Dir(name), &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
	if err != nil {
		return nil, errors.New("tracked path traverses an unsafe directory")
	}
	return os.NewFile(uintptr(fd), filepath.Join(root.Name(), filepath.Dir(name))), nil
}

func lockedResetTemplate(ctx context.Context, repository string) (map[string]domain.TemplateFile, domain.UpdateInputSnapshot, error) {
	snapshot, err := (Local{}).InspectUpdateInput(repository)
	if err != nil {
		return nil, snapshot, errors.New("managed reset requires a simple GitHub framework declaration and a complete lock; custom sources need manual review")
	}
	if _, err := inspectPackageBase(snapshot); err != nil {
		return nil, snapshot, errors.New("managed reset requires a direct locked NixOS channel and framework follows edge; legacy/custom input layouts need manual review")
	}
	upstream, present, err := directRootInputNode(snapshot.LockContent, "nixorium")
	var node struct {
		Locked   struct{ Type, Owner, Repo, Rev, NarHash string }
		Original struct{ Type, Owner, Repo, Ref, Rev string }
	}
	if err != nil || !present || json.Unmarshal(upstream, &node) != nil {
		return nil, snapshot, errors.New("invalid locked framework input")
	}
	hash, hashErr := base64.StdEncoding.DecodeString(strings.TrimPrefix(node.Locked.NarHash, "sha256-"))
	originalMatches := (node.Original.Ref == snapshot.CurrentRef && node.Original.Rev == "") ||
		(node.Original.Ref == "" && node.Original.Rev == snapshot.CurrentRef && node.Original.Rev == snapshot.CurrentRev)
	if node.Locked.Type != "github" || node.Original.Type != "github" ||
		node.Locked.Owner+"/"+node.Locked.Repo != snapshot.SourcePrefix || node.Original.Owner+"/"+node.Original.Repo != snapshot.SourcePrefix ||
		!originalMatches || node.Locked.Rev != snapshot.CurrentRev || !fullGitObjectIDPattern.MatchString(node.Locked.Rev) ||
		!strings.HasPrefix(node.Locked.NarHash, "sha256-") || hashErr != nil || len(hash) != 32 {
		return nil, snapshot, errors.New("framework declaration and locked source/ref/revision/hash do not agree")
	}
	data, err := resetNix(ctx, repository, `{
  revision = f.inputs.nixorium.rev;
  source = toString f.inputs.nixorium.outPath;
  runtime = f.inputs.nixorium.lib.workspaceRuntimeVersion or 0;
  schema = f.inputs.nixorium.lib.workspaceProfileSchemaVersion or 0;
}`)
	if err != nil {
		return nil, snapshot, err
	}
	var source struct {
		Revision, Source string
		Runtime, Schema  int
	}
	if json.Unmarshal(data, &source) != nil || source.Revision != snapshot.CurrentRev || !domain.ValidStorePath(source.Source) || source.Runtime != 2 || source.Schema != 1 {
		return nil, snapshot, errors.New("the locked upstream must provide the supported guided-home template; update Nixorium separately if needed")
	}
	files, err := readResetTemplate(filepath.Join(source.Source, "templates/site"))
	return files, snapshot, err
}

func readResetTemplate(directory string) (map[string]domain.TemplateFile, error) {
	files := map[string]domain.TemplateFile{}
	total := 0
	err := filepath.WalkDir(directory, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(directory, name)
		if err != nil || !domain.ValidTemplatePath(relative) || len(files) >= 2000 {
			return errors.New("unsupported pinned template tree")
		}
		info, err := entry.Info()
		if err != nil || info.Size() > 16*1024*1024 {
			return errors.New("invalid or oversized template file")
		}
		file := domain.TemplateFile{Mode: "100644"}
		if entry.Type()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(name)
			err, file.Mode, file.Data = readErr, "120000", []byte(target)
		} else if info.Mode().IsRegular() {
			file.Data, err = os.ReadFile(name)
			if info.Mode()&0111 != 0 {
				file.Mode = "100755"
			}
		} else {
			return errors.New("template contains a special file")
		}
		if err != nil {
			return err
		}
		total += len(file.Data)
		if total > resetMaximumBytes {
			return errors.New("template exceeds the 128 MiB limit")
		}
		files[relative] = file
		return nil
	})
	if err == nil {
		err = domain.ValidateTemplateFiles(files)
	}
	return files, err
}

func (TemplateReset) TemplateResetCatalog(ctx context.Context, repository string) domain.TemplateResetCatalog {
	report := domain.TemplateResetCatalog{Repository: repository}
	root, err := openWorkspaceRoot(repository, unix.LOCK_SH)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	defer root.Close()
	if _, err := inspectResetRepository(ctx, root); err != nil {
		report.Error = err.Error()
		return report
	}
	files, snapshot, err := lockedResetTemplate(ctx, repository)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	catalog, err := domain.DecodeSoftwarePresetCatalog(files["software-presets.json"].Data)
	if err != nil {
		report.Error = "The pinned template does not provide a valid software profile catalog."
		return report
	}
	report.Catalog, report.UpstreamRevision = catalog, snapshot.CurrentRev
	report.Fingerprint = resetFileDigest(files)
	return report
}

func prepareResetFiles(original, template map[string]domain.TemplateFile, snapshot domain.UpdateInputSnapshot, preset string) (map[string]domain.TemplateFile, domain.SoftwarePreset, error) {
	files := map[string]domain.TemplateFile{}
	for name, file := range template {
		files[name] = file
	}
	for name, file := range original {
		if domain.TemplateResetPreserves(name) {
			files[name] = file
		}
	}
	var selected domain.SoftwarePreset
	catalog, err := domain.DecodeSoftwarePresetCatalog(files["software-presets.json"].Data)
	if err != nil {
		return nil, selected, err
	}
	for _, entry := range catalog.Presets {
		if entry.ID == preset {
			selected = entry
		}
	}
	if selected.ID == "" {
		return nil, selected, errors.New("select a software profile from the pinned upstream template")
	}
	software := domain.LabSoftwareFile{SchemaVersion: domain.SoftwareSchemaVersion}
	for _, name := range selected.Packages {
		software.Packages = append(software.Packages, domain.SoftwareDeclaration{Package: name, Scope: domain.SoftwareScope{Kind: domain.SoftwareScopeShared}})
	}
	softwareJSON, err := domain.MarshalLabSoftware(software)
	if err != nil {
		return nil, selected, err
	}
	files["lab-software.json"] = domain.TemplateFile{Mode: "100644", Data: softwareJSON}
	// A template may tailor the initial home to one software profile, for
	// example its editor extensions. Other profiles start from the common one.
	example, tailored := files["workspace-profile."+selected.ID+".example.json"]
	if !tailored {
		example = files["workspace-profile.example.json"]
	}
	profile, issues := domain.DecodeWorkspaceProfile(example.Data)
	if len(issues) != 0 {
		return nil, selected, errors.New("the pinned template has no supported initial workspace profile")
	}
	if !tailored && slices.Contains(selected.Packages, "vscode") && profile.Desktop != nil && profile.Desktop.Favorites != nil {
		favorites := append([]string{}, (*profile.Desktop.Favorites)...)
		if !slices.Contains(favorites, "code.desktop") {
			favorites = append(favorites, "code.desktop")
		}
		profile.Desktop.Favorites = &favorites
	}
	profileJSON, err := domain.MarshalWorkspaceProfile(profile)
	if err != nil {
		return nil, selected, err
	}
	files[domain.WorkspaceFileName] = domain.TemplateFile{Mode: "100644", Data: profileJSON}
	flake := files["flake.nix"]
	baseMatch := managedPackageBaseInput.FindAllSubmatch(snapshot.FlakeContent, -1)
	if len(baseMatch) != 1 || len(managedPackageBaseInput.FindAllSubmatch(flake.Data, -1)) != 1 ||
		len(managedNixoriumInput.FindAllSubmatch(flake.Data, -1)) != 1 {
		return nil, selected, errors.New("pinned template input declarations need an explicit compatibility review")
	}
	replaceInput := func(content []byte, indices []int, url string) []byte {
		out := append([]byte{}, content[:indices[4]]...)
		out = append(out, url...)
		return append(out, content[indices[5]:]...)
	}
	flake.Data = replaceInput(flake.Data, managedNixoriumInput.FindSubmatchIndex(flake.Data), snapshot.SourceURL)
	flake.Data = replaceInput(flake.Data, managedPackageBaseInput.FindSubmatchIndex(flake.Data), string(baseMatch[0][2]))
	// Older workspace-capable templates force mkLab before flake self exists.
	// Discover names with a profile-free shape, then lazily forward each real
	// output. This keeps the exact pinned API, validation and revision metadata.
	if bytes.Count(flake.Data, []byte("    deployment // {")) == 1 {
		flake.Data = bytes.Replace(flake.Data, []byte("    deployment // {"), []byte("    builtins.mapAttrs (name: _: deployment.${name})\n      (nixorium.lib.mkLab { deploymentSelf = ./.; inherit labConfig; }) // {"), 1)
	}
	files["flake.nix"] = flake
	return files, selected, nil
}

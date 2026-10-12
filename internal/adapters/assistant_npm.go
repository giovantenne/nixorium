package adapters

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"github.com/giovantenne/nixorium/internal/npmartifact"
	"golang.org/x/sys/unix"
)

func assistantNixQuote(value string) string {
	data, _ := json.Marshal(value)
	return strings.ReplaceAll(string(data), "${", `\${`)
}

func assistantNpmResolve(ctx context.Context, registry *npmartifact.Registry, node, name, version string) ([]byte, error) {
	bwrap, resolver := os.Getenv("NIXORIUM_NPM_BWRAP"), os.Getenv("NIXORIUM_NPM_RESOLVER")
	if !filepath.IsAbs(bwrap) || !filepath.IsAbs(resolver) {
		return nil, errors.New("resolver_unavailable")
	}
	stage, err := os.MkdirTemp("", "nixorium-npm-resolver-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	socket := filepath.Join(stage, "registry.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	server := &http.Server{Handler: registry, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	defer server.Close()
	args := []string{"--unshare-all", "--die-with-parent", "--new-session", "--ro-bind", "/nix/store", "/nix/store", "--tmpfs", "/tmp", "--dir", "/run", "--ro-bind", socket, "/run/registry.sock", "--ro-bind", resolver, "/resolver", "--proc", "/proc", "--dev", "/dev", "--clearenv", "/resolver", node, name, version}
	command := exec.CommandContext(ctx, bwrap, args...)
	configureCommandCancellation(command)
	command.Env = []string{"PATH=/nonexistent"}
	stdout := &boundedCommandBuffer{limit: npmartifact.MaxLockBytes}
	command.Stdout = stdout
	command.Stderr = io.Discard
	if err = command.Run(); err != nil {
		return nil, errors.New("resolution_failed")
	}
	if stdout.truncated {
		return nil, errors.New("limit_exceeded")
	}
	return stdout.buffer.Bytes(), nil
}

func assistantNpmFiles(w *AssistantWorkspace, files map[string]string) error {
	// Preflight the entire transaction against the same limits and destinations
	// used by normal file writes before publishing any generated file.
	entries, err := w.Entries()
	if err != nil {
		return err
	}
	for p := range files {
		if w.deleted[p] {
			return errors.New("artifact_path_exists")
		}
		if _, exists := w.base[p]; exists {
			return errors.New("artifact_path_exists")
		}
		for _, entry := range entries {
			if entry.Path == p || strings.HasPrefix(entry.Path, p+"/") || strings.HasPrefix(p, entry.Path+"/") {
				return errors.New("artifact_path_exists")
			}
		}
		if !domain.ValidAssistantPath(p) || len(files[p]) > 512*1024 || !assistantText([]byte(files[p])) {
			return errors.New("limit_exceeded")
		}
	}
	count, total := len(w.deleted)+len(w.modes)+len(files), 0
	if count > 200 {
		return errors.New("limit_exceeded")
	}
	for p := range w.modes {
		data, err := w.raw(p)
		if err != nil {
			return err
		}
		total += len(data)
	}
	for _, data := range files {
		total += len(data)
	}
	if total > 4*1024*1024 {
		return errors.New("limit_exceeded")
	}
	paths := []string{}
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return errors.New("invalid_arguments")
	}
	directory := filepath.Dir(paths[0])
	parts := strings.Split(directory, "/")
	if len(parts) != 2 || parts[0] != "npm" || !npmartifact.IDPattern.MatchString(parts[1]) {
		return errors.New("invalid_arguments")
	}
	for _, p := range paths {
		if filepath.Dir(p) != directory {
			return errors.New("invalid_arguments")
		}
	}
	overlay, err := w.overlay()
	if err != nil {
		return err
	}
	defer overlay.Close()
	parent, err := assistantDirectory(overlay, "npm", true)
	if err != nil {
		return err
	}
	defer parent.Close()
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return err
	}
	stagedName := ".npm-prepare-" + hex.EncodeToString(random[:])
	stagedParent := w.root
	staged, err := assistantDirectory(stagedParent, stagedName, true)
	if err != nil {
		return err
	}
	defer staged.Close()
	published := false
	defer func() {
		if !published {
			for _, p := range paths {
				unix.Unlinkat(int(staged.Fd()), filepath.Base(p), 0)
			}
			unix.Unlinkat(int(stagedParent.Fd()), stagedName, unix.AT_REMOVEDIR)
		}
	}()
	for _, p := range paths {
		if err = assistantWriteState(staged, filepath.Base(p), []byte(files[p])); err != nil {
			return err
		}
	}
	// Publish the complete directory in one rename; modes then records the whole
	// set in one atomic write. A crash cannot create a partially savable artifact.
	if err = unix.Renameat2(int(stagedParent.Fd()), stagedName, int(parent.Fd()), parts[1], unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	stagedName = parts[1]
	stagedParent = parent
	if err = parent.Sync(); err != nil {
		return err
	}
	nextModes := map[string]bool{}
	for p, v := range w.modes {
		nextModes[p] = v
	}
	for _, p := range paths {
		nextModes[p] = false
	}
	data, _ := json.Marshal(nextModes)
	if err = assistantWriteState(w.root, "modes.json", data); err != nil {
		return err
	}
	w.modes = nextModes
	published = true
	return nil
}

func prepareAssistantNpm(ctx context.Context, store AssistantWorkspaceStore, w *AssistantWorkspace, call domain.AssistantToolCall) string {
	return prepareAssistantNpmWithRunner(ctx, store, w, call, runBoundedCommand)
}

func prepareAssistantNpmWithRunner(ctx context.Context, store AssistantWorkspaceStore, w *AssistantWorkspace, call domain.AssistantToolCall, run assistantCommandRunner) string {
	var args struct {
		Package string
		Version string
		ID      string
	}
	if json.Unmarshal(call.Arguments, &args) != nil || !npmartifact.ValidName(args.Package) || !npmartifact.IDPattern.MatchString(args.ID) {
		return assistantToolFailure(nil)
	}
	if args.Version == "" {
		args.Version = "latest"
	}
	if !npmartifact.ValidSelector(args.Version) {
		return assistantToolFailure(nil)
	}
	artifactLock, err := assistantNpmRootDirectory(store)
	if err != nil {
		return assistantToolResult(map[string]any{"ok": false, "error": "artifact_storage_unavailable"})
	}
	defer artifactLock.Close()
	if err = unix.Flock(int(artifactLock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return assistantToolResult(map[string]any{"ok": false, "error": "artifact_busy"})
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	fail := func(code string) string {
		return assistantToolResult(map[string]any{"ok": false, "error": code, "message": "Could not prepare the npm package. Public registry CLI packages with SHA-512 integrity are supported; native addons, bundled dependencies, and hooks requiring network access or unavailable build tools are unsupported."})
	}
	registry := npmartifact.NewRegistry(ctx)
	version, err := registry.Resolve(args.Package, args.Version)
	if err != nil {
		return fail(err.Error())
	}
	ref := (&url.URL{Scheme: "git+file", Path: w.Conversation.Repository, RawQuery: "rev=" + w.Conversation.Base}).String()
	expression := "let f = builtins.getFlake " + assistantNixQuote(ref) + "; in f.inputs.nixpkgs.legacyPackages.x86_64-linux.nodejs"
	output, _, err := run(ctx, 65536, "nix", "build", "--no-link", "--json", "--no-write-lock-file", "--no-update-lock-file", "--expr", expression)
	if err != nil {
		return fail("runtime_unavailable")
	}
	var result []struct {
		Outputs map[string]string `json:"outputs"`
	}
	if json.Unmarshal([]byte(output), &result) != nil || len(result) != 1 || !domain.ValidStorePath(result[0].Outputs["out"]) {
		return fail("runtime_unavailable")
	}
	node := result[0].Outputs["out"]
	lock, err := assistantNpmResolve(ctx, registry, node, args.Package, version)
	if err != nil {
		return fail(err.Error())
	}
	lock, manifest, err := npmartifact.ValidateLock(lock, args.Package, version)
	if err != nil {
		return fail(err.Error())
	}
	stage, err := os.MkdirTemp("", "nixorium-npm-artifact-")
	if err != nil {
		return fail("preparation_failed")
	}
	defer os.RemoveAll(stage)
	manifest.NodeRuntime = node
	identifier := sha256.Sum256([]byte(stage + node))
	manifest.ArtifactID = hex.EncodeToString(identifier[:])
	budget := npmartifact.ArchiveBudget{}
	for _, dep := range manifest.Dependencies {
		if dep.Optional {
			continue
		}
		data, err := registry.Get(strings.TrimPrefix(dep.URL, "https://registry.npmjs.org"), npmartifact.MaxDownloadBytes)
		if err != nil {
			return fail(err.Error())
		}
		bins, err := budget.Inspect(data, dep, dep.Path == "node_modules/"+args.Package)
		if err != nil {
			return assistantToolResult(map[string]any{"ok": false, "error": err.Error(), "dependency": dep.Path, "message": "This dependency could not be safely prepared for offline installation."})
		}
		if bins != nil {
			manifest.Bins = bins
		}
		name := npmartifact.TarballName(dep)
		file := filepath.Join(stage, name)
		if err = os.WriteFile(file, data, 0600); err != nil {
			return fail("preparation_failed")
		}
		if _, _, err = run(ctx, 4096, "nix", "store", "add-file", "--hash-algo", "sha512", "--name", name, file); err != nil {
			return fail("preparation_failed")
		}
		os.Remove(file)
	}
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	packageData, _ := json.MarshalIndent(map[string]any{"name": "nixorium-npm-baseline", "version": "1.0.0", "private": true, "dependencies": map[string]string{args.Package: version}}, "", "  ")
	nix, module := npmartifact.Render(manifest)
	generated := map[string]string{"package.json": string(packageData) + "\n", "package-lock.json": string(lock), "manifest.json": string(manifestData) + "\n", "default.nix": nix, "module.nix": module}
	for path, data := range generated {
		if len(data) > 512*1024 {
			return fail("limit_exceeded")
		}
		if err = os.WriteFile(filepath.Join(stage, path), []byte(data), 0600); err != nil {
			return fail("preparation_failed")
		}
	}
	id := manifest.ArtifactID
	roots, err := assistantNpmRootDirectory(store)
	if err != nil {
		return fail("artifact_storage_unavailable")
	}
	rootPath := filepath.Join(store.Directory, "npm-artifacts", id)
	entries, err := roots.ReadDir(-1)
	roots.Close()
	if err != nil || len(entries) >= 64 {
		return fail("artifact_storage_limit")
	}
	if _, err = os.Lstat(rootPath); !errors.Is(err, os.ErrNotExist) {
		return fail("artifact_storage_limit")
	}
	buildExpression := "let f = builtins.getFlake " + assistantNixQuote(ref) + "; pkgs = f.inputs.nixpkgs.legacyPackages.x86_64-linux; in import " + assistantNixQuote(stage+"/default.nix") + " { inherit pkgs; }"
	output, _, err = run(ctx, 65536, "nix", "build", "--impure", "--out-link", rootPath, "--json", "--option", "sandbox", "true", "--option", "timeout", "300", "--option", "max-silent-time", "60", "--option", "max-jobs", "1", "--option", "cores", "2", "--expr", buildExpression)
	if err != nil {
		os.Remove(rootPath)
		return fail("offline_build_or_runtime_failed")
	}
	result = nil
	if json.Unmarshal([]byte(output), &result) != nil || len(result) != 1 || !domain.ValidStorePath(result[0].Outputs["out"]) {
		os.Remove(rootPath)
		return fail("preparation_failed")
	}
	artifact := result[0].Outputs["out"]
	sizeOutput, _, err := run(ctx, 65536, "nix", "path-info", "--json", "--closure-size", artifact)
	var sizes map[string]struct {
		ClosureSize int64 `json:"closureSize"`
	}
	if err != nil || json.Unmarshal([]byte(sizeOutput), &sizes) != nil || len(sizes) != 1 || sizes[artifact].ClosureSize <= 0 {
		os.Remove(rootPath)
		return fail("artifact_storage_unavailable")
	}
	roots, err = assistantNpmRootDirectory(store)
	if err != nil {
		os.Remove(rootPath)
		return fail("artifact_storage_unavailable")
	}
	var total int64 = sizes[artifact].ClosureSize
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			record, err := assistantReadStateLimit(roots, entry.Name(), 4096)
			var metadata assistantNpmArtifactRecord
			if err != nil || assistantDecodeStrict(record, &metadata) != nil {
				roots.Close()
				os.Remove(rootPath)
				return fail("artifact_storage_unavailable")
			}
			total += metadata.ClosureSize
		}
	}
	if total > 4*1024*1024*1024 {
		roots.Close()
		os.Remove(rootPath)
		return fail("artifact_storage_limit")
	}
	metadata, _ := json.Marshal(assistantNpmArtifactRecord{Repository: w.Conversation.Repository, Manifest: "npm/" + args.ID + "/manifest.json", ArtifactID: id, ClosureSize: sizes[artifact].ClosureSize})
	if err = assistantWriteState(roots, id+".json", metadata); err != nil {
		roots.Close()
		os.Remove(rootPath)
		return fail("artifact_storage_unavailable")
	}
	roots.Close()
	files := map[string]string{}
	prefix := "npm/" + args.ID + "/"
	for path, data := range generated {
		files[prefix+path] = data
	}
	if err = assistantNpmFiles(w, files); err != nil {
		os.Remove(rootPath)
		os.Remove(rootPath + ".json")
		return fail(err.Error())
	}
	return assistantToolResult(map[string]any{"ok": true, "package": args.Package, "version": version, "artifact": result[0].Outputs["out"], "nodeRuntime": node, "module": prefix + "module.nix", "manifest": prefix + "manifest.json", "bins": manifest.Bins, "dependencies": len(manifest.Dependencies), "omittedOptional": manifest.OmittedOptional, "verification": []string{"offline npm ci", "sandboxed --version", "sandboxed --help"}, "message": "Import the generated module only for the requested hosts, then evaluate and build the candidate. Optional dependencies are omitted; optional features such as native clipboard support may be unavailable. No installation, activation or home reset has occurred."})
}

type assistantNpmArtifactRecord struct {
	Repository  string `json:"repository"`
	Manifest    string `json:"manifest"`
	ArtifactID  string `json:"artifactID"`
	ClosureSize int64  `json:"closureSize"`
}

func (store AssistantWorkspaceStore) cleanupNpmArtifacts() error {
	roots, err := assistantNpmRootDirectory(store)
	if err != nil {
		return err
	}
	defer roots.Close()
	if err := unix.Flock(int(roots.Fd()), unix.LOCK_EX|unix.LOCK_NB); errors.Is(err, unix.EWOULDBLOCK) {
		return nil
	} else if err != nil {
		return err
	}
	entries, err := roots.ReadDir(-1)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, entry := range entries {
		known[entry.Name()] = true
	}
	for _, entry := range entries {
		if regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(entry.Name()) && entry.Type()&os.ModeSymlink != 0 && !known[entry.Name()+".json"] {
			if err := unix.Unlinkat(int(roots.Fd()), entry.Name(), 0); err != nil {
				return err
			}
		}
	}
	conversations, err := store.conversations(false)
	if err != nil {
		return err
	}
	defer conversations.Close()
	chats, err := conversations.ReadDir(-1)
	if err != nil {
		return err
	}
	contains := func(data []byte, id string) bool {
		var manifest npmartifact.Manifest
		return json.Unmarshal(data, &manifest) == nil && manifest.ArtifactID == id
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := assistantReadStateLimit(roots, entry.Name(), 4096)
		var record assistantNpmArtifactRecord
		if err != nil || assistantDecodeStrict(data, &record) != nil || entry.Name() != record.ArtifactID+".json" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(record.ArtifactID) || !domain.ValidAssistantPath(record.Manifest) || !strings.HasPrefix(record.Manifest, "npm/") || !filepath.IsAbs(record.Repository) {
			return errors.New("invalid npm artifact record")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		if _, err := resetGit(ctx, record.Repository, nil, 4096, "rev-parse", "--verify", "HEAD^{commit}"); err != nil {
			cancel()
			continue
		}
		tracked, err := resetGit(ctx, record.Repository, nil, 4096, "ls-tree", "HEAD", "--", record.Manifest)
		if err != nil {
			cancel()
			continue
		}
		dataString := ""
		if strings.TrimSpace(tracked) != "" {
			dataString, err = resetGit(ctx, record.Repository, nil, npmartifact.MaxLockBytes, "show", "HEAD:"+record.Manifest)
			if err != nil {
				cancel()
				continue
			}
		}
		cancel()
		keep := contains([]byte(dataString), record.ArtifactID)
		for _, chat := range chats {
			if keep {
				break
			}
			if !chat.IsDir() || !assistantTurnPattern.MatchString(chat.Name()) {
				continue
			}
			root, err := assistantDirectory(conversations, chat.Name(), false)
			if err != nil {
				return err
			}
			meta, err := assistantReadStateLimit(root, "meta.json", assistantMetadataLimit)
			var conversation domain.AssistantConversation
			if err != nil || json.Unmarshal(meta, &conversation) != nil {
				root.Close()
				return errors.New("invalid Assistant artifact reference")
			}
			if conversation.Repository == record.Repository {
				deleted, err := assistantReadStateLimit(root, "deleted.json", 128*1024)
				var paths []string
				if err != nil || json.Unmarshal(deleted, &paths) != nil {
					root.Close()
					return errors.New("invalid Assistant artifact reference")
				}
				removed := false
				for _, p := range paths {
					if p == record.Manifest {
						removed = true
					}
				}
				if !removed {
					overlay, err := assistantDirectory(root, "overlay", false)
					if err != nil {
						root.Close()
						return err
					}
					parent, name, err := assistantParent(overlay, record.Manifest, false)
					overlay.Close()
					if err == nil {
						content, err := assistantReadStateLimit(parent, name, npmartifact.MaxLockBytes)
						parent.Close()
						if err == nil {
							keep = contains(content, record.ArtifactID)
						}
					}
				}
			}
			root.Close()
		}
		if !keep {
			if err := unix.Unlinkat(int(roots.Fd()), record.ArtifactID, 0); err != nil && !errors.Is(err, unix.ENOENT) {
				return err
			}
			if err := unix.Unlinkat(int(roots.Fd()), entry.Name(), 0); err != nil {
				return err
			}
		}
	}
	return nil
}

func assistantNpmRootDirectory(store AssistantWorkspaceStore) (*os.File, error) {
	root, err := (AssistantStateStore(store)).root()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return assistantDirectory(root, "npm-artifacts", true)
}

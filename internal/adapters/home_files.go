package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

var homeFilesHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Go's ordinary decoder accepts case-insensitive field aliases. Walk tokens
// against the actual wire type first, preserving duplicate/null rejection in
// draft and exported-plan wrappers as well as in the raw profile schema.
func decodeHomeFilesWire(data []byte, target any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var walk func(reflect.Type, int) error
	walk = func(kind reflect.Type, depth int) error {
		if depth > 12 {
			return errors.New("draft nesting limit exceeded")
		}
		for kind.Kind() == reflect.Pointer {
			kind = kind.Elem()
		}
		token, err := d.Token()
		if err != nil || token == nil {
			return errors.New("invalid or null draft value")
		}
		delimiter, container := token.(json.Delim)
		if !container {
			if kind.Kind() == reflect.Struct || kind.Kind() == reflect.Slice {
				return errors.New("invalid draft structure")
			}
			return nil
		}
		switch delimiter {
		case '{':
			if kind.Kind() != reflect.Struct {
				return errors.New("unexpected draft object")
			}
			allowed := map[string]reflect.Type{}
			for index := 0; index < kind.NumField(); index++ {
				field := kind.Field(index)
				name := strings.Split(field.Tag.Get("json"), ",")[0]
				if name == "" {
					name = field.Name
				}
				if name != "-" {
					allowed[name] = field.Type
				}
			}
			seen := map[string]bool{}
			for d.More() {
				token, err := d.Token()
				name, ok := token.(string)
				next, known := allowed[name]
				if err != nil || !ok || !known || seen[name] {
					return errors.New("unknown, aliased or duplicate draft field")
				}
				seen[name] = true
				if err := walk(next, depth+1); err != nil {
					return err
				}
			}
		case '[':
			if kind.Kind() != reflect.Slice {
				return errors.New("unexpected draft array")
			}
			for d.More() {
				if err := walk(kind.Elem(), depth+1); err != nil {
					return err
				}
			}
		default:
			return errors.New("invalid draft delimiter")
		}
		_, err = d.Token()
		return err
	}
	if err := walk(reflect.TypeOf(target).Elem(), 0); err != nil {
		return err
	}
	if _, err := d.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing draft JSON")
	}
	return assistantDecodeStrict(data, target)
}

func (Local) ReadHomeFiles(repository string) ([]byte, error) {
	root, err := openWorkspaceRoot(repository, unix.LOCK_SH)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, info, err := readWorkspaceFile(root, domain.HomeFilesFileName, domain.HomeFilesMaxBytes)
	if err == nil && info.Nlink != 1 {
		return nil, errors.New("initial home declaration has hard links")
	}
	return data, err
}

func ReadHomeFilesDraft(path string) (domain.HomeFilesDraft, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return domain.HomeFilesDraft{}, err
	}
	root, err := openWorkspaceRoot(filepath.Dir(absolute), unix.LOCK_SH)
	if err != nil {
		return domain.HomeFilesDraft{}, err
	}
	defer root.Close()
	data, info, err := readWorkspaceFile(root, filepath.Base(absolute), 4*domain.HomeFilesMaxBytes)
	if err != nil {
		return domain.HomeFilesDraft{}, err
	}
	if info.Nlink != 1 {
		return domain.HomeFilesDraft{}, errors.New("draft must not have hard links")
	}
	if !utf8.Valid(data) {
		return domain.HomeFilesDraft{}, errors.New("draft must be UTF-8 JSON")
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(data, &fields); err != nil {
		return domain.HomeFilesDraft{}, errors.New("invalid draft JSON")
	}
	var draft domain.HomeFilesDraft
	if _, ok := fields["candidate"]; ok {
		var report domain.HomeFilesPlanReport
		if err = decodeHomeFilesWire(data, &report); err != nil || report.Candidate == nil {
			return draft, errors.New("invalid reviewed initial-file report")
		}
		draft = *report.Candidate
	} else if _, ok := fields["profile"]; ok {
		if err = decodeHomeFilesWire(data, &draft); err != nil {
			return draft, errors.New("invalid initial-file draft")
		}
	} else {
		p, issues := domain.DecodeHomeFilesProfile(data)
		if len(issues) != 0 {
			return draft, errors.New(issues[0].Message)
		}
		draft.Profile = p
	}
	if _, err = domain.MarshalHomeFilesProfile(draft.Profile); err != nil {
		return domain.HomeFilesDraft{}, err
	}
	return draft, nil
}

type homeFilesState struct {
	Revision, Index, Pin, Base string
	BaseExists                 bool
	RootDevice, RootInode      uint64
}

func readHomeFilesState(ctx context.Context, root *os.File) (state homeFilesState, base domain.HomeFilesProfile, err error) {
	base.SchemaVersion = 1
	info, err := workspaceRootIdentity(root)
	if err != nil {
		return state, base, err
	}
	state.RootDevice = uint64(info.Dev)
	state.RootInode = info.Ino
	top, err := workspaceGit(ctx, root.Name(), "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(top) != root.Name() {
		return state, base, errors.New("initial files require the committed deployment Git root")
	}
	revision, err := workspaceGit(ctx, root.Name(), "rev-parse", "--verify", "HEAD")
	if err != nil {
		return state, base, err
	}
	state.Revision = strings.TrimSpace(revision)
	if !validGitRevision(state.Revision) {
		return state, base, errors.New("a committed deployment revision is required")
	}
	index, err := workspaceGit(ctx, root.Name(), "ls-files", "--stage", "-z")
	if err != nil {
		return state, base, err
	}
	state.Index = workspaceDigest(index)
	tracked := map[string]bool{}
	for _, line := range strings.Split(index, "\x00") {
		if line == "" {
			continue
		}
		header, path, ok := strings.Cut(line, "\t")
		fields := strings.Fields(header)
		if !ok || len(fields) != 3 || fields[2] != "0" || (fields[0] != "100644" && fields[0] != "100755" && fields[0] != "120000") {
			return state, base, errors.New("resolve Git conflicts or submodules before reviewing initial files")
		}
		if isPrivateDeploymentPath(path) {
			return state, base, errors.New("private deployment files must not be tracked")
		}
		tracked[path] = true
	}
	if !tracked["flake.nix"] || !tracked["flake.lock"] {
		return state, base, errors.New("initial files require tracked flake.nix and flake.lock")
	}
	if _, _, err = readWorkspaceFile(root, "flake.nix", 1024*1024); err != nil {
		return state, base, err
	}
	lock, _, err := readWorkspaceFile(root, "flake.lock", 4*1024*1024)
	if err != nil {
		return state, base, err
	}
	state.Pin = workspaceDigest(lock)
	data, fileInfo, err := readWorkspaceFile(root, domain.HomeFilesFileName, domain.HomeFilesMaxBytes)
	if errors.Is(err, os.ErrNotExist) {
		state.Base = workspaceDigest("absent-home-files")
		return state, base, nil
	}
	if err != nil {
		return state, base, err
	}
	if fileInfo.Nlink != 1 {
		return state, base, errors.New("initial home declaration must not have hard links")
	}
	base, issues := domain.DecodeHomeFilesProfile(data)
	if len(issues) != 0 {
		return state, base, errors.New("existing initial home declaration is invalid; inspect Git review")
	}
	state.BaseExists = true
	state.Base = workspaceDigest(struct {
		Data           []byte
		Mode, UID, GID uint32
		Device, Inode  uint64
	}{data, fileInfo.Mode, fileInfo.Uid, fileInfo.Gid, uint64(fileInfo.Dev), fileInfo.Ino})
	return state, base, nil
}

func homeFilesPayloads(ctx context.Context, root *os.File, draft domain.HomeFilesDraft) (map[string][]byte, []domain.HomeFilesEntry, error) {
	captured := map[string][]byte{}
	wanted := map[string]bool{}
	for _, f := range draft.Profile.Files {
		if f.Source != nil {
			wanted[strings.TrimPrefix(*f.Source, "imports/")] = true
		}
	}
	capturedSize := int64(0)
	for _, id := range draft.CaptureIDs {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		record, stage, err := readHomeCapture(root.Name(), id)
		if err != nil {
			return nil, nil, err
		}
		for _, entry := range record.Report.Files {
			if entry.Source == nil {
				stage.Close()
				return nil, nil, errors.New("invalid capture file metadata")
			}
			hash := strings.TrimPrefix(*entry.Source, "imports/")
			if !wanted[hash] {
				continue
			}
			if _, ok := captured[hash]; ok {
				continue
			}
			data, info, err := readWorkspaceFile(stage, hash, domain.HomeFilesMaxFileBytes)
			if err != nil || info.Nlink != 1 || info.Uid != uint32(os.Geteuid()) || info.Mode&0777 != 0600 || fmt.Sprintf("%x", sha256.Sum256(data)) != hash {
				stage.Close()
				return nil, nil, domain.ErrHomeFilesConflict
			}
			capturedSize += int64(len(data))
			if capturedSize > domain.HomeFilesMaxTotalBytes {
				stage.Close()
				return nil, nil, errors.New("captured payloads exceed 64 MiB")
			}
			captured[hash] = data
		}
		stage.Close()
	}
	payloads := map[string][]byte{}
	entries := []domain.HomeFilesEntry{}
	total := int64(0)
	for _, dir := range draft.Profile.Directories {
		entries = append(entries, domain.HomeFilesEntry{Destination: dir, Directory: true})
	}
	for _, entry := range draft.Profile.Files {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		var data []byte
		if entry.Text != nil {
			data = []byte(*entry.Text)
		} else {
			hash := strings.TrimPrefix(*entry.Source, "imports/")
			var ok bool
			data, ok = captured[hash]
			if !ok {
				tracked, err := workspaceGit(ctx, root.Name(), "ls-files", "--error-unmatch", "--", domain.HomeFilesDirectoryName+"/"+*entry.Source)
				if err != nil || strings.TrimSpace(tracked) == "" {
					return nil, nil, errors.New("imported payload is not tracked or captured; capture it again in Initial files")
				}
				var info unix.Stat_t
				data, info, err = readWorkspaceFile(root, domain.HomeFilesDirectoryName+"/"+*entry.Source, domain.HomeFilesMaxFileBytes)
				if err != nil || info.Nlink != 1 {
					return nil, nil, errors.New("imported payload must be a bounded regular file without hard links")
				}
			}
			if fmt.Sprintf("%x", sha256.Sum256(data)) != hash {
				return nil, nil, domain.ErrHomeFilesConflict
			}
			if domain.HomeFilesSensitiveContent(data) || rejectSecretPatch(string(data)) != nil {
				return nil, nil, errors.New("imported content contains credential markers")
			}
			payloads[hash] = data
		}
		total += int64(len(data))
		if total > domain.HomeFilesMaxTotalBytes {
			return nil, nil, errors.New("additional files exceed 64 MiB")
		}
		entries = append(entries, domain.HomeFilesEntry{Destination: entry.Destination, SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Size: int64(len(data)), Executable: entry.Executable})
	}
	return payloads, entries, nil
}

func copyHomeFilesEvaluationSource(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil || relative == "." {
			return err
		}
		if isPrivateDeploymentPath(relative) {
			return errors.New("private file in evaluation source")
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.Mkdir(destination, 0700)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(relative), link))
			if filepath.IsAbs(link) || resolved == ".." || strings.HasPrefix(resolved, "../") {
				return errors.New("evaluation source has an escaping link")
			}
			return os.Symlink(link, destination)
		}
		if !entry.Type().IsRegular() {
			return errors.New("evaluation source has a special file")
		}
		data, mode, err := readRecoveryFile(path, 128*1024*1024)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, data, os.FileMode(0600|(mode&0111)))
	})
}

func (local Local) InspectHomeFiles(ctx context.Context, repository string, draft domain.HomeFilesDraft) (domain.HomeFilesInspection, error) {
	root, err := openWorkspaceRoot(repository, unix.LOCK_SH)
	if err != nil {
		return domain.HomeFilesInspection{}, err
	}
	defer root.Close()
	return local.inspectHomeFilesLocked(ctx, root, draft)
}
func (local Local) inspectHomeFilesLocked(ctx context.Context, root *os.File, draft domain.HomeFilesDraft) (domain.HomeFilesInspection, error) {
	var inspection domain.HomeFilesInspection
	before, base, err := readHomeFilesState(ctx, root)
	if err != nil {
		return inspection, err
	}
	payloads, entries, err := homeFilesPayloads(ctx, root, draft)
	if err != nil {
		return inspection, err
	}
	_, baseEntries, err := homeFilesPayloads(ctx, root, domain.HomeFilesDraft{Profile: base})
	if err != nil {
		return inspection, err
	}
	ref, err := local.EvaluationSource(ctx, root.Name(), "")
	if err != nil {
		return inspection, err
	}
	archived, err := runBoundedNix(ctx, 4*1024*1024, "flake", "archive", ref, "--json", "--no-write-lock-file", "--no-update-lock-file")
	if err != nil {
		return inspection, errors.New("cannot prepare the tracked initial-file source; inspect pinned inputs")
	}
	var source struct {
		Path string `json:"path"`
	}
	if json.Unmarshal([]byte(archived), &source) != nil || !domain.ValidStorePath(source.Path) {
		return inspection, errors.New("invalid tracked evaluation source")
	}
	temporary, err := os.MkdirTemp("", "nixorium-home-files-review-*")
	if err != nil {
		return inspection, err
	}
	defer os.RemoveAll(temporary)
	if err = copyHomeFilesEvaluationSource(source.Path, temporary); err != nil {
		return inspection, err
	}
	data, err := domain.MarshalHomeFilesProfile(draft.Profile)
	if err != nil {
		return inspection, err
	}
	stageRoot, err := openWorkspaceRoot(temporary, unix.LOCK_EX)
	if err != nil {
		return inspection, err
	}
	defer stageRoot.Close()
	if err = writeHomeStageManifest(stageRoot, data); err != nil {
		return inspection, err
	}
	storage, err := homeFilesDirectory(stageRoot, domain.HomeFilesDirectoryName)
	if err != nil {
		return inspection, err
	}
	defer storage.Close()
	imports, err := homeFilesDirectory(storage, "imports")
	if err != nil {
		return inspection, err
	}
	defer imports.Close()
	for hash, content := range payloads {
		fd, err := unix.Openat2(int(imports.Fd()), hash, &unix.OpenHow{Flags: unix.O_WRONLY | unix.O_CREAT | unix.O_TRUNC | unix.O_CLOEXEC | unix.O_NONBLOCK, Mode: 0600, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
		if err != nil {
			return inspection, errors.New("payload conflicts with a source link")
		}
		file := os.NewFile(uintptr(fd), hash)
		_, writeErr := file.Write(content)
		closeErr := file.Close()
		if err = errors.Join(writeErr, closeErr); err != nil {
			return inspection, err
		}
	}
	added, err := runBoundedNix(ctx, 4096, "store", "add-path", "--name", "source", temporary)
	if err != nil {
		return inspection, errors.New("cannot freeze the reviewed initial-file source")
	}
	candidate := strings.TrimSpace(added)
	if !domain.ValidStorePath(candidate) {
		return inspection, errors.New("invalid reviewed source path")
	}
	expression := fmt.Sprintf(`let d = builtins.getFlake %q; c = builtins.readFile (d.outPath + "/home-files.json"); in assert d.nixoriumValidateHomeFilesCandidate c == true; { resolved = d.nixoriumResolveHomeFilesCandidate c; actual = d.nixoriumHomeFiles.declared; }`, "path:"+candidate)
	output, err := runBoundedNix(ctx, 2*1024*1024, "eval", "--impure", "--json", "--no-write-lock-file", "--no-update-lock-file", "--option", "allow-import-from-derivation", "false", "--expr", expression)
	if err != nil {
		return inspection, errors.New("initial-file preparation failed; check Student workspace preferences, pinned inputs and both deployment candidate hooks")
	}
	var response struct {
		Resolved struct {
			SchemaVersion      int `json:"schemaVersion"`
			State, ManagedFile string
			Declared           domain.HomeFilesProfile
			StudentUser        string
			Targets            []domain.WorkspaceTarget
			Seed               string
		}
		Actual domain.HomeFilesProfile
	}
	if json.Unmarshal([]byte(output), &response) != nil || response.Resolved.SchemaVersion != 1 || response.Resolved.State != "prepared" || response.Resolved.ManagedFile != domain.HomeFilesFileName || !domain.ValidStorePath(response.Resolved.Seed) || len(response.Resolved.Targets) == 0 || response.Resolved.Targets[0].Role != "controller" {
		return inspection, errors.New("deployment does not support initial student files; update Nixorium first")
	}
	metadata := response.Resolved
	resolved, err := domain.MarshalHomeFilesProfile(metadata.Declared)
	if err != nil || !bytes.Equal(resolved, data) {
		return inspection, errors.New("deployment overrides the initial-file declaration; review its mkLab arguments")
	}
	actual, err := domain.MarshalHomeFilesProfile(response.Actual)
	if err != nil || !bytes.Equal(actual, data) {
		return inspection, errors.New("deployment overrides the initial-file declaration; review its mkLab arguments")
	}
	after, _, err := readHomeFilesState(ctx, root)
	if err != nil {
		return inspection, err
	}
	if before != after {
		return inspection, domain.ErrHomeFilesConflict
	}
	// Materialize the tracked source again after evaluation, catching worktree
	// byte changes that neither HEAD nor the index manifest would reveal.
	afterRef, err := local.EvaluationSource(ctx, root.Name(), "")
	if err != nil {
		return inspection, err
	}
	afterArchive, err := runBoundedNix(ctx, 4*1024*1024, "flake", "archive", afterRef, "--json", "--no-write-lock-file", "--no-update-lock-file")
	if err != nil {
		return inspection, err
	}
	var afterSource struct {
		Path string `json:"path"`
	}
	if json.Unmarshal([]byte(afterArchive), &afterSource) != nil || source.Path != afterSource.Path {
		return inspection, domain.ErrHomeFilesConflict
	}
	inspection = domain.HomeFilesInspection{Snapshot: domain.HomeFilesSnapshot{Revision: after.Revision, SourceFingerprint: workspaceDigest(struct {
		State  homeFilesState
		Source string
	}{after, source.Path}), PinFingerprint: after.Pin, BaseFingerprint: after.Base, BaseExists: after.BaseExists, PayloadFingerprint: workspaceDigest(entries)}, Base: base, Entries: entries, BaseEntries: baseEntries, StudentUser: metadata.StudentUser, Targets: metadata.Targets}
	return inspection, nil
}

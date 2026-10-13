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
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/giovantenne/nixorium/internal/domain"
)

const assistantHistoryOutputLimit = 32 * 1024
const assistantHistoryBlobLimit = 64 * 1024

var assistantHistoryRevision = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

type assistantHistoryBlob struct {
	object string
	mode   string
	size   int
}
type assistantHistoryTree struct {
	files   map[string]assistantHistoryBlob
	denied  map[string]bool
	ignores string
}
type assistantHistoryReader struct {
	ctx        context.Context
	repository string
	base       string
	trees      map[string]*assistantHistoryTree
	temporary  []string
	imported   map[string]bool
}

// assistantHistoryGit never returns Git diagnostics, which can contain private
// paths. Every command uses bounded output and the caller's fixed deadline.
func assistantHistoryGit(ctx context.Context, repository string, input []byte, limit int, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"--no-pager", "--no-optional-locks", "-C", repository,
		"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "core.attributesFile=/dev/null",
		"-c", "core.excludesFile=/dev/null", "-c", "diff.external=", "-c", "log.showSignature=false", "-c", "core.quotePath=true"}, args...)...)
	configureCommandCancellation(command)
	command.Env = append(workspaceEnvironment(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_REPLACE_OBJECTS=1", "GIT_GRAFT_FILE=/dev/null", "GIT_NO_LAZY_FETCH=1", "GIT_PAGER=cat")
	command.Stdin = bytes.NewReader(input)
	output := &boundedCommandBuffer{limit: limit}
	command.Stdout, command.Stderr = output, &boundedCommandBuffer{limit: 1024}
	err := command.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if output.truncated {
		return output.buffer.String(), errors.New("limit_exceeded")
	}
	if err != nil {
		return output.buffer.String(), fmt.Errorf("Git %s: %w", args[0], err)
	}
	return output.buffer.String(), nil
}

func assistantHistoryVisiblePath(path string) bool {
	return domain.ValidAssistantPath(path) && !domain.AssistantHiddenPath(path) &&
		filepath.Base(path) != "AGENTS.md" && filepath.Base(path) != "CLAUDE.md" &&
		domain.RedactForModel(path) == path
}

func (reader *assistantHistoryReader) tree(revision string) (*assistantHistoryTree, error) {
	if tree := reader.trees[revision]; tree != nil {
		return tree, nil
	}
	data, err := assistantHistoryGit(reader.ctx, reader.repository, nil, 2*1024*1024, "ls-tree", "-r", "-l", "-z", revision)
	if err != nil {
		return nil, err
	}
	tree := &assistantHistoryTree{files: map[string]assistantHistoryBlob{}, denied: map[string]bool{}}
	for _, entry := range strings.Split(data, "\x00") {
		if entry == "" {
			continue
		}
		metadata, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(metadata)
		if !ok || len(fields) != 4 || !assistantHistoryRevision.MatchString(fields[2]) {
			return nil, errors.New("invalid tree")
		}
		if len(tree.files) >= 4000 {
			return nil, errors.New("limit_exceeded")
		}
		size, sizeErr := strconv.Atoi(fields[3])
		if (fields[0] == "100644" || fields[0] == "100755") && (sizeErr != nil || size < 0 || fields[1] != "blob") {
			return nil, errors.New("invalid blob")
		}
		blob := assistantHistoryBlob{fields[2], fields[0], size}
		tree.files[path] = blob
		if !assistantHistoryVisiblePath(path) || (blob.mode != "100644" && blob.mode != "100755") {
			tree.denied[blob.object] = true
		}
	}
	// Git itself interprets historical ignore patterns in an isolated worktree.
	// Only regular, visible .gitignore blobs are copied; deployment files and its
	// index are never used as the ignore evaluator's working tree.
	directory, err := os.MkdirTemp("", "nixorium-assistant-history-*")
	if err != nil {
		return nil, err
	}
	reader.temporary = append(reader.temporary, directory)
	if _, err = assistantHistoryGit(reader.ctx, directory, nil, 1024, "init", "--quiet", "--template="); err != nil {
		return nil, err
	}
	count, total := 0, 0
	for path, blob := range tree.files {
		if filepath.Base(path) != ".gitignore" || !assistantHistoryVisiblePath(path) || (blob.mode != "100644" && blob.mode != "100755") {
			continue
		}
		if tree.denied[blob.object] {
			return nil, errors.New("protected ignore blob")
		}
		count++
		total += blob.size
		if count > 64 || total > 256*1024 || blob.size > assistantHistoryBlobLimit {
			return nil, errors.New("limit_exceeded")
		}
		content, err := assistantHistoryGit(reader.ctx, reader.repository, nil, assistantHistoryBlobLimit, "cat-file", "blob", blob.object)
		if err != nil {
			return nil, err
		}
		target := filepath.Join(directory, filepath.FromSlash(path))
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return nil, err
		}
		if err = os.WriteFile(target, []byte(content), 0600); err != nil {
			return nil, err
		}
	}
	tree.ignores = directory
	paths := []string{}
	for path := range tree.files {
		if assistantHistoryVisiblePath(path) {
			paths = append(paths, path)
		}
	}
	ignored, err := reader.ignored(tree, paths)
	if err != nil {
		return nil, err
	}
	for path := range ignored {
		tree.denied[tree.files[path].object] = true
	}
	reader.trees[revision] = tree
	return tree, nil
}

func (reader *assistantHistoryReader) ignored(tree *assistantHistoryTree, paths []string) (map[string]bool, error) {
	ignored := map[string]bool{}
	if len(paths) == 0 {
		return ignored, nil
	}
	literal := make([]string, len(paths))
	for index, path := range paths {
		// check-ignore rejects pathspec magic, including the literal magic flag.
		// A relative ./ prefix makes names beginning with ':' ordinary filenames.
		literal[index] = "./" + path
	}
	output, err := assistantHistoryGit(reader.ctx, tree.ignores, []byte(strings.Join(literal, "\x00")+"\x00"), 2*1024*1024,
		"check-ignore", "--no-index", "--stdin", "-z")
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return nil, err
	}
	for _, path := range strings.Split(output, "\x00") {
		if path != "" {
			ignored[strings.TrimPrefix(path, "./")] = true
		}
	}
	return ignored, nil
}

func (reader *assistantHistoryReader) content(path string, blob assistantHistoryBlob) (string, error) {
	if blob.size > assistantHistoryBlobLimit {
		return "", errors.New("limit_exceeded")
	}
	data, err := assistantHistoryGit(reader.ctx, reader.repository, nil, assistantHistoryBlobLimit, "cat-file", "blob", blob.object)
	if err != nil {
		return "", err
	}
	if !assistantText([]byte(data)) {
		return "", errors.New("not_text")
	}
	if filepath.Base(path) == "lab-settings.json" {
		var settings any
		if json.Unmarshal([]byte(data), &settings) != nil {
			return "", errors.New("not_text")
		}
		encoded, err := json.MarshalIndent(redactAssistantSettings(settings), "", "  ")
		if err != nil {
			return "", err
		}
		data = string(encoded)
	}
	return domain.RedactForModel(assistantHistoryContentSecrets.ReplaceAllString(data, `${1}"[WITHHELD]"`)), nil
}

func assistantHistoryClip(text string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit], true
}

func (reader *assistantHistoryReader) changes(older, newer, path string, content bool) ([]map[string]any, bool, error) {
	old := &assistantHistoryTree{files: map[string]assistantHistoryBlob{}, denied: map[string]bool{}}
	var err error
	if older != "" {
		old, err = reader.tree(older)
		if err != nil {
			return nil, false, err
		}
	}
	next, err := reader.tree(newer)
	if err != nil {
		return nil, false, err
	}
	base, err := reader.tree(reader.base)
	if err != nil {
		return nil, false, err
	}
	union := map[string]bool{}
	for name := range old.files {
		union[name] = true
	}
	for name := range next.files {
		union[name] = true
	}
	paths := []string{}
	for name := range union {
		if assistantHistoryVisiblePath(name) {
			paths = append(paths, name)
		}
	}
	sort.Strings(paths)
	blocked := map[string]bool{}
	denied := map[string]bool{}
	for object := range reader.imported {
		denied[object] = true
	}
	for _, tree := range []*assistantHistoryTree{old, next, base} {
		for object := range tree.denied {
			denied[object] = true
		}
		if tree.ignores == "" {
			continue
		}
		ignored, err := reader.ignored(tree, paths)
		if err != nil {
			return nil, false, err
		}
		for name := range ignored {
			blocked[name] = true
			for _, source := range []*assistantHistoryTree{old, next, base} {
				if blob, ok := source.files[name]; ok {
					denied[blob.object] = true
				}
			}
		}
	}
	result := []map[string]any{}
	truncated := false
	for _, name := range paths {
		if path != "" && name != path {
			continue
		}
		before, had := old.files[name]
		after, has := next.files[name]
		baseBlob, inBase := base.files[name]
		regular := func(blob assistantHistoryBlob, exists bool) bool {
			return !exists || (blob.mode == "100644" || blob.mode == "100755")
		}
		if blocked[name] || denied[before.object] || denied[after.object] || !regular(before, had) || !regular(after, has) || !regular(baseBlob, inBase) || (had && has && before.object == after.object && before.mode == after.mode) {
			continue
		}
		if len(result) >= 20 {
			truncated = true
			break
		}
		action := "modify"
		if !had {
			action = "add"
		} else if !has {
			action = "remove"
		}
		change := map[string]any{"path": name, "action": action}
		if content {
			texts := map[string]string{"before": "", "after": ""}
			for _, side := range []struct {
				name   string
				blob   assistantHistoryBlob
				exists bool
			}{{"before", before, had}, {"after", after, has}} {
				if !side.exists {
					continue
				}
				text, err := reader.content(name, side.blob)
				if err != nil {
					if err.Error() == "not_text" || err.Error() == "limit_exceeded" {
						change["contentUnavailable"] = true
						continue
					}
					return nil, false, err
				}
				texts[side.name] = text
			}
			if change["contentUnavailable"] != true {
				diff, cut, err := reader.diff(texts["before"], texts["after"])
				if err != nil {
					return nil, false, err
				}
				change["diff"] = diff
				if cut {
					change["contentTruncated"] = true
					truncated = true
				}
			}
			if had {
				change["beforeMode"] = before.mode
			}
			if has {
				change["afterMode"] = after.mode
			}
		}
		result = append(result, change)
	}
	return result, truncated, nil
}

// AssistantGitHistory exposes only the immutable conversation base and its
// ancestors. Comparison content is sanitized independently at each revision.
func AssistantGitHistory(ctx context.Context, workspace *AssistantWorkspace, call domain.AssistantToolCall) string {
	fail := func(code string) string { return assistantToolResult(map[string]any{"ok": false, "error": code}) }
	if !domain.ValidateAssistantArguments(call.Name, call.Arguments) || call.Name != "git_history" {
		return fail("invalid_arguments")
	}
	var args struct {
		Limit    int
		Revision string
		Path     string
	}
	if json.Unmarshal(call.Arguments, &args) != nil || (args.Revision != "" && !assistantHistoryRevision.MatchString(args.Revision)) {
		return fail("invalid_arguments")
	}
	if args.Path != "" && !assistantHistoryVisiblePath(args.Path) {
		return fail("protected_path")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	reader := &assistantHistoryReader{ctx: ctx, repository: workspace.Conversation.Repository, base: workspace.Conversation.Base, trees: map[string]*assistantHistoryTree{}}
	defer func() {
		for _, path := range reader.temporary {
			os.RemoveAll(path)
		}
	}()
	result, err := reader.execute(args.Limit, args.Revision, args.Path)
	if err != nil {
		if ctx.Err() != nil {
			return fail("timeout")
		}
		return fail("history_unavailable")
	}
	return result
}

func (reader *assistantHistoryReader) execute(limit int, revision, path string) (string, error) {
	if !assistantHistoryRevision.MatchString(reader.base) {
		return "", errors.New("invalid base")
	}
	repository, err := filepath.EvalSymlinks(reader.repository)
	if err != nil {
		return "", err
	}
	for _, part := range strings.Split(filepath.ToSlash(repository), "/") {
		if part == ".git" {
			return "", errors.New("invalid repository")
		}
	}
	top, err := assistantHistoryGit(reader.ctx, repository, nil, 4096, "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(top) != repository {
		return "", errors.New("invalid repository")
	}
	reader.repository = repository
	reader.imported, err = assistantImportedBlobs(reader.ctx, repository, reader.base)
	if err != nil {
		return "", err
	}
	objectType, err := assistantHistoryGit(reader.ctx, repository, nil, 32, "cat-file", "-t", reader.base)
	if err != nil || strings.TrimSpace(objectType) != "commit" {
		return "", errors.New("invalid base")
	}
	if revision != "" {
		objectType, err = assistantHistoryGit(reader.ctx, repository, nil, 32, "cat-file", "-t", revision)
		if err != nil || strings.TrimSpace(objectType) != "commit" {
			return "", errors.New("invalid revision")
		}
		if _, err = assistantHistoryGit(reader.ctx, repository, nil, 1024, "merge-base", "--is-ancestor", revision, reader.base); err != nil {
			return "", errors.New("invalid ancestry")
		}
		changes, truncated, err := reader.changes(revision, reader.base, path, true)
		if err != nil {
			return "", err
		}
		result := map[string]any{"ok": true, "base": reader.base, "revision": revision, "changes": changes, "truncated": truncated}
		return assistantHistoryBoundResult(result, "changes", changes), nil
	}
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 20 {
		return "", errors.New("invalid limit")
	}
	arguments := []string{"--literal-pathspecs", "rev-list", "--max-count=" + strconv.Itoa(limit+1), reader.base, "--"}
	if path != "" {
		arguments = append(arguments, path)
	}
	revisions, err := assistantHistoryGit(reader.ctx, repository, nil, 2048, arguments...)
	if err != nil {
		return "", err
	}
	items := strings.Fields(revisions)
	truncated := len(items) > limit
	if truncated {
		items = items[:limit]
	}
	commits := []map[string]any{}
	for _, item := range items {
		if !assistantHistoryRevision.MatchString(item) {
			return "", errors.New("invalid revision")
		}
		metadata, err := assistantHistoryGit(reader.ctx, repository, nil, 8192, "show", "--no-patch", "--no-notes", "--no-show-signature", "--format=%cI%x00%s%x00%P", item, "--")
		if err != nil {
			return "", err
		}
		fields := strings.Split(strings.TrimSuffix(metadata, "\n"), "\x00")
		if len(fields) != 3 || !assistantText([]byte(fields[1])) {
			return "", errors.New("invalid metadata")
		}
		if _, err := time.Parse(time.RFC3339, fields[0]); err != nil {
			return "", errors.New("invalid date")
		}
		parents := strings.Fields(fields[2])
		parent := ""
		if len(parents) > 0 {
			parent = parents[0]
			if !assistantHistoryRevision.MatchString(parent) {
				return "", errors.New("invalid parent")
			}
		}
		changes, cut, err := reader.changes(parent, item, path, false)
		if err != nil {
			return "", err
		}
		summary := []rune(assistantHistorySummary(fields[1]))
		if len(summary) > 160 {
			summary = summary[:160]
		}
		commits = append(commits, map[string]any{"revision": item, "date": fields[0], "summary": string(summary), "changes": changes, "truncated": cut})
	}
	result := map[string]any{"ok": true, "base": reader.base, "commits": commits, "truncated": truncated}
	return assistantHistoryBoundResult(result, "commits", commits), nil
}

func assistantHistorySummary(text string) string {
	for _, word := range strings.Fields(text) {
		candidate := strings.Trim(word, "\"'`():,;[]{}")
		if domain.AssistantHiddenPath(candidate) || filepath.Base(candidate) == "AGENTS.md" || filepath.Base(candidate) == "CLAUDE.md" {
			return "[WITHHELD]"
		}
	}
	text = assistantHistorySubjectSecrets.ReplaceAllString(text, "[WITHHELD]")
	text = assistantHistorySubjectEmails.ReplaceAllString(text, "[WITHHELD]")
	return domain.RedactForModel(text)
}

var assistantHistorySubjectSecrets = regexp.MustCompile(`(?i)(?:password|passwd|token|secret|credential|api[_-]?key)[\t ]*[:=][\t ]*[^\s,;]+|https?://[^\s]+`)
var assistantHistorySubjectEmails = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
var assistantHistoryContentSecrets = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:password|passwd|token|secret|credential|api[_-]?key)[A-Za-z0-9_.-]*["']?[\t ]*[:=][\t ]*)(?:"(?:\\.|[^"\\])*"|'[^']*'|[^\s,;}]+)`)

func (reader *assistantHistoryReader) diff(before, after string) (string, bool, error) {
	directory, err := os.MkdirTemp("", "nixorium-assistant-history-diff-*")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(directory)
	for name, content := range map[string]string{"before": before, "after": after} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0600); err != nil {
			return "", false, err
		}
	}
	output, err := assistantHistoryGit(reader.ctx, directory, nil, 4096, "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--no-renames", "--no-prefix", "--unified=3", "--", "before", "after")
	var exit *exec.ExitError
	if err != nil && err.Error() == "limit_exceeded" {
		clipped, _ := assistantHistoryClip(output, len(output)-4)
		return clipped, true, nil
	}
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return "", false, err
	}
	return output, false, nil
}

func assistantHistoryBoundResult(result map[string]any, field string, items []map[string]any) string {
	for {
		result[field] = items
		encoded := assistantToolResult(result)
		if len(encoded) <= assistantHistoryOutputLimit {
			return encoded
		}
		result["truncated"] = true
		items = items[:len(items)-1]
	}
}

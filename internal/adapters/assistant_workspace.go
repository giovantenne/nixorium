package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

type AssistantWorkspaceStore struct{ Directory string }
type assistantBaseFile struct {
	size int
	mode string
}
type AssistantWorkspace struct {
	Conversation domain.AssistantConversation
	root         *os.File
	base         map[string]assistantBaseFile
	deleted      map[string]bool
	modes        map[string]bool
}

func (workspace *AssistantWorkspace) Close() error { return workspace.root.Close() }
func assistantDirectory(parent *os.File, name string, create bool) (*os.File, error) {
	if create {
		if err := unix.Mkdirat(int(parent.Fd()), name, 0700); err != nil && !errors.Is(err, unix.EEXIST) {
			return nil, err
		}
	}
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	if telemetrySafeFile(fd, unix.S_IFDIR) != nil {
		unix.Close(fd)
		return nil, errors.New("unsafe Assistant workspace directory")
	}
	return os.NewFile(uintptr(fd), name), nil
}
func (store AssistantWorkspaceStore) conversations(create bool) (*os.File, error) {
	root, err := (AssistantStateStore{Directory: store.Directory}).root()
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return assistantDirectory(root, "conversations", create)
}
func (store AssistantWorkspaceStore) CreateConversation(repository string) (domain.AssistantConversation, error) {
	var conversation domain.AssistantConversation
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository, err := filepath.Abs(repository)
	if err != nil {
		return conversation, err
	}
	status, err := resetGit(ctx, repository, nil, 1024*1024, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return conversation, err
	}
	if status != "" {
		return conversation, errors.New("Commit or discard the deployment changes before using Assistant.")
	}
	base, err := resetGit(ctx, repository, nil, 128, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return conversation, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return conversation, err
	}
	random[6] = (random[6] & 15) | 64
	random[8] = (random[8] & 63) | 128
	id := hex.EncodeToString(random[:])
	id = id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
	conversation = domain.AssistantConversation{ID: id, Repository: repository, Base: strings.TrimSpace(base), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	parent, err := store.conversations(true)
	if err != nil {
		return domain.AssistantConversation{}, err
	}
	defer parent.Close()
	if err = unix.Mkdirat(int(parent.Fd()), id, 0700); err != nil {
		return domain.AssistantConversation{}, err
	}
	root, err := assistantDirectory(parent, id, false)
	if err != nil {
		return domain.AssistantConversation{}, err
	}
	defer root.Close()
	overlay, err := assistantDirectory(root, "overlay", true)
	if err != nil {
		return domain.AssistantConversation{}, err
	}
	overlay.Close()
	metadata, _ := json.Marshal(conversation)
	if err = assistantWriteState(root, "meta.json", metadata); err != nil {
		return domain.AssistantConversation{}, err
	}
	if err = assistantWriteState(root, "deleted.json", []byte("[]\n")); err != nil {
		return domain.AssistantConversation{}, err
	}
	if err = assistantWriteState(root, "transcript.json", []byte("[]\n")); err != nil {
		return domain.AssistantConversation{}, err
	}
	return conversation, nil
}
func (store AssistantWorkspaceStore) Open(id string) (*AssistantWorkspace, error) {
	if !assistantTurnPattern.MatchString(id) {
		return nil, errors.New("invalid Assistant conversation")
	}
	parent, err := store.conversations(false)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	root, err := assistantDirectory(parent, id, false)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			root.Close()
		}
	}()
	data, err := assistantReadState(root, "meta.json")
	if err != nil {
		return nil, err
	}
	var conversation domain.AssistantConversation
	if assistantDecodeStrict(data, &conversation) != nil || conversation.ID != id || !filepath.IsAbs(conversation.Repository) {
		return nil, errors.New("invalid Assistant conversation metadata")
	}
	if len(conversation.Base) != 40 && len(conversation.Base) != 64 {
		return nil, errors.New("invalid Assistant base revision")
	}
	if _, err = hex.DecodeString(conversation.Base); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	head, err := resetGit(ctx, conversation.Repository, nil, 128, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(head) != conversation.Base {
		return nil, errors.New("The deployment changed since this chat started. Press Ctrl+N to start a new chat.")
	}
	tree, err := resetGit(ctx, conversation.Repository, nil, 8*1024*1024, "ls-tree", "-r", "-l", "-z", conversation.Base)
	if err != nil {
		return nil, err
	}
	workspace := &AssistantWorkspace{Conversation: conversation, root: root, base: map[string]assistantBaseFile{}, deleted: map[string]bool{}, modes: map[string]bool{}}
	for _, entry := range strings.Split(tree, "\x00") {
		if entry == "" {
			continue
		}
		info, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(info)
		if !ok || len(fields) != 4 {
			return nil, errors.New("invalid Assistant Git tree")
		}
		if !domain.ValidAssistantPath(path) || domain.AssistantHiddenPath(path) {
			continue
		}
		if fields[0] != "100644" && fields[0] != "100755" {
			continue
		}
		size, err := strconv.Atoi(fields[3])
		if err != nil || size < 0 {
			return nil, errors.New("invalid Assistant Git blob")
		}
		workspace.base[path] = assistantBaseFile{size: size, mode: fields[0]}
	}
	data, err = assistantReadStateLimit(root, "deleted.json", 128*1024)
	if err != nil {
		return nil, err
	}
	var deleted []string
	if json.Unmarshal(data, &deleted) != nil || len(deleted) > 200 {
		return nil, errors.New("invalid Assistant deleted files")
	}
	for _, path := range deleted {
		if !domain.ValidAssistantPath(path) || domain.AssistantHiddenPath(path) || domain.AssistantReadOnlyPath(path) {
			return nil, errors.New("invalid Assistant deleted path")
		}
		workspace.deleted[path] = true
	}
	data, err = assistantReadStateLimit(root, "modes.json", 128*1024)
	if err != nil && !errors.Is(err, unix.ENOENT) {
		return nil, err
	}
	if err == nil {
		if json.Unmarshal(data, &workspace.modes) != nil || workspace.modes == nil || len(workspace.modes) > 200 {
			return nil, errors.New("invalid Assistant modes")
		}
		for path := range workspace.modes {
			if !domain.ValidAssistantPath(path) || domain.AssistantHiddenPath(path) || domain.AssistantReadOnlyPath(path) {
				return nil, errors.New("invalid Assistant mode path")
			}
		}
	}
	success = true
	return workspace, nil
}
func (workspace *AssistantWorkspace) overlay() (*os.File, error) {
	return assistantDirectory(workspace.root, "overlay", false)
}
func assistantParent(root *os.File, path string, create bool) (*os.File, string, error) {
	fd, err := unix.Dup(int(root.Fd()))
	if err != nil {
		return nil, "", err
	}
	parent := os.NewFile(uintptr(fd), "overlay")
	parts := strings.Split(path, "/")
	for _, part := range parts[:len(parts)-1] {
		next, err := assistantDirectory(parent, part, create)
		parent.Close()
		if err != nil {
			return nil, "", err
		}
		parent = next
	}
	return parent, parts[len(parts)-1], nil
}
func assistantText(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, r := range string(data) {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return false
		}
	}
	return true
}
func (workspace *AssistantWorkspace) raw(path string) ([]byte, error) {
	if !domain.ValidAssistantPath(path) || domain.AssistantHiddenPath(path) {
		return nil, fs.ErrPermission
	}
	if workspace.deleted[path] {
		return nil, fs.ErrNotExist
	}
	overlay, err := workspace.overlay()
	if err != nil {
		return nil, err
	}
	defer overlay.Close()
	parent, name, err := assistantParent(overlay, path, false)
	if err == nil {
		defer parent.Close()
		fd, openErr := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if openErr == nil {
			file := os.NewFile(uintptr(fd), name)
			defer file.Close()
			if telemetrySafeFile(fd, unix.S_IFREG) != nil {
				return nil, fs.ErrPermission
			}
			info, err := file.Stat()
			if err != nil || info.Size() > 512*1024 {
				return nil, errors.New("limit_exceeded")
			}
			return io.ReadAll(io.LimitReader(file, 512*1024+1))
		}
		if !errors.Is(openErr, unix.ENOENT) {
			return nil, fs.ErrPermission
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return nil, fs.ErrPermission
	}
	base, ok := workspace.base[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	content, err := resetGit(ctx, workspace.Conversation.Repository, nil, base.size, "cat-file", "blob", workspace.Conversation.Base+":"+path)
	return []byte(content), err
}
func redactAssistantSettings(value any) any {
	switch data := value.(type) {
	case map[string]any:
		for key, child := range data {
			lower := strings.ToLower(key)
			withheld := false
			for _, word := range []string{"password", "secret", "token", "credential"} {
				if strings.Contains(lower, word) {
					withheld = true
				}
			}
			if withheld {
				data[key] = "[WITHHELD]"
			} else {
				data[key] = redactAssistantSettings(child)
			}
		}
	case []any:
		for i, child := range data {
			data[i] = redactAssistantSettings(child)
		}
	}
	return value
}
func (workspace *AssistantWorkspace) Read(path string) ([]byte, error) {
	data, err := workspace.raw(path)
	if err != nil {
		return nil, err
	}
	if !assistantText(data) {
		return nil, errors.New("not_text")
	}
	if filepath.Base(path) == "lab-settings.json" {
		var settings any
		if json.Unmarshal(data, &settings) != nil {
			return nil, errors.New("not_text")
		}
		data, err = json.MarshalIndent(redactAssistantSettings(settings), "", "  ")
		if err != nil {
			return nil, err
		}
	}
	return []byte(domain.RedactForModel(string(data))), nil
}

type AssistantFileEntry struct {
	Path string `json:"path"`
	Type string `json:"type"`
	Size int    `json:"size"`
}

func (workspace *AssistantWorkspace) Entries() ([]AssistantFileEntry, error) {
	files := map[string]int{}
	for path, base := range workspace.base {
		if !workspace.deleted[path] {
			files[path] = base.size
		}
	}
	overlay, err := workspace.overlay()
	if err != nil {
		return nil, err
	}
	defer overlay.Close()
	var walk func(*os.File, string) error
	walk = func(dir *os.File, prefix string) error {
		entries, err := dir.ReadDir(-1)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			path := prefix + entry.Name()
			if !domain.ValidAssistantPath(path) || domain.AssistantHiddenPath(path) {
				continue
			}
			if entry.IsDir() {
				child, err := assistantDirectory(dir, entry.Name(), false)
				if err != nil {
					return err
				}
				err = walk(child, path+"/")
				child.Close()
				if err != nil {
					return err
				}
				continue
			}
			fd, err := unix.Openat(int(dir.Fd()), entry.Name(), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
			if err != nil {
				return fs.ErrPermission
			}
			file := os.NewFile(uintptr(fd), path)
			safe := telemetrySafeFile(fd, unix.S_IFREG)
			info, err := file.Stat()
			file.Close()
			if safe != nil || err != nil {
				return fs.ErrPermission
			}
			if !workspace.deleted[path] {
				files[path] = int(info.Size())
			}
		}
		return nil
	}
	if err = walk(overlay, ""); err != nil {
		return nil, err
	}
	result := make([]AssistantFileEntry, 0, len(files))
	for path, size := range files {
		result = append(result, AssistantFileEntry{path, "file", size})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result, nil
}
func (workspace *AssistantWorkspace) saveDeleted() error {
	paths := []string{}
	for path := range workspace.deleted {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	data, _ := json.Marshal(paths)
	return assistantWriteState(workspace.root, "deleted.json", data)
}
func (workspace *AssistantWorkspace) Write(path, content string, executable bool) error {
	if !domain.ValidAssistantPath(path) || domain.AssistantHiddenPath(path) || domain.AssistantReadOnlyPath(path) {
		return fs.ErrPermission
	}
	if len(content) > 512*1024 {
		return errors.New("limit_exceeded")
	}
	if !assistantText([]byte(content)) {
		return errors.New("not_text")
	}
	entries, err := workspace.Entries()
	if err != nil {
		return err
	}
	total, count := len(content), 1
	for deletedPath := range workspace.deleted {
		if deletedPath != path {
			count++
		}
	}
	// Conversation limits apply to the overlay, not the unchanged base files.
	overlay, err := workspace.overlay()
	if err != nil {
		return err
	}
	defer overlay.Close()
	for _, entry := range entries {
		parent, name, err := assistantParent(overlay, entry.Path, false)
		if err != nil {
			if errors.Is(err, unix.ENOENT) {
				continue
			}
			return err
		}
		var stat unix.Stat_t
		err = unix.Fstatat(int(parent.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW)
		parent.Close()
		if err == nil {
			if entry.Path != path {
				count++
				total += int(stat.Size)
			}
		} else if !errors.Is(err, unix.ENOENT) {
			return err
		}
	}
	if count > 200 || total > 4*1024*1024 {
		return errors.New("limit_exceeded")
	}
	parent, name, err := assistantParent(overlay, path, true)
	if err != nil {
		return err
	}
	defer parent.Close()
	if err = assistantWriteState(parent, name, []byte(content)); err != nil {
		return err
	}
	// Executability is metadata; private overlay files always remain 0600.
	workspace.modes[path] = executable
	data, _ := json.Marshal(workspace.modes)
	if err = assistantWriteState(workspace.root, "modes.json", data); err != nil {
		return err
	}
	delete(workspace.deleted, path)
	return workspace.saveDeleted()
}
func (workspace *AssistantWorkspace) Delete(path string) error {
	if !domain.ValidAssistantPath(path) || domain.AssistantHiddenPath(path) || domain.AssistantReadOnlyPath(path) {
		return fs.ErrPermission
	}
	if _, err := workspace.raw(path); err != nil {
		return err
	}
	if len(workspace.deleted)+len(workspace.modes) >= 200 && !workspace.deleted[path] {
		if _, alreadyWritten := workspace.modes[path]; !alreadyWritten {
			return errors.New("limit_exceeded")
		}
	}
	if len(workspace.deleted) >= 200 && !workspace.deleted[path] {
		return errors.New("limit_exceeded")
	}
	overlay, err := workspace.overlay()
	if err != nil {
		return err
	}
	defer overlay.Close()
	parent, name, err := assistantParent(overlay, path, false)
	if err == nil {
		defer parent.Close()
		if err = unix.Unlinkat(int(parent.Fd()), name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
			return err
		}
	} else if !errors.Is(err, unix.ENOENT) {
		return err
	}
	workspace.deleted[path] = true
	delete(workspace.modes, path)
	data, _ := json.Marshal(workspace.modes)
	if err = assistantWriteState(workspace.root, "modes.json", data); err != nil {
		return err
	}
	return workspace.saveDeleted()
}

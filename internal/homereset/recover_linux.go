package homereset

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/giovantenne/nixorium/internal/classroomview"
	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

// Recovering a student's files from the published snapshots. The controller
// runs the helper as root over SSH with one JSON request on its standard
// input and reads one JSON answer from its standard output. Root only reads
// the read-only snapshots, opening every name without following links; the
// files are written into the student's home by a child process running as
// the student, so nothing root writes can be redirected by the student.

// RecoverWriteArgument starts the helper as the writing child.
const RecoverWriteArgument = "--write-recovered"

const (
	recoverRequestBytes = 64 * 1024
	recoverHeaderBytes  = 1 << 20
	recoverWriteTimeout = 15 * time.Minute
)

// recoverFailure is a refusal the teacher reads.
type recoverFailure struct{ code, message string }

func (f recoverFailure) Error() string { return f.message }

func refuse(code, message string) error { return recoverFailure{code, message} }

// RecoverConfigured answers one request read from input. It reports through
// the answer, returning an error only when the answer cannot be written.
func RecoverConfigured(input io.Reader, output io.Writer) error {
	answer := recoverAnswer(input)
	answer.SchemaVersion = domain.FileRecoveryProtocolVersion
	data, err := json.Marshal(answer)
	if err != nil {
		return err
	}
	_, err = output.Write(append(data, '\n'))
	return err
}

func recoverAnswer(input io.Reader) domain.FileRecoveryHelperAnswer {
	answer, err := recoverRequest(input)
	if err == nil {
		return answer
	}
	var failure recoverFailure
	if errors.As(err, &failure) {
		return domain.FileRecoveryHelperAnswer{Error: failure.message, Code: failure.code}
	}
	return domain.FileRecoveryHelperAnswer{Error: "The student computer could not read its saved sessions: " + err.Error(), Code: domain.FileRecoveryCodeFailed}
}

func recoverRequest(input io.Reader) (domain.FileRecoveryHelperAnswer, error) {
	if os.Geteuid() != 0 {
		return domain.FileRecoveryHelperAnswer{}, errors.New("recovery requires root")
	}
	data, err := io.ReadAll(io.LimitReader(input, recoverRequestBytes+1))
	if err != nil {
		return domain.FileRecoveryHelperAnswer{}, err
	}
	if len(data) > recoverRequestBytes {
		return domain.FileRecoveryHelperAnswer{}, refuse(domain.FileRecoveryCodeInvalid, "The request is too large.")
	}
	var request domain.FileRecoveryHelperRequest
	if err := decodeStrict(data, &request); err != nil || request.SchemaVersion != domain.FileRecoveryProtocolVersion {
		return domain.FileRecoveryHelperAnswer{}, refuse(domain.FileRecoveryCodeInvalid, "The request is not understood; update the student computer.")
	}
	config, err := loadConfigured()
	if err != nil {
		return domain.FileRecoveryHelperAnswer{}, err
	}
	if err := trustedDirectory(snapshotRoot); err != nil {
		return domain.FileRecoveryHelperAnswer{}, err
	}
	switch request.Operation {
	case domain.FileRecoveryHelperSessions:
		sessions, err := recoverySessions(publishedSnapshots)
		return domain.FileRecoveryHelperAnswer{Sessions: sessions}, err
	case domain.FileRecoveryHelperList:
		root, err := sessionDirectory(publishedSnapshots, request.Session)
		if err != nil {
			return domain.FileRecoveryHelperAnswer{}, err
		}
		listing, err := recoveryList(root, request.Session, request.Path)
		return domain.FileRecoveryHelperAnswer{Listing: listing}, err
	case domain.FileRecoveryHelperRestore:
		root, err := sessionDirectory(publishedSnapshots, request.Session)
		if err != nil {
			return domain.FileRecoveryHelperAnswer{}, err
		}
		restored, err := recoverAsStudent(config, root, request.Paths)
		return domain.FileRecoveryHelperAnswer{Restored: restored}, err
	default:
		return domain.FileRecoveryHelperAnswer{}, refuse(domain.FileRecoveryCodeInvalid, "The request is not understood; update the student computer.")
	}
}

// loadConfigured reads the reset configuration: an immutable store file
// below the root-controlled /etc tree.
func loadConfigured() (Config, error) {
	var config Config
	if err := trustedDirectory("/etc"); err != nil {
		return config, err
	}
	const configPath = "/etc/nixorium-workspace-reset.json"
	info, err := os.Lstat(configPath)
	if err != nil {
		return config, err
	}
	if info.Sys().(*syscall.Stat_t).Uid != 0 {
		return config, errors.New("reset configuration is not root-owned")
	}
	resolved, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		return config, err
	}
	if err := immutableStoreFile(resolved); err != nil {
		return config, err
	}
	data, err := readBounded(resolved, 128*1024)
	if err != nil {
		return config, err
	}
	err = decodeStrict(data, &config)
	return config, err
}

// recoverySessions lists the published snapshots, most recent first, with
// the time each was taken. No published folder means no saved session.
func recoverySessions(directory string) ([]domain.FileRecoverySession, error) {
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.FileRecoverySession{}, nil
	} else if err != nil {
		return nil, err
	}
	sessions := []domain.FileRecoverySession{}
	for _, entry := range entries {
		number, valid := snapshotNumber(entry.Name())
		if !valid || !entry.IsDir() {
			continue
		}
		var stat unix.Statx_t
		if err := unix.Statx(unix.AT_FDCWD, filepath.Join(directory, entry.Name()), unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BTIME|unix.STATX_MTIME, &stat); err != nil {
			return nil, err
		}
		saved := time.Unix(stat.Mtime.Sec, int64(stat.Mtime.Nsec))
		if stat.Mask&unix.STATX_BTIME != 0 {
			saved = time.Unix(stat.Btime.Sec, int64(stat.Btime.Nsec))
		}
		sessions = append(sessions, domain.FileRecoverySession{Number: number, SavedAt: saved.UTC()})
	}
	slices.SortFunc(sessions, func(a, b domain.FileRecoverySession) int { return a.Number - b.Number })
	return sessions, nil
}

// sessionDirectory names a published snapshot that exists.
func sessionDirectory(directory string, session int) (string, error) {
	if _, valid := snapshotNumber("snapshot-" + strconv.Itoa(session)); !valid {
		return "", refuse(domain.FileRecoveryCodeInvalid, "The saved session is not valid.")
	}
	name := filepath.Join(directory, "snapshot-"+strconv.Itoa(session))
	info, err := os.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return "", refuse(domain.FileRecoveryCodeNoSession, "This saved session no longer exists; the computer may have restarted. Choose the session again.")
	} else if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("the saved session is not a folder")
	}
	return name, nil
}

// openBelow opens a plain relative path below root without following any
// link; "" opens root itself.
func openBelow(root, relative string, dir bool) (*os.File, error) {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	if relative != "" {
		parts := strings.Split(relative, "/")
		for index, part := range parts {
			flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC | unix.O_NONBLOCK
			if index < len(parts)-1 || dir {
				flags |= unix.O_DIRECTORY
			}
			next, err := unix.Openat(fd, part, flags, 0)
			unix.Close(fd)
			if err != nil {
				return nil, err
			}
			fd = next
		}
	}
	return os.NewFile(uintptr(fd), relative), nil
}

// shownName is a name a listing or a recovery includes: not hidden, and
// without control characters.
func shownName(name string) bool {
	return !strings.HasPrefix(name, ".") && domain.ValidFileRecoveryPath(name) == nil && !strings.Contains(name, "/")
}

func missing(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.ENOTDIR) || errors.Is(err, unix.ELOOP)
}

// recoveryList lists one folder of a saved session: folders first, then
// files, each by name. Hidden names, links and special files are left out.
func recoveryList(root string, session int, relative string) (*domain.FileRecoveryListing, error) {
	if err := domain.ValidFileRecoveryPath(relative); err != nil {
		return nil, refuse(domain.FileRecoveryCodeInvalid, "The folder name is not valid.")
	}
	folder, err := openBelow(root, relative, true)
	if missing(err) {
		return nil, refuse(domain.FileRecoveryCodeNotFound, "This folder is not in the saved session.")
	} else if err != nil {
		return nil, err
	}
	defer folder.Close()
	listing := &domain.FileRecoveryListing{Session: session, Path: relative, Entries: []domain.FileRecoveryEntry{}}
	err = walkEntries(folder, func(name string) error {
		if !shownName(name) {
			return nil
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(int(folder.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		entry := domain.FileRecoveryEntry{Name: name, Modified: time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec).UTC()}
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			entry.Dir = true
		case unix.S_IFREG:
			entry.Size = stat.Size
		default:
			return nil
		}
		if len(listing.Entries) == domain.FileRecoveryMaxListEntries {
			listing.Truncated = true
			return nil
		}
		listing.Entries = append(listing.Entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(listing.Entries, func(a, b domain.FileRecoveryEntry) int {
		if a.Dir != b.Dir {
			if a.Dir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return listing, nil
}

// recoverySource is the saved file of a file entry, relative to the
// session root.
type recoverySource struct {
	entry  int
	source string
}

// recoveryTree lists the chosen files and folders as a tree to write:
// folders before their contents, each chosen item at the top under its own
// name ("name (2)" when two chosen items share a name). Hidden names, links
// and special files inside chosen folders are left out.
func recoveryTree(root string, paths []string) ([]classroomview.FileEntry, []recoverySource, error) {
	if err := domain.ValidFileRecoveryPaths(paths); err != nil {
		return nil, nil, refuse(domain.FileRecoveryCodeInvalid, "The choice is not valid: "+err.Error()+".")
	}
	tooLarge := refuse(domain.FileRecoveryCodeTooLarge, fmt.Sprintf("The chosen files are more than %d or larger than %d MB; choose fewer.", classroomview.MaxFileEntries, classroomview.MaxFileBytes>>20))
	entries := []classroomview.FileEntry{}
	sources := []recoverySource{}
	used := map[string]bool{}
	var total int64
	var add func(parent *os.File, name, source, target string, depth int) error
	add = func(parent *os.File, name, source, target string, depth int) error {
		var stat unix.Stat_t
		if err := unix.Fstatat(int(parent.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		mode := stat.Mode & unix.S_IFMT
		if mode != unix.S_IFDIR && mode != unix.S_IFREG {
			if depth == 0 {
				return refuse(domain.FileRecoveryCodeNotFound, "“"+source+"” is a link or a special file, not a file or folder.")
			}
			return nil
		}
		if len(entries) == classroomview.MaxFileEntries || depth >= 32 {
			return tooLarge
		}
		if mode == unix.S_IFREG {
			total += stat.Size
			if total > classroomview.MaxFileBytes {
				return tooLarge
			}
			entries = append(entries, classroomview.FileEntry{Path: target, Size: stat.Size})
			sources = append(sources, recoverySource{entry: len(entries) - 1, source: source})
			return nil
		}
		entries = append(entries, classroomview.FileEntry{Path: target, Dir: true})
		fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		folder := os.NewFile(uintptr(fd), source)
		defer folder.Close()
		names := []string{}
		if err := walkEntries(folder, func(child string) error {
			if shownName(child) {
				names = append(names, child)
			}
			return nil
		}); err != nil {
			return err
		}
		slices.Sort(names)
		for _, child := range names {
			if err := add(folder, child, source+"/"+child, target+"/"+child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	for _, chosen := range paths {
		folder := path.Dir(chosen)
		if folder == "." {
			folder = ""
		}
		parent, err := openBelow(root, folder, true)
		if missing(err) {
			return nil, nil, refuse(domain.FileRecoveryCodeNotFound, "“"+chosen+"” is not in the saved session.")
		} else if err != nil {
			return nil, nil, err
		}
		name := path.Base(chosen)
		err = add(parent, name, chosen, uniqueName(name, used), 0)
		parent.Close()
		if missing(err) {
			return nil, nil, refuse(domain.FileRecoveryCodeNotFound, "“"+chosen+"” is not in the saved session.")
		} else if err != nil {
			return nil, nil, err
		}
	}
	if err := classroomview.ValidateFileEntries(entries); err != nil {
		return nil, nil, refuse(domain.FileRecoveryCodeInvalid, "The chosen files cannot be recovered: "+err.Error()+".")
	}
	return entries, sources, nil
}

// uniqueName gives a top-level name not used yet: "name (2).ext" and so on.
func uniqueName(name string, used map[string]bool) string {
	extension := path.Ext(name)
	if extension == name {
		extension = ""
	}
	base := strings.TrimSuffix(name, extension)
	candidate := name
	for attempt := 2; used[candidate]; attempt++ {
		candidate = fmt.Sprintf("%s (%d)%s", base, attempt, extension)
	}
	used[candidate] = true
	return candidate
}

// recoveryHeader starts the stream to the writing child.
type recoveryHeader struct {
	Entries []classroomview.FileEntry `json:"entries"`
}

// streamRecovery writes the header and then each file's bytes in order.
func streamRecovery(output io.Writer, root string, entries []classroomview.FileEntry, sources []recoverySource) error {
	header, err := json.Marshal(recoveryHeader{Entries: entries})
	if err != nil {
		return err
	}
	if len(header) > recoverHeaderBytes {
		return refuse(domain.FileRecoveryCodeTooLarge, "Too many files are chosen; choose fewer.")
	}
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(header)))
	if _, err := output.Write(append(length[:], header...)); err != nil {
		return err
	}
	for _, source := range sources {
		size := entries[source.entry].Size
		if size == 0 {
			continue
		}
		file, err := openBelow(root, source.source, false)
		if err != nil {
			return err
		}
		copied, err := io.CopyN(output, file, size)
		file.Close()
		if err != nil || copied != size {
			return fmt.Errorf("could not read %s completely", source.source)
		}
	}
	return nil
}

// recoverAsStudent lists the chosen files and streams them to a child that
// writes them as the student.
func recoverAsStudent(config Config, root string, paths []string) (*domain.FileRecoveryRestored, error) {
	entries, sources, err := recoveryTree(root, paths)
	if err != nil {
		return nil, err
	}
	account, err := user.Lookup(config.User)
	if err != nil {
		return nil, err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil || uid == 0 {
		return nil, errors.New("the student account is not valid")
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil || gid == 0 {
		return nil, errors.New("the student account is not valid")
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), recoverWriteTimeout)
	defer cancel()
	child := exec.CommandContext(ctx, executable, RecoverWriteArgument)
	child.Dir = "/"
	child.Env = []string{"HOME=" + account.HomeDir, "LANG=C", "LC_ALL=C", "PATH=/var/empty"}
	child.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: []uint32{}}}
	child.WaitDelay = 10 * time.Second
	stdin, err := child.StdinPipe()
	if err != nil {
		return nil, err
	}
	answer := &limitedBuffer{limit: domain.FileRecoveryMaxAnswerBytes}
	child.Stdout = answer
	child.Stderr = io.Discard
	if err := child.Start(); err != nil {
		return nil, err
	}
	streamErr := streamRecovery(stdin, root, entries, sources)
	stdin.Close()
	waitErr := child.Wait()
	if streamErr != nil {
		var failure recoverFailure
		if errors.As(streamErr, &failure) {
			return nil, streamErr
		}
	}
	var reply domain.FileRecoveryHelperAnswer
	if err := decodeStrict(answer.Bytes(), &reply); err != nil {
		if streamErr != nil {
			return nil, streamErr
		}
		if waitErr != nil {
			return nil, fmt.Errorf("writing as the student failed: %w", waitErr)
		}
		return nil, errors.New("writing as the student gave no answer")
	}
	if reply.Error != "" || reply.Restored == nil {
		code := reply.Code
		if code == "" {
			code = domain.FileRecoveryCodeFailed
		}
		return nil, refuse(code, reply.Error)
	}
	if streamErr != nil {
		return nil, streamErr
	}
	return reply.Restored, nil
}

// limitedBuffer keeps at most limit bytes and fails beyond.
type limitedBuffer struct {
	limit int
	data  []byte
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if len(b.data)+len(data) > b.limit {
		return 0, errors.New("answer too large")
	}
	b.data = append(b.data, data...)
	return len(data), nil
}

func (b *limitedBuffer) Bytes() []byte { return b.data }

// WriteRecovered is the writing child: it runs as the student, reads the
// stream from input and writes the files into "Recovered files" on the
// student's desktop, then answers on output.
func WriteRecovered(input io.Reader, output io.Writer) error {
	answer := domain.FileRecoveryHelperAnswer{SchemaVersion: domain.FileRecoveryProtocolVersion}
	restored, err := writeRecovered(input)
	if err != nil {
		answer.Error = "The files could not be written on the student's desktop: " + err.Error()
		answer.Code = domain.FileRecoveryCodeFailed
	} else {
		answer.Restored = restored
	}
	data, err := json.Marshal(answer)
	if err != nil {
		return err
	}
	_, err = output.Write(append(data, '\n'))
	return err
}

func writeRecovered(input io.Reader) (*domain.FileRecoveryRestored, error) {
	if os.Geteuid() == 0 {
		return nil, errors.New("writing runs as the student, not as root")
	}
	account, err := user.LookupId(strconv.Itoa(os.Geteuid()))
	if err != nil {
		return nil, err
	}
	return writeRecoveredInto(input, account.HomeDir)
}

func writeRecoveredInto(input io.Reader, home string) (*domain.FileRecoveryRestored, error) {
	reader := bufio.NewReaderSize(input, 1<<20)
	var length [4]byte
	if _, err := io.ReadFull(reader, length[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(length[:])
	if size == 0 || size > recoverHeaderBytes {
		return nil, errors.New("the list of files is not valid")
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, err
	}
	var header recoveryHeader
	if err := decodeStrict(data, &header); err != nil {
		return nil, err
	}
	desktop, err := classroomview.DesktopDirectory(home)
	if err != nil {
		return nil, err
	}
	writer, err := classroomview.NewStagedTreeWriter(filepath.Join(desktop, domain.FileRecoveryFolder), filepath.Join(home, ".local", "share", "nixorium-recovery"), header.Entries)
	if err != nil {
		return nil, err
	}
	defer writer.Abort()
	buffer := make([]byte, classroomview.MaxChunkBytes)
	restored := &domain.FileRecoveryRestored{}
	for index, entry := range header.Entries {
		if entry.Dir {
			continue
		}
		restored.Files++
		restored.Bytes += entry.Size
		for remaining := entry.Size; remaining > 0; {
			chunk := buffer[:min(int64(len(buffer)), remaining)]
			if _, err := io.ReadFull(reader, chunk); err != nil {
				return nil, errors.New("some files did not arrive completely")
			}
			if err := writer.Write(index, chunk); err != nil {
				return nil, err
			}
			remaining -= int64(len(chunk))
		}
	}
	placed, err := writer.Finish()
	if err != nil {
		return nil, err
	}
	relative, err := filepath.Rel(home, filepath.Join(desktop, domain.FileRecoveryFolder))
	if err != nil {
		relative = domain.FileRecoveryFolder
	}
	restored.Folder = filepath.ToSlash(relative)
	restored.Placed = placed
	return restored, nil
}

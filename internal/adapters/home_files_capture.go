package adapters

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

var homeCaptureID = regexp.MustCompile(`^[0-9a-f]{32}$`)

type homeCaptureRecord struct {
	Repository string                        `json:"repository"`
	Report     domain.HomeFilesCaptureReport `json:"report"`
	CreatedAt  time.Time                     `json:"createdAt"`
}

func discardHomeStage(root *os.File, id string, stage *os.File) error {
	if !homeCaptureID.MatchString(id) {
		return errors.New("invalid capture identifier")
	}
	var held, current unix.Stat_t
	if err := unix.Fstat(int(stage.Fd()), &held); err != nil {
		return err
	}
	if err := unix.Fstatat(int(root.Fd()), id, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if held.Dev != current.Dev || held.Ino != current.Ino || current.Mode&unix.S_IFMT != unix.S_IFDIR || current.Uid != uint32(os.Geteuid()) || current.Mode&0777 != 0700 {
		return domain.ErrHomeFilesConflict
	}
	if _, err := stage.Seek(0, 0); err != nil {
		return err
	}
	names, err := stage.ReadDir(domain.HomeFilesMaxEntries + 2)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(names) > domain.HomeFilesMaxEntries+1 {
		return errors.New("capture cleanup exceeds entry limit")
	}
	for _, entry := range names {
		if entry.Name() != "capture.json" && !homeFilesHashPattern.MatchString(entry.Name()) {
			return errors.New("capture contains unexpected cleanup data")
		}
		_, info, err := readWorkspaceFile(stage, entry.Name(), domain.HomeFilesMaxFileBytes)
		if err != nil || info.Nlink != 1 || info.Uid != uint32(os.Geteuid()) || info.Mode&0777 != 0600 {
			return errors.New("capture cleanup cannot confirm file ownership")
		}
	}
	for _, entry := range names {
		if err := unix.Unlinkat(int(stage.Fd()), entry.Name(), 0); err != nil {
			return err
		}
	}
	if err := stage.Sync(); err != nil {
		return err
	}
	if err := unix.Unlinkat(int(root.Fd()), id, unix.AT_REMOVEDIR); err != nil {
		return err
	}
	return root.Sync()
}

func (Local) DiscardHomeFilesCapture(ctx context.Context, repository, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !homeCaptureID.MatchString(id) {
		return errors.New("invalid capture identifier")
	}
	root, err := openHomeCaptures()
	if err != nil {
		return err
	}
	defer root.Close()
	stage, err := assistantDirectory(root, id, false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer stage.Close()
	data, err := assistantReadStateLimit(stage, "capture.json", domain.HomeFilesMaxBytes)
	if err == nil {
		var record homeCaptureRecord
		if assistantDecodeStrict(data, &record) != nil || record.Repository != repository {
			return domain.ErrHomeFilesConflict
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return discardHomeStage(root, id, stage)
}

func openHomeCaptures() (*os.File, error) {
	state, err := userStateRoot()
	if err != nil {
		return nil, err
	}
	// Reuse the confined private state ancestor creation, then a separate
	// product-owned namespace; captures never live in the deployment repository.
	support, err := openSupportDirectory(state)
	if err != nil {
		return nil, err
	}
	support.Close()
	fd, err := unix.Openat2(unix.AT_FDCWD, filepath.Join(state, "nixorium"), &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_NO_SYMLINKS})
	if err != nil {
		return nil, err
	}
	product := os.NewFile(uintptr(fd), filepath.Join(state, "nixorium"))
	defer product.Close()
	return assistantDirectory(product, "home-files-captures", true)
}

func (Local) CaptureHomeFiles(ctx context.Context, repository, sourcePath, destination string) (report domain.HomeFilesCaptureReport, err error) {
	repository, err = filepath.Abs(repository)
	if err != nil {
		return report, err
	}
	if strings.HasPrefix(sourcePath, "~/") {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return report, homeErr
		}
		sourcePath = filepath.Join(home, sourcePath[2:])
	}
	sourcePath, err = filepath.Abs(sourcePath)
	if err != nil {
		return report, err
	}
	if domain.HomeFilesSensitivePath(filepath.ToSlash(sourcePath)) {
		return report, errors.New("credential and agent-state sources cannot be imported")
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, sourcePath, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK, Resolve: unix.RESOLVE_NO_SYMLINKS})
	if err != nil {
		return report, errors.New("choose a regular file or folder without symbolic-link ancestors")
	}
	source := os.NewFile(uintptr(fd), sourcePath)
	defer source.Close()
	var initial unix.Stat_t
	if err = unix.Fstat(fd, &initial); err != nil {
		return report, err
	}
	isDirectory := initial.Mode&unix.S_IFMT == unix.S_IFDIR
	if !domain.ValidHomeFilesDestination(destination, isDirectory) {
		return report, errors.New("destination is invalid, protected or a startup hook")
	}
	root, err := openHomeCaptures()
	if err != nil {
		return report, err
	}
	defer root.Close()
	if err = unix.Flock(int(root.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return report, errors.New("another initial-file capture is running; retry after it finishes")
	}
	retained, err := root.ReadDir(257)
	if err != nil && !errors.Is(err, io.EOF) {
		return report, err
	}
	if len(retained) > 256 {
		return report, errors.New("capture storage needs inspection; stop and collect a support report")
	}
	live := 0
	for _, entry := range retained {
		if !homeCaptureID.MatchString(entry.Name()) {
			return report, errors.New("capture storage contains unexpected entries")
		}
		stage, err := assistantDirectory(root, entry.Name(), false)
		if err != nil {
			return report, err
		}
		data, err := assistantReadStateLimit(stage, "capture.json", domain.HomeFilesMaxBytes)
		var record homeCaptureRecord
		valid := err == nil && assistantDecodeStrict(data, &record) == nil && !record.CreatedAt.IsZero()
		if valid && time.Since(record.CreatedAt) > 24*time.Hour {
			err = discardHomeStage(root, entry.Name(), stage)
		} else {
			live++
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		}
		stage.Close()
		if err != nil {
			return report, errors.New("capture storage needs cleanup; retry Initial files or collect a support report")
		}
	}
	if live >= 16 {
		return report, errors.New("16 private captures are retained; cancel an earlier Initial files draft or discard its capture before choosing another source")
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return report, err
	}
	id := hex.EncodeToString(nonce[:])
	if err = unix.Mkdirat(int(root.Fd()), id, 0700); err != nil {
		return report, err
	}
	stage, err := assistantDirectory(root, id, false)
	if err != nil {
		return report, err
	}
	defer stage.Close()
	complete := false
	defer func() {
		if !complete {
			_ = discardHomeStage(root, id, stage)
		}
	}()
	report = domain.HomeFilesCaptureReport{SchemaVersion: 1, State: "captured", CaptureID: id, Destination: destination, Files: []domain.HomeFile{}, Directories: []string{}, Entries: []domain.HomeFilesEntry{}, Issues: []domain.ValidationIssue{}, Message: "Captured a private local copy. Original file changes are not watched. Review destinations and explicit executability before saving; materials enter Git, the Nix store and cache."}
	total := int64(0)
	var walk func(*os.File, string, int) error
	walk = func(file *os.File, target string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 12 {
			return errors.New("source tree exceeds depth limit")
		}
		var before unix.Stat_t
		if err := unix.Fstat(int(file.Fd()), &before); err != nil {
			return err
		}
		if len(report.Files)+len(report.Directories) >= domain.HomeFilesMaxEntries {
			return errors.New("source tree exceeds 512 entries")
		}
		switch before.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			if !domain.ValidHomeFilesDestination(target, true) {
				return errors.New("source tree contains a protected destination")
			}
			report.Directories = append(report.Directories, target)
			report.Entries = append(report.Entries, domain.HomeFilesEntry{Destination: target, Directory: true})
			entries, err := file.ReadDir(domain.HomeFilesMaxEntries + 1)
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			if len(entries) > domain.HomeFilesMaxEntries {
				return errors.New("source directory has too many entries")
			}
			for _, entry := range entries {
				if domain.HomeFilesSensitivePath(entry.Name()) {
					return errors.New("source tree contains a credential or agent-state path")
				}
				childFD, err := unix.Openat2(int(file.Fd()), entry.Name(), &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK, Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_XDEV})
				if err != nil {
					return errors.New("source tree contains a symbolic link, mount or unreadable entry")
				}
				child := os.NewFile(uintptr(childFD), entry.Name())
				err = walk(child, target+"/"+entry.Name(), depth+1)
				var held, current unix.Stat_t
				statErr := unix.Fstat(childFD, &held)
				pathErr := unix.Fstatat(int(file.Fd()), entry.Name(), &current, unix.AT_SYMLINK_NOFOLLOW)
				child.Close()
				held.Atim = current.Atim
				if err != nil {
					return err
				}
				if statErr != nil || pathErr != nil || held != current {
					return domain.ErrHomeFilesConflict
				}
			}
		case unix.S_IFREG:
			if before.Nlink != 1 {
				return errors.New("hard-linked source files cannot be imported")
			}
			if !domain.ValidHomeFilesDestination(target, false) || before.Size > domain.HomeFilesMaxFileBytes {
				return errors.New("file destination is protected or file exceeds 16 MiB")
			}
			data, err := io.ReadAll(io.LimitReader(file, domain.HomeFilesMaxFileBytes+1))
			if err != nil {
				return err
			}
			if len(data) > domain.HomeFilesMaxFileBytes {
				return errors.New("source file exceeds 16 MiB")
			}
			total += int64(len(data))
			if total > domain.HomeFilesMaxTotalBytes {
				return errors.New("capture exceeds 64 MiB")
			}
			if domain.HomeFilesSensitiveContent(data) || rejectSecretPatch(string(data)) != nil {
				return errors.New("source contains credential markers; choose non-secret materials")
			}
			hash := fmt.Sprintf("%x", sha256.Sum256(data))
			blobFD, err := unix.Openat(int(stage.Fd()), hash, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
			if err != nil && !errors.Is(err, unix.EEXIST) {
				return err
			}
			if err == nil {
				blob := os.NewFile(uintptr(blobFD), hash)
				_, writeErr := blob.Write(data)
				syncErr := blob.Sync()
				closeErr := blob.Close()
				if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
					return err
				}
			}
			payload := "imports/" + hash
			report.Files = append(report.Files, domain.HomeFile{Destination: target, Source: &payload})
			report.Entries = append(report.Entries, domain.HomeFilesEntry{Destination: target, SHA256: hash, Size: int64(len(data))})
		default:
			return errors.New("source contains a special file")
		}
		var after unix.Stat_t
		if err := unix.Fstat(int(file.Fd()), &after); err != nil {
			return err
		}
		after.Atim = before.Atim
		if after != before {
			return domain.ErrHomeFilesConflict
		}
		return ctx.Err()
	}
	if err = walk(source, destination, 1); err != nil {
		return report, err
	}
	var final, atPath unix.Stat_t
	if err = unix.Fstat(fd, &final); err != nil {
		return report, err
	}
	if err = unix.Fstatat(unix.AT_FDCWD, sourcePath, &atPath, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return report, err
	}
	final.Atim = atPath.Atim
	if final != atPath {
		return report, domain.ErrHomeFilesConflict
	}
	data, _ := json.Marshal(homeCaptureRecord{Repository: repository, Report: report, CreatedAt: time.Now().UTC()})
	if err = assistantWriteState(stage, "capture.json", data); err != nil {
		return report, err
	}
	if err = stage.Sync(); err != nil {
		return report, err
	}
	if err = root.Sync(); err != nil {
		return report, err
	}
	complete = true
	return report, nil
}

func readHomeCapture(repository, id string) (homeCaptureRecord, *os.File, error) {
	if !homeCaptureID.MatchString(id) {
		return homeCaptureRecord{}, nil, errors.New("invalid capture identifier")
	}
	root, err := openHomeCaptures()
	if err != nil {
		return homeCaptureRecord{}, nil, err
	}
	defer root.Close()
	stage, err := assistantDirectory(root, id, false)
	if err != nil {
		return homeCaptureRecord{}, nil, fmt.Errorf("%w: capture unavailable; choose the source again", domain.ErrHomeFilesConflict)
	}
	data, err := assistantReadStateLimit(stage, "capture.json", domain.HomeFilesMaxBytes)
	if err != nil {
		stage.Close()
		return homeCaptureRecord{}, nil, err
	}
	var record homeCaptureRecord
	if err = assistantDecodeStrict(data, &record); err != nil || record.Repository != repository || record.Report.CaptureID != id || record.Report.HasErrors() || record.CreatedAt.IsZero() || time.Since(record.CreatedAt) > 24*time.Hour {
		stage.Close()
		return record, nil, errors.New("capture does not belong to this reviewed deployment")
	}
	return record, stage, nil
}

func (Local) CompleteHomeFilesPath(ctx context.Context, query string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.HasPrefix(query, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		query = filepath.Join(home, query[2:])
	}
	absolute, err := filepath.Abs(query)
	if err != nil {
		return nil, err
	}
	directory, prefix := filepath.Dir(absolute), filepath.Base(absolute)
	if strings.HasSuffix(query, "/") {
		directory = absolute
		prefix = ""
	}
	fd, err := unix.Openat2(unix.AT_FDCWD, directory, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC, Resolve: unix.RESOLVE_NO_SYMLINKS})
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), directory)
	defer file.Close()
	entries, err := file.ReadDir(1025)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 1024 {
		return nil, errors.New("source folder has too many names; type a more specific path")
	}
	results := []string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) && entry.Type()&os.ModeSymlink == 0 && !domain.HomeFilesSensitivePath(entry.Name()) {
			path := filepath.Join(directory, entry.Name())
			if entry.IsDir() {
				path += "/"
			}
			results = append(results, path)
			if len(results) == 50 {
				break
			}
		}
	}
	slices.Sort(results)
	return results, ctx.Err()
}

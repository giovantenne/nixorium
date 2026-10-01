package adapters

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"golang.org/x/sys/unix"

	"github.com/giovantenne/nixorium/internal/domain"
)

const (
	backupManifestName  = "manifest.json"
	backupDeployment    = "deployment"
	backupKnownHosts    = "ssh/nixorium-known-hosts"
	backupRecordName    = "backup.json"
	backupMaximumFile   = 512 * 1024 * 1024
	backupMaximumTotal  = 4 * 1024 * 1024 * 1024
	backupMaximumFiles  = 200000
	backupManifestLimit = 64 * 1024 * 1024
)

type backupSource struct {
	prefix string
	root   string
}

func backupSources(repository string) []backupSource {
	sources := []backupSource{{prefix: backupDeployment, root: repository}}
	if home, err := os.UserHomeDir(); err == nil {
		knownHosts := filepath.Join(home, ".ssh", "nixorium-known-hosts")
		if info, err := os.Lstat(knownHosts); err == nil && info.IsDir() {
			sources = append(sources, backupSource{prefix: backupKnownHosts, root: knownHosts})
		}
	}
	return sources
}

// collectBackupFiles lists what a backup contains. Links into the Nix store
// (build results) are skipped; they are rebuilt from the configuration.
func collectBackupFiles(sources []backupSource) ([]domain.BackupFile, map[string]string, error) {
	files := []domain.BackupFile{}
	origins := map[string]string{}
	var total int64
	for _, source := range sources {
		err := filepath.WalkDir(source.root, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(source.root, current)
			if err != nil {
				return err
			}
			name := source.prefix
			if relative != "." {
				name = path.Join(source.prefix, filepath.ToSlash(relative))
			}
			info, err := os.Lstat(current)
			if err != nil {
				return err
			}
			file := domain.BackupFile{Path: name, Mode: uint32(info.Mode().Perm())}
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				target, err := os.Readlink(current)
				if err != nil {
					return err
				}
				if strings.HasPrefix(target, "/nix/store/") {
					return nil
				}
				file.Link = target
			case info.IsDir():
				file.Mode |= uint32(fs.ModeDir)
			case info.Mode().IsRegular():
				if info.Size() > backupMaximumFile {
					return fmt.Errorf("%s is larger than the backup limit", name)
				}
				digest, err := hashBackupFile(current)
				if err != nil {
					return err
				}
				file.Size, file.SHA256 = info.Size(), digest
				total += info.Size()
			default:
				return nil
			}
			files = append(files, file)
			origins[name] = current
			if len(files) > backupMaximumFiles || total > backupMaximumTotal {
				return errors.New("the deployment is too large for a backup")
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, origins, nil
}

func hashBackupFile(name string) (string, error) {
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func validBackupPassphrase(passphrase []byte) error {
	if len([]rune(string(passphrase))) < domain.BackupMinimumPassphrase {
		return fmt.Errorf("the passphrase must have at least %d characters", domain.BackupMinimumPassphrase)
	}
	return nil
}

// CreateBackup writes one encrypted backup file into destination. The file is
// written under a temporary name and appears only once complete.
func (Local) CreateBackup(ctx context.Context, repository, destination string, passphrase []byte, version string) domain.BackupReport {
	report := domain.BackupReport{SchemaVersion: domain.SchemaVersion, Operation: "backup-create", State: "blocked", PrivateKeys: []string{}, Issues: []domain.ValidationIssue{}}
	fail := func(message string) domain.BackupReport {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "backup", Message: message})
		report.Message = message
		return report
	}
	if err := validBackupPassphrase(passphrase); err != nil {
		return fail(err.Error())
	}
	if !filepath.IsAbs(destination) {
		return fail("the destination must be an absolute directory")
	}
	if info, err := os.Stat(destination); err != nil || !info.IsDir() {
		return fail("the destination directory does not exist")
	}
	repository, err := filepath.Abs(repository)
	if err != nil {
		return fail(err.Error())
	}
	if strings.HasPrefix(filepath.Clean(destination)+"/", repository+"/") {
		return fail("the destination must be outside the deployment repository")
	}
	revision, err := (Local{}).GitRevision(ctx, repository)
	if err != nil {
		return fail("cannot read the deployment revision: " + err.Error())
	}
	files, origins, err := collectBackupFiles(backupSources(repository))
	if err != nil {
		return fail(err.Error())
	}
	created := time.Now().UTC()
	manifest := domain.BackupManifest{SchemaVersion: domain.BackupSchemaVersion, CreatedAt: created, NixoriumVersion: version, Revision: revision, Files: files}
	recipient, err := age.NewScryptRecipient(string(passphrase))
	if err != nil {
		return fail(err.Error())
	}
	name := "nixorium-backup-" + created.Format("20060102-150405") + ".age"
	partial, err := os.OpenFile(filepath.Join(destination, "."+name+".partial"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fail("cannot write into the destination: " + err.Error())
	}
	partialName := partial.Name()
	complete := false
	defer func() {
		if !complete {
			partial.Close()
			os.Remove(partialName)
		}
	}()
	if err := writeBackupArchive(partial, recipient, manifest, origins); err != nil {
		return fail(err.Error())
	}
	if err := partial.Sync(); err != nil {
		return fail(err.Error())
	}
	info, err := partial.Stat()
	if err != nil {
		return fail(err.Error())
	}
	if err := partial.Close(); err != nil {
		return fail(err.Error())
	}
	final := filepath.Join(destination, name)
	if err := unix.Renameat2(unix.AT_FDCWD, partialName, unix.AT_FDCWD, final, unix.RENAME_NOREPLACE); err != nil {
		return fail("cannot finish the backup file: " + err.Error())
	}
	complete = true
	if directory, err := os.Open(destination); err == nil {
		_ = directory.Sync()
		directory.Close()
	}
	report.State, report.Path, report.Bytes, report.CreatedAt, report.Revision, report.Files = "completed", final, info.Size(), created, revision, len(files)
	report.PrivateKeys = backupPrivateKeys(files)
	report.Message = "The encrypted backup is complete. Keep it away from this controller, and keep the passphrase separately: without it the backup cannot be read."
	if len(report.PrivateKeys) < len(privateDeploymentPaths) {
		report.Message += " Some private keys are missing from the deployment; a restore will need new keys for them."
	}
	recordBackup(domain.BackupRecord{CreatedAt: created, Path: final, Revision: revision, KeysDigest: backupKeysDigest(repository), SettingsHash: backupSettingsHash(repository)})
	return report
}

func writeBackupArchive(output io.Writer, recipient age.Recipient, manifest domain.BackupManifest, origins map[string]string) error {
	encrypted, err := age.Encrypt(output, recipient)
	if err != nil {
		return err
	}
	compressed := gzip.NewWriter(encrypted)
	archive := tar.NewWriter(compressed)
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := archive.WriteHeader(&tar.Header{Name: backupManifestName, Mode: 0o600, Size: int64(len(manifestData)), Typeflag: tar.TypeReg, ModTime: manifest.CreatedAt}); err != nil {
		return err
	}
	if _, err := archive.Write(manifestData); err != nil {
		return err
	}
	for _, file := range manifest.Files {
		header := &tar.Header{Name: file.Path, Mode: int64(file.Mode & 0o7777), ModTime: manifest.CreatedAt}
		switch {
		case file.Link != "":
			header.Typeflag, header.Linkname = tar.TypeSymlink, file.Link
		case fs.FileMode(file.Mode)&fs.ModeDir != 0:
			header.Typeflag, header.Name = tar.TypeDir, file.Path+"/"
		default:
			header.Typeflag, header.Size = tar.TypeReg, file.Size
		}
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		source, err := os.Open(origins[file.Path])
		if err != nil {
			return err
		}
		digest := sha256.New()
		written, err := io.Copy(archive, io.TeeReader(io.LimitReader(source, file.Size), digest))
		source.Close()
		if err != nil {
			return err
		}
		if written != file.Size || hex.EncodeToString(digest.Sum(nil)) != file.SHA256 {
			return fmt.Errorf("%s changed while the backup was written; try again", file.Path)
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if err := compressed.Close(); err != nil {
		return err
	}
	return encrypted.Close()
}

// readBackup decrypts a backup and calls visit for each entry after checking
// it against the manifest. Every listed file must be present and intact.
func readBackup(name string, passphrase []byte, visit func(domain.BackupFile, io.Reader) error) (domain.BackupManifest, error) {
	var manifest domain.BackupManifest
	file, err := os.Open(name)
	if err != nil {
		return manifest, err
	}
	defer file.Close()
	identity, err := age.NewScryptIdentity(string(passphrase))
	if err != nil {
		return manifest, err
	}
	decrypted, err := age.Decrypt(file, identity)
	if err != nil {
		return manifest, errors.New("the passphrase is wrong or the file is not a Nixorium backup")
	}
	compressed, err := gzip.NewReader(decrypted)
	if err != nil {
		return manifest, errors.New("the backup is damaged")
	}
	archive := tar.NewReader(compressed)
	header, err := archive.Next()
	if err != nil || header.Name != backupManifestName || header.Size > backupManifestLimit {
		return manifest, errors.New("the backup has no manifest")
	}
	data, err := io.ReadAll(io.LimitReader(archive, backupManifestLimit))
	if err != nil || json.Unmarshal(data, &manifest) != nil || manifest.SchemaVersion != domain.BackupSchemaVersion {
		return manifest, errors.New("the backup manifest is not readable")
	}
	expected := map[string]domain.BackupFile{}
	for _, entry := range manifest.Files {
		if !safeBackupPath(entry.Path) {
			return manifest, fmt.Errorf("the backup contains an unsafe path: %s", entry.Path)
		}
		expected[entry.Path] = entry
	}
	seen := map[string]bool{}
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return manifest, errors.New("the backup is damaged")
		}
		name := strings.TrimSuffix(header.Name, "/")
		entry, found := expected[name]
		if !found || seen[name] {
			return manifest, fmt.Errorf("the backup contains an unexpected entry: %s", name)
		}
		seen[name] = true
		digest := sha256.New()
		reader := io.TeeReader(archive, digest)
		if err := visit(entry, reader); err != nil {
			return manifest, err
		}
		if _, err := io.Copy(io.Discard, reader); err != nil {
			return manifest, errors.New("the backup is damaged")
		}
		if header.Typeflag == tar.TypeReg && (header.Size != entry.Size || hex.EncodeToString(digest.Sum(nil)) != entry.SHA256) {
			return manifest, fmt.Errorf("%s does not match the manifest", name)
		}
	}
	if len(seen) != len(expected) {
		return manifest, errors.New("the backup is incomplete")
	}
	return manifest, nil
}

func safeBackupPath(name string) bool {
	clean := path.Clean(name)
	return clean == name && !path.IsAbs(name) && !strings.HasPrefix(name, "../") && name != ".." &&
		(name == backupDeployment || strings.HasPrefix(name, backupDeployment+"/") || name == backupKnownHosts || strings.HasPrefix(name, backupKnownHosts+"/"))
}

func (Local) VerifyBackup(name string, passphrase []byte) domain.BackupReport {
	report := domain.BackupReport{SchemaVersion: domain.SchemaVersion, Operation: "backup-verify", State: "blocked", Path: name, PrivateKeys: []string{}, Issues: []domain.ValidationIssue{}}
	manifest, err := readBackup(name, passphrase, func(domain.BackupFile, io.Reader) error { return nil })
	if err != nil {
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "backup", Message: err.Error()})
		report.Message = err.Error()
		return report
	}
	report.State, report.CreatedAt, report.Revision, report.Files = "completed", manifest.CreatedAt, manifest.Revision, len(manifest.Files)
	report.PrivateKeys = backupPrivateKeys(manifest.Files)
	report.Message = fmt.Sprintf("The backup from %s is complete and readable.", manifest.CreatedAt.Local().Format("2006-01-02 15:04"))
	return report
}

// RestoreBackup extracts a verified backup into an empty directory. It never
// writes over an existing deployment.
func (Local) RestoreBackup(name string, passphrase []byte, target string) domain.BackupReport {
	report := (Local{}).VerifyBackup(name, passphrase)
	report.Operation = "backup-restore"
	if report.State != "completed" {
		return report
	}
	fail := func(message string) domain.BackupReport {
		report.State = "blocked"
		report.Issues = append(report.Issues, domain.ValidationIssue{Field: "target", Message: message})
		report.Message = message
		return report
	}
	if !filepath.IsAbs(target) {
		return fail("the target must be an absolute directory")
	}
	if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
		return fail("the target directory is not empty")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err.Error())
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		return fail(err.Error())
	}
	_, err := readBackup(name, passphrase, func(entry domain.BackupFile, content io.Reader) error {
		destination := filepath.Join(target, filepath.FromSlash(entry.Path))
		mode := fs.FileMode(entry.Mode)
		switch {
		case entry.Link != "":
			return os.Symlink(entry.Link, destination)
		case mode&fs.ModeDir != 0:
			return os.MkdirAll(destination, mode.Perm()|0o700)
		default:
			if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
				return err
			}
			file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, content)
			syncErr := file.Sync()
			closeErr := file.Close()
			return errors.Join(copyErr, syncErr, closeErr)
		}
	})
	if err != nil {
		return fail("the restore stopped: " + err.Error() + "; remove the partial target before trying again")
	}
	report.State, report.Path = "completed", target
	report.Message = "The backup was restored into " + target + ". Follow the controller replacement steps in the troubleshooting guide to use it."
	return report
}

func backupPrivateKeys(files []domain.BackupFile) []string {
	present := map[string]bool{}
	for _, file := range files {
		present[file.Path] = true
	}
	keys := []string{}
	for _, name := range privateDeploymentPaths {
		if present[backupDeployment+"/"+name] {
			keys = append(keys, name)
		}
	}
	return keys
}

func backupKeysDigest(repository string) string {
	digest := sha256.New()
	for _, name := range privateDeploymentPaths {
		content, _ := os.ReadFile(filepath.Join(repository, name))
		fmt.Fprintf(digest, "%s\x00%d\x00", name, len(content))
		digest.Write(content)
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func backupSettingsHash(repository string) string {
	content, err := os.ReadFile(filepath.Join(repository, "lab-settings.json"))
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func backupRecordPath() (string, error) {
	root, err := userStateRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "nixorium", backupRecordName), nil
}

func recordBackup(record domain.BackupRecord) {
	name, err := backupRecordPath()
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return
	}
	data, err := json.Marshal(record)
	if err != nil {
		return
	}
	temporary := name + ".tmp"
	if os.WriteFile(temporary, data, 0o600) == nil {
		_ = os.Rename(temporary, name)
	}
}

func (Local) LastBackup() (domain.BackupRecord, bool) {
	var record domain.BackupRecord
	name, err := backupRecordPath()
	if err != nil {
		return record, false
	}
	data, err := os.ReadFile(name)
	if err != nil || json.Unmarshal(data, &record) != nil || record.CreatedAt.IsZero() {
		return domain.BackupRecord{}, false
	}
	return record, true
}

// BackupDue explains why a new backup is advisable, if it is.
func (local Local) BackupDue(repository string) (string, bool) {
	record, found := local.LastBackup()
	switch {
	case !found:
		return "No backup of this controller is recorded. Without its private keys, a replaced controller cannot manage the installed computers.", true
	case time.Since(record.CreatedAt) > domain.BackupReminderAge:
		return fmt.Sprintf("The last backup is %d days old.", int(time.Since(record.CreatedAt).Hours()/24)), true
	case record.KeysDigest != backupKeysDigest(repository):
		return "The private keys changed since the last backup.", true
	case record.SettingsHash != "" && record.SettingsHash != backupSettingsHash(repository):
		return "The laboratory settings changed since the last backup.", true
	}
	return "", false
}

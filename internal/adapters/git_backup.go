package adapters

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

const gitRecoveryFile = "nixorium-recovery.age"
const gitBackupReceipt = "nixorium-remote-backup.json"
const maxRecoveryBytes = 8 * 1024 * 1024

// Local transports are available only to in-package disposable Git fixtures.
// Production accepts SSH URLs and uses the administrator's independent SSH
// agent/configuration. No credential or passphrase is saved in the deployment.
type GitBackup struct{ allowLocal bool }

type gitRecovery struct {
	SchemaVersion int               `json:"schemaVersion"`
	Keys          map[string][]byte `json:"keys"`
	KnownHosts    map[string][]byte `json:"knownHosts"`
}

type gitBackupRecord struct {
	Plan              domain.GitBackupPlan `json:"plan"`
	PublishedRevision string               `json:"publishedRevision"`
	KeysDigest        string               `json:"keysDigest"`
	CreatedAt         time.Time            `json:"createdAt"`
}

var backupPasswordHash = regexp.MustCompile(`\$6\$(?:rounds=[0-9]+\$)?[A-Za-z0-9./]{1,64}\$[A-Za-z0-9./]{3,}`)

var backupSSHURL = regexp.MustCompile(`^[a-zA-Z0-9_.-]+@[a-zA-Z0-9.-]+:[a-zA-Z0-9_./-]+$`)
var backupBranch = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]*$`)

func (g GitBackup) validateDestination(remote, branch string) error {
	valid := backupSSHURL.MatchString(remote)
	if u, err := url.Parse(remote); err == nil && u.Scheme == "ssh" && u.Hostname() != "" && u.Path != "" && u.RawQuery == "" && u.Fragment == "" {
		_, password := u.User.Password()
		valid = !password && !strings.ContainsAny(remote, "\r\n\t ")
	}
	if g.allowLocal && filepath.IsAbs(remote) && !strings.ContainsAny(remote, "\r\n") {
		valid = true
	}
	if !valid {
		return errors.New("use the private repository's SSH URL (git@github.com:owner/repository.git or ssh://git@host/path); configure SSH access separately")
	}
	if !backupBranch.MatchString(branch) || strings.Contains(branch, "..") || strings.Contains(branch, "//") || strings.HasSuffix(branch, "/") || strings.HasSuffix(branch, ".") || strings.Contains(branch, ".lock") {
		return errors.New("choose a valid Git branch, for example main")
	}
	return nil
}

// Use a fresh Git directory for network access: deployment/global Git config
// cannot redirect the reviewed URL, run hooks/filters or push additional refs.
func backupGit(ctx context.Context, repository string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	environment := []string{}
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "GIT_") && !strings.HasPrefix(v, "SSH_ASKPASS=") {
			environment = append(environment, v)
		}
	}
	environment = append(environment, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -oBatchMode=yes -oStrictHostKeyChecking=yes -oConnectTimeout=15")
	arguments := []string{"-c", "core.hooksPath=/dev/null", "-c", "protocol.ext.allow=never", "-c", "protocol.file.allow=always", "-c", "push.followTags=false", "-c", "commit.gpgSign=false"}
	arguments = append(arguments, args...)
	out, truncated, err := runBoundedGitWithEnvironment(ctx, repository, environment, 16*1024*1024, arguments...)
	if err != nil {
		return "", errors.New("Git operation failed; check SSH access, the repository URL and branch, then retry (no force push is performed)")
	}
	if truncated {
		return "", errors.New("Git output exceeded the backup limit")
	}
	return out, nil
}

func recoveryMaterial(repository string) (gitRecovery, error) {
	r := gitRecovery{SchemaVersion: 1, Keys: map[string][]byte{}, KnownHosts: map[string][]byte{}}
	for _, name := range privateDeploymentPaths {
		data, mode, err := readRecoveryFile(filepath.Join(repository, name), maximumKeyMaterialBytes)
		if err != nil || mode&0077 != 0 || len(data) == 0 {
			return r, fmt.Errorf("%s is missing or unsafe; use Maintenance → Change settings → Controller keys before backup", name)
		}
		r.Keys[name] = data
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return r, err
	}
	root := filepath.Join(home, ".ssh", "nixorium-known-hosts")
	if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err := requireRealDirectory(root, "trusted computer keys"); err != nil {
		return r, err
	}
	total := 0
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() == ".nixorium-known-hosts.lock" {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		data, _, err := readRecoveryFile(name, maximumKeyMaterialBytes)
		if err != nil {
			return errors.New("trusted computer keys contain an unsafe or oversized file")
		}
		if len(bytes.TrimSpace(data)) == 0 {
			return nil
		}
		total += len(data)
		if total > maxRecoveryBytes/2 || len(r.KnownHosts) >= 1000 {
			return errors.New("trusted computer keys exceed the recovery limit")
		}
		r.KnownHosts[filepath.ToSlash(rel)] = data
		return nil
	})
	return r, err
}

func recoveryDigest(r gitRecovery) string {
	data, _ := json.Marshal(r)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func (g GitBackup) Plan(ctx context.Context, repository, remote, branch string) (domain.GitBackupPlan, error) {
	p := domain.GitBackupPlan{Repository: repository, Remote: remote, Branch: branch}
	if err := g.validateDestination(remote, branch); err != nil {
		return p, err
	}
	if err := requireRealDirectory(repository, "deployment"); err != nil {
		return p, err
	}
	if err := requireRealDirectory(filepath.Join(repository, ".git"), "deployment Git directory"); err != nil {
		return p, err
	}
	for _, spec := range keyMaterialSpecs {
		if _, _, err := readRecoveryFile(filepath.Join(repository, spec.publicPath), maximumKeyMaterialBytes); err != nil {
			return p, errors.New("public keys must be regular files inside the deployment")
		}
	}
	if (Local{}).TemplateResetPending(repository) {
		return p, errors.New("recover the interrupted template reset before backing up")
	}
	if err := ensurePrivateFilesUntracked(ctx, repository); err != nil {
		return p, err
	}
	status, err := backupGit(ctx, repository, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return p, err
	}
	if strings.TrimSpace(status) != "" {
		return p, errors.New("save all deployment changes first: Maintenance → Review Git changes (nixorium git review); the backup never stages unrelated files")
	}
	for _, name := range []string{"flake.nix", "flake.lock", "lab-settings.json"} {
		if _, err := backupGit(ctx, repository, "cat-file", "-e", "HEAD:"+name); err != nil {
			return p, fmt.Errorf("save %s in Git before backing up", name)
		}
	}
	for _, key := range (Local{}).KeyMaterial(ctx, repository) {
		if !key.Verified || !key.Matches || !key.Safe {
			return p, fmt.Errorf("verify the %s key pair before backup", key.Name)
		}
	}
	material, err := recoveryMaterial(repository)
	if err != nil {
		return p, err
	}
	if _, err := decodeCredentials(material.Keys[credentialsFile]); err != nil {
		return p, err
	}
	localSettings, err := (Local{}).ReadSettings(repository)
	if err != nil {
		return p, err
	}
	settings, issues := domain.DecodeLabSettings(localSettings)
	if len(issues) != 0 || !domain.CredentialsFromSettings(settings).Ready() {
		return p, errors.New("set all account passwords before backup")
	}
	p.RecoveryDigest = recoveryDigest(material)
	p.Revision, err = (Local{}).GitRevision(ctx, repository)
	if err != nil {
		return p, err
	}
	files, err := backupGit(ctx, repository, "ls-tree", "-rz", "--name-only", p.Revision)
	if err != nil {
		return p, err
	}
	p.Files = strings.Count(files, "\x00")
	tree, err := backupGit(ctx, repository, "ls-tree", "-r", p.Revision)
	if err != nil {
		return p, err
	}
	for _, entry := range strings.Split(tree, "\n") {
		if strings.HasPrefix(entry, "160000 ") {
			return p, errors.New("submodule contents are not a complete lab backup; save deployment files directly before backing up")
		}
	}
	// Scan reachable history as well as the current tree: deleting a tracked
	// secret in the latest commit does not remove it from a push.
	if err := g.checkHistory(ctx, repository, p.Revision, material); err != nil {
		return p, err
	}
	p.ReviewToken = p.Token()
	return p, nil
}

func (g GitBackup) checkHistory(ctx context.Context, repository, revision string, material gitRecovery) error {
	paths, err := backupGit(ctx, repository, "log", "--format=", "--name-only", revision, "--", "secret-key", "admin-ssh", credentialsFile)
	if err != nil {
		return err
	}
	if strings.TrimSpace(paths) != "" {
		return errors.New("private keys occur in Git history; do not push this repository: rotate exposed keys and clean the history first")
	}
	objects, err := backupGit(ctx, repository, "rev-list", "--objects", revision)
	if err != nil {
		return err
	}
	if strings.Count(objects, "\n") > 20000 {
		return errors.New("repository history exceeds the backup inspection limit")
	}
	var total int
	for _, line := range strings.Split(strings.TrimSpace(objects), "\n") {
		oid, path, found := strings.Cut(line, " ")
		if !found || path == "" {
			continue
		}
		kind, err := backupGit(ctx, repository, "cat-file", "-t", oid)
		if err != nil {
			return err
		}
		if strings.TrimSpace(kind) != "blob" {
			continue
		}
		data, err := backupGit(ctx, repository, "cat-file", "blob", oid)
		if err != nil {
			return err
		}
		total += len(data)
		if total > 128*1024*1024 {
			return errors.New("repository history exceeds the 128 MiB inspection limit")
		}
		// Ciphertext is binary; scan without including content in errors or logs.
		if privateKeyContent.MatchString(data) || backupPasswordHash.MatchString(data) {
			return errors.New("recognizable private key, password hash or access token in Git history; clean the history before backup")
		}
		for _, key := range material.Keys {
			if strings.Contains(data, strings.TrimSpace(string(key))) {
				return errors.New("private key content in Git history; clean the history before backup")
			}
		}
	}
	return nil
}

func encryptedRecovery(r gitRecovery, passphrase []byte) ([]byte, error) {
	if err := validBackupPassphrase(passphrase); err != nil {
		return nil, err
	}
	recipient, err := age.NewScryptRecipient(string(passphrase))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	writer, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(writer).Encode(r); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decryptRecovery(data, passphrase []byte) (gitRecovery, error) {
	var r gitRecovery
	identity, err := age.NewScryptIdentity(string(passphrase))
	if err != nil {
		return r, err
	}
	identity.SetMaxWorkFactor(18)
	reader, err := age.Decrypt(bytes.NewReader(data), identity)
	if err != nil {
		return r, errors.New("wrong passphrase or damaged recovery file")
	}
	plain, err := io.ReadAll(io.LimitReader(reader, maxRecoveryBytes+1))
	if err != nil || len(plain) > maxRecoveryBytes {
		return r, errors.New("damaged or oversized recovery file")
	}
	decoder := json.NewDecoder(bytes.NewReader(plain))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&r); err != nil || r.SchemaVersion != 1 || len(r.Keys) != len(privateDeploymentPaths) {
		return r, errors.New("invalid recovery manifest")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return r, errors.New("trailing recovery manifest data")
	}
	for _, name := range privateDeploymentPaths {
		if len(r.Keys[name]) == 0 || len(r.Keys[name]) > maximumKeyMaterialBytes {
			return r, errors.New("incomplete recovery keys")
		}
	}
	if _, err := decodeCredentials(r.Keys[credentialsFile]); err != nil {
		return r, err
	}
	if len(r.KnownHosts) > 1000 {
		return r, errors.New("too many trusted-key files")
	}
	for name, data := range r.KnownHosts {
		if filepath.Base(name) == ".nixorium-known-hosts.lock" || name == "." || filepath.IsAbs(name) || filepath.ToSlash(filepath.Clean(name)) != name || strings.HasPrefix(name, "../") || name == ".." || strings.ContainsAny(name, "\x00\r\n") || len(data) > maximumKeyMaterialBytes {
			return r, errors.New("unsafe trusted-key recovery entry")
		}
	}
	return r, nil
}

func gitBackupFailure(operation string, err error) domain.BackupReport {
	return domain.BackupReport{SchemaVersion: domain.SchemaVersion, Operation: operation, State: "blocked", Message: err.Error(), Issues: []domain.ValidationIssue{{Field: "backup", Message: err.Error()}}}
}

func (g GitBackup) Publish(ctx context.Context, plan domain.GitBackupPlan, passphrase []byte) domain.BackupReport {
	fail := func(err error) domain.BackupReport { return gitBackupFailure("backup-publish", err) }
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	root, err := openWorkspaceRoot(plan.Repository, unix.LOCK_EX)
	if err != nil {
		return fail(err)
	}
	defer root.Close()
	fresh, err := g.Plan(ctx, plan.Repository, plan.Remote, plan.Branch)
	if err != nil {
		return fail(err)
	}
	if plan.ReviewToken == "" || fresh.ReviewToken != plan.ReviewToken || plan.Token() != plan.ReviewToken {
		return fail(errors.New("backup review changed; create a new review before pushing"))
	}
	material, err := recoveryMaterial(plan.Repository)
	if err != nil {
		return fail(err)
	}
	if recoveryDigest(material) != plan.RecoveryDigest {
		return fail(errors.New("keys changed after review"))
	}
	encrypted, err := encryptedRecovery(material, passphrase)
	if err != nil {
		return fail(err)
	}
	name := filepath.Join(plan.Repository, gitRecoveryFile)
	// A retry reuses the existing ciphertext and commit if the passphrase and
	// recovery data still match. Failed pushes never cause automatic force.
	existing, _, readErr := readRecoveryFile(name, maxRecoveryBytes)
	reuse := false
	if readErr == nil {
		previous, err := decryptRecovery(existing, passphrase)
		reuse = err == nil && recoveryDigest(previous) == plan.RecoveryDigest
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return fail(readErr)
	}
	revision := plan.Revision
	if !reuse {
		if err := writeRecoveryFile(plan.Repository, encrypted); err != nil {
			return fail(err)
		}
		proposal, err := (Local{}).GitCommitProposal(ctx, plan.Repository, []string{gitRecoveryFile})
		if err != nil {
			return fail(fmt.Errorf("encrypted file saved locally; review it in Maintenance → Review Git changes before retrying: %w", err))
		}
		revision, err = (Local{}).CommitGitPaths(ctx, plan.Repository, []string{gitRecoveryFile}, "chore: save encrypted laboratory recovery keys", plan.Revision, proposal.TreeID)
		if err != nil {
			return fail(fmt.Errorf("encrypted file saved locally; inspect Git changes before retrying: %w", err))
		}
	}
	if err := pushVerifiedRevision(ctx, plan, revision); err != nil {
		return fail(fmt.Errorf("recovery commit %s is saved locally; remote backup is unconfirmed. %w", revision[:12], err))
	}
	now := time.Now().UTC()
	current, err := recoveryMaterial(plan.Repository)
	head, headErr := (Local{}).GitRevision(ctx, plan.Repository)
	if err != nil || headErr != nil || head != revision || recoveryDigest(current) != plan.RecoveryDigest {
		return fail(errors.New("local configuration or keys changed while pushing; back up again"))
	}
	record := gitBackupRecord{Plan: plan, PublishedRevision: revision, KeysDigest: backupKeysDigest(plan.Repository), CreatedAt: now}
	if err := writeGitBackupRecord(plan.Repository, record); err != nil {
		return fail(errors.New("remote revision verified, but its local receipt could not be saved; retry Back up lab"))
	}
	return domain.BackupReport{SchemaVersion: domain.SchemaVersion, Operation: "backup-publish", State: "completed", Path: plan.Remote, Revision: revision, CreatedAt: now, Files: plan.Files, PrivateKeys: append([]string{}, privateDeploymentPaths...), Message: "Backup verified on the private remote. Keep its SSH access and recovery passphrase separately. No system was applied."}
}

// pushVerifiedRevision publishes one exact revision from a fresh bare copy, so
// deployment Git configuration cannot redirect it, and confirms the remote ref.
// It never forces: a diverged remote needs human reconciliation.
func pushVerifiedRevision(ctx context.Context, plan domain.GitBackupPlan, revision string) error {
	bare, err := os.MkdirTemp("", "nixorium-backup-transport-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(bare)
	// No deployment config is copied; only objects and HEAD are imported.
	if _, err := backupGit(ctx, bare, "clone", "--bare", "--no-local", "--", plan.Repository, filepath.Join(bare, "repo")); err != nil {
		return err
	}
	transport := filepath.Join(bare, "repo")
	if _, err := backupGit(ctx, transport, "config", "--remove-section", "remote.origin"); err != nil {
		return err
	}
	if _, err := backupGit(ctx, transport, "push", "--porcelain", "--no-follow-tags", "--", plan.Remote, revision+":refs/heads/"+plan.Branch); err != nil {
		return err
	}
	remote, err := backupGit(ctx, transport, "ls-remote", "--refs", "--", plan.Remote, "refs/heads/"+plan.Branch)
	if err != nil || strings.TrimSpace(remote) != revision+"\trefs/heads/"+plan.Branch {
		return errors.New("push completed but remote verification failed; retry Back up lab to verify it")
	}
	return nil
}

// Sync keeps a configured Git backup current without the passphrase. The
// encrypted recovery file already in Git stays valid while keys, account
// credentials and trusted computers are unchanged, so saved configuration can
// be pushed as is. Any change to that material needs Back up lab, because only
// the administrator can encrypt it again. skip names a revision whose push
// already failed in this session, to avoid retrying it on every refresh.
func (g GitBackup) Sync(ctx context.Context, repository, skip string) domain.BackupReport {
	report := domain.BackupReport{SchemaVersion: domain.SchemaVersion, Operation: "backup-sync", PrivateKeys: []string{}, Issues: []domain.ValidationIssue{}}
	record, found := readGitBackupRecord(repository)
	if !found {
		report.State = "unconfigured"
		return report
	}
	report.Path = record.Plan.Remote
	stop := func(state, message string) domain.BackupReport {
		report.State, report.Message = state, message
		if state == "blocked" {
			report.Issues = append(report.Issues, domain.ValidationIssue{Field: "backup", Message: message})
		}
		return report
	}
	needsPassphrase := "Keys, account passwords or trusted computers changed since the last Git backup. Open Maintenance → Back up lab and enter the recovery passphrase."
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	head, err := (Local{}).GitRevision(ctx, repository)
	if err != nil {
		return stop("blocked", "Cannot read the saved configuration revision.")
	}
	report.Revision = head
	material, err := recoveryMaterial(repository)
	if err != nil {
		return stop("blocked", err.Error())
	}
	if recoveryDigest(material) != record.Plan.RecoveryDigest {
		return stop("needs-passphrase", needsPassphrase)
	}
	if head == record.PublishedRevision {
		return stop("current", "The Git backup is up to date.")
	}
	if head == skip {
		return stop("skipped", "")
	}
	// Unsaved edits are pushed after they are saved, not reported as failures.
	if status, err := backupGit(ctx, repository, "status", "--porcelain=v1", "--untracked-files=all"); err != nil || strings.TrimSpace(status) != "" {
		return stop("waiting", "")
	}
	root, err := openWorkspaceRoot(repository, unix.LOCK_EX)
	if err != nil {
		return stop("blocked", err.Error())
	}
	defer root.Close()
	plan, err := g.Plan(ctx, repository, record.Plan.Remote, record.Plan.Branch)
	if err != nil {
		return stop("blocked", err.Error())
	}
	if plan.Revision != head {
		return stop("blocked", "The configuration changed while preparing the backup; it will be pushed next time.")
	}
	if plan.RecoveryDigest != record.Plan.RecoveryDigest {
		return stop("needs-passphrase", needsPassphrase)
	}
	// The pushed tree must carry the same encrypted recovery file as the last
	// verified backup; otherwise a restore could not decrypt matching keys.
	current, currentErr := backupGit(ctx, repository, "rev-parse", "--verify", "--quiet", head+":"+gitRecoveryFile)
	published, publishedErr := backupGit(ctx, repository, "rev-parse", "--verify", "--quiet", record.PublishedRevision+":"+gitRecoveryFile)
	if currentErr != nil || publishedErr != nil || strings.TrimSpace(current) != strings.TrimSpace(published) {
		return stop("needs-passphrase", "The encrypted recovery file changed since the last Git backup. Open Maintenance → Back up lab and enter the recovery passphrase.")
	}
	if err := pushVerifiedRevision(ctx, plan, head); err != nil {
		return stop("blocked", "Automatic Git backup failed: "+err.Error())
	}
	if err := writeGitBackupRecord(repository, gitBackupRecord{Plan: plan, PublishedRevision: head, KeysDigest: backupKeysDigest(repository), CreatedAt: time.Now().UTC()}); err != nil {
		return stop("blocked", "The remote backup was pushed, but its local receipt could not be saved.")
	}
	report.CreatedAt = time.Now().UTC()
	return stop("completed", "Saved changes were backed up to "+record.Plan.Remote+".")
}

func writeRecoveryFile(repository string, data []byte) error {
	file, err := os.CreateTemp(repository, ".nixorium-recovery-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), filepath.Join(repository, gitRecoveryFile)); err != nil {
		return err
	}
	return syncDirectory(repository)
}

func gitBackupRecordPath(repository string) string {
	return filepath.Join(repository, ".git", gitBackupReceipt)
}
func writeGitBackupRecord(repository string, record gitBackupRecord) error {
	if err := requireRealDirectory(filepath.Join(repository, ".git"), "Git directory"); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Join(repository, ".git"), ".remote-backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	err = errors.Join(err, file.Sync(), file.Close())
	if err != nil {
		return err
	}
	if err := os.Rename(file.Name(), gitBackupRecordPath(repository)); err != nil {
		return err
	}
	return syncDirectory(filepath.Join(repository, ".git"))
}
func readGitBackupRecord(repository string) (gitBackupRecord, bool) {
	var r gitBackupRecord
	data, _, err := readRecoveryFile(gitBackupRecordPath(repository), 64*1024)
	ok := err == nil && json.Unmarshal(data, &r) == nil && !r.CreatedAt.IsZero()
	return r, ok
}

// Destination returns the last verified Git backup, or proposes the deployment's
// own branch (master for deployments created by the installer).
func (g GitBackup) Destination(repository string) (string, string) {
	if r, ok := readGitBackupRecord(repository); ok {
		return r.Plan.Remote, r.Plan.Branch
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if branch, err := backupGit(ctx, repository, "symbolic-ref", "--quiet", "--short", "HEAD"); err == nil && backupBranch.MatchString(strings.TrimSpace(branch)) {
		return "", strings.TrimSpace(branch)
	}
	return "", DefaultBackupBranch
}

// DefaultBackupBranch is the branch the installer creates for new deployments.
const DefaultBackupBranch = "master"

// Restore uses a fresh private directory, disables hooks/filters and submodule
// recursion, checks the original key pairs, then publishes without replacing
// any existing deployment. It never evaluates fetched Nix or activates a system.
func (g GitBackup) Restore(ctx context.Context, remote, branch, target string, passphrase []byte) domain.BackupReport {
	fail := func(err error) domain.BackupReport { return gitBackupFailure("backup-clone", err) }
	if err := g.validateDestination(remote, branch); err != nil {
		return fail(err)
	}
	if !filepath.IsAbs(target) || filepath.Clean(target) != target || target == "/" {
		return fail(errors.New("choose an absolute, new deployment directory"))
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		return fail(errors.New("restore needs a new directory; existing directories are never replaced"))
	}
	parent, err := openWorkspaceRoot(filepath.Dir(target), unix.LOCK_EX)
	if err != nil {
		return fail(err)
	}
	defer parent.Close()
	stage, err := os.MkdirTemp(filepath.Dir(target), ".nixorium-restore-*")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(stage)
	if _, err := backupGit(ctx, stage, "init", "--initial-branch="+branch); err != nil {
		return fail(err)
	}
	if _, err := backupGit(ctx, stage, "fetch", "--update-head-ok", "--no-tags", "--no-recurse-submodules", "--", remote, "refs/heads/"+branch+":refs/heads/"+branch); err != nil {
		return fail(err)
	}
	if _, err := backupGit(ctx, stage, "checkout", "--force", branch); err != nil {
		return fail(err)
	}
	for _, name := range []string{"flake.nix", "flake.lock", "lab-settings.json", gitRecoveryFile} {
		if _, _, err := readRecoveryFile(filepath.Join(stage, name), maxRecoveryBytes); err != nil {
			return fail(fmt.Errorf("restore requires a safe %s file", name))
		}
	}
	if err := ensurePrivateFilesUntracked(ctx, stage); err != nil {
		return fail(err)
	}
	encrypted, _, err := readRecoveryFile(filepath.Join(stage, gitRecoveryFile), maxRecoveryBytes)
	if err != nil {
		return fail(err)
	}
	material, err := decryptRecovery(encrypted, passphrase)
	if err != nil {
		return fail(err)
	}
	for _, name := range privateDeploymentPaths {
		if err := writeRegularFileCreateNew(filepath.Join(stage, name), material.Keys[name], 0600); err != nil {
			return fail(err)
		}
	}
	configureRemote := func() error {
		for _, setting := range [][2]string{
			{"remote.origin.url", remote},
			{"remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"},
			{"branch." + branch + ".remote", "origin"},
			{"branch." + branch + ".merge", "refs/heads/" + branch},
		} {
			if _, err := backupGit(ctx, stage, "config", setting[0], setting[1]); err != nil {
				return err
			}
		}
		return nil
	}
	revision, publishErr, err := publishRestoredLab(ctx, stage, target, material.KnownHosts, configureRemote)
	if err != nil {
		return fail(err)
	}
	partial := func(err error) domain.BackupReport {
		r := fail(err)
		r.State = "partial"
		r.Path = target
		r.Message = "Deployment and private keys restored into " + target + ", but trust/receipt finalization failed. Keep this directory; do not activate yet. " + err.Error()
		return r
	}
	if publishErr != nil {
		return partial(publishErr)
	}
	plan := domain.GitBackupPlan{Repository: target, Remote: remote, Branch: branch, Revision: revision, RecoveryDigest: recoveryDigest(material)}
	if err := writeGitBackupRecord(target, gitBackupRecord{Plan: plan, PublishedRevision: revision, KeysDigest: backupKeysDigest(target), CreatedAt: time.Now().UTC()}); err != nil {
		return partial(err)
	}
	return domain.BackupReport{SchemaVersion: domain.SchemaVersion, Operation: "backup-clone", State: "completed", Path: target, Revision: revision, PrivateKeys: append([]string{}, privateDeploymentPaths...), Message: "Lab restored with its original keys and trusted computers. Open nixorium --repo " + target + "; follow the controller replacement guide before applying. No system was activated."}
}

// publishRestoredLab checks a staged deployment whose private keys are already
// in place, then moves it to target without replacing anything and fills the
// controller's trusted computer keys. It is shared by Git and file restores.
// err means nothing was published; publishErr means target exists but trust
// finalization failed and the directory must be kept for inspection.
func publishRestoredLab(ctx context.Context, stage, target string, knownHosts map[string][]byte, beforePublish func() error) (revision string, publishErr, err error) {
	for _, name := range []string{"flake.nix", "flake.lock", "lab-settings.json"} {
		if _, _, err := readRecoveryFile(filepath.Join(stage, name), maxRecoveryBytes); err != nil {
			return "", nil, fmt.Errorf("restore requires a safe %s file", name)
		}
	}
	if err := ensurePrivateFilesUntracked(ctx, stage); err != nil {
		return "", nil, err
	}
	restored, err := (Local{}).ReadSettings(stage)
	if err != nil {
		return "", nil, err
	}
	settings, issues := domain.DecodeLabSettings(restored)
	if len(issues) != 0 || !domain.CredentialsFromSettings(settings).Ready() {
		return "", nil, errors.New("restored credentials do not match the configuration")
	}
	for _, spec := range keyMaterialSpecs {
		if _, _, err := readRecoveryFile(filepath.Join(stage, spec.publicPath), maximumKeyMaterialBytes); err != nil {
			return "", nil, errors.New("restored public keys must be regular deployment files")
		}
	}
	for _, key := range (Local{}).KeyMaterial(ctx, stage) {
		if !key.Verified || !key.Matches || !key.Safe {
			return "", nil, fmt.Errorf("restored %s key does not match the repository's public key", key.Name)
		}
	}
	// Ensure future routine Git operations cannot accidentally add clear keys.
	exclude := filepath.Join(stage, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0700); err != nil {
		return "", nil, err
	}
	f, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return "", nil, err
	}
	_, err = f.WriteString("\n/secret-key\n/admin-ssh\n/lab-credentials.json\n")
	err = errors.Join(err, f.Close())
	if err != nil {
		return "", nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", nil, err
	}
	trust := filepath.Join(home, ".ssh", "nixorium-known-hosts")
	// Lock the same file used by client enrollment and reviewed trust rotation.
	// An activated replacement controller normally already has an empty store.
	if err := ensureRealDirectory(filepath.Join(home, ".ssh"), 0700, "SSH directory"); err != nil {
		return "", nil, err
	}
	if err := ensureRealDirectory(trust, 0700, "trusted computer keys"); err != nil {
		return "", nil, err
	}
	trustLock, err := lockRecoveryTrust(trust)
	if err != nil {
		return "", nil, err
	}
	defer trustLock.Close()
	current, err := recoveryMaterial(stage)
	if err != nil || !compatibleTrustedKeys(current.KnownHosts, knownHosts) {
		return "", nil, errors.New("this controller already has different trusted computer keys; restore on a clean replacement controller")
	}
	revision, err = (Local{}).GitRevision(ctx, stage)
	if err != nil {
		return "", nil, err
	}
	if beforePublish != nil {
		if err := beforePublish(); err != nil {
			return "", nil, err
		}
	}
	if err := unix.Renameat2(unix.AT_FDCWD, stage, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE); err != nil {
		return "", nil, err
	}
	if err := syncDirectory(filepath.Dir(target)); err != nil {
		return revision, err, nil
	}
	if err := restoreTrustedKeys(trust, knownHosts); err != nil {
		return revision, err, nil
	}
	return revision, nil, nil
}

// Missing entries and empty files are safe to populate. A different nonempty
// file is a trust conflict, including retained evidence from another lab.
func compatibleTrustedKeys(current, wanted map[string][]byte) bool {
	for name, data := range current {
		if len(bytes.TrimSpace(data)) > 0 && !bytes.Equal(data, wanted[name]) {
			return false
		}
	}
	return true
}

func lockRecoveryTrust(directory string) (*os.File, error) {
	if err := ensurePrivateOwnedDirectory(directory); err != nil {
		return nil, err
	}
	name := filepath.Join(directory, ".nixorium-known-hosts.lock")
	fd, err := unix.Openat2(unix.AT_FDCWD, name, &unix.OpenHow{Flags: unix.O_RDWR | unix.O_CREAT | unix.O_CLOEXEC | unix.O_NONBLOCK, Mode: 0600, Resolve: unix.RESOLVE_NO_SYMLINKS})
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if err := validatePrivateOwnedFile(file, unix.S_IFREG, 0600); err != nil {
		file.Close()
		return nil, err
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		return nil, errors.New("trusted computer keys are being updated; retry restore when that operation finishes")
	}
	return file, nil
}

// Caller holds the managed trust lock. Only absent, empty or identical files
// can be filled; existing verified identities are never silently replaced.
func restoreTrustedKeys(target string, files map[string][]byte) error {
	for name, data := range files {
		destination := filepath.Join(target, name)
		old, _, err := readRecoveryFile(destination, maximumKeyMaterialBytes)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && len(bytes.TrimSpace(old)) > 0 {
			if !bytes.Equal(old, data) {
				return errors.New("trusted computer keys changed during restore")
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(destination), ".restore-trust-*")
		if err != nil {
			return err
		}
		_, writeErr := file.Write(data)
		err = errors.Join(writeErr, file.Sync(), file.Close())
		if err == nil {
			err = os.Rename(file.Name(), destination)
		}
		_ = os.Remove(file.Name())
		if err != nil {
			return err
		}
		if err := syncDirectory(filepath.Dir(destination)); err != nil {
			return err
		}
	}
	return syncDirectory(target)
}

// Recovery inputs must not traverse symlinks, and a FIFO must never hang the TUI.
func readRecoveryFile(name string, limit int64) ([]byte, uint32, error) {
	fd, err := unix.Openat2(unix.AT_FDCWD, name, &unix.OpenHow{Flags: unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NONBLOCK, Resolve: unix.RESOLVE_NO_SYMLINKS})
	if err != nil {
		return nil, 0, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, 0, errors.New("recovery input is not a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if len(data) > int(limit) {
		return nil, 0, errors.New("recovery input exceeds its limit")
	}
	return data, uint32(info.Mode().Perm()), err
}

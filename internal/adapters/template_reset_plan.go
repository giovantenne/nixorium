package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/giovantenne/nixorium/internal/domain"
	"golang.org/x/sys/unix"
)

func (TemplateReset) PrepareTemplateReset(ctx context.Context, repository, preset string, progress func(string)) domain.TemplateResetPlan {
	p := domain.TemplateResetPlan{Repository: repository, State: "blocked"}
	fail := func(err error) domain.TemplateResetPlan { p.Message = err.Error(); return p }
	emit := func(message string) {
		if progress != nil {
			progress(message)
		}
	}
	root, err := openWorkspaceRoot(repository, unix.LOCK_SH)
	if err != nil {
		return fail(err)
	}
	defer root.Close()
	emit("Inspecting committed files and preserved private paths")
	source, err := inspectResetRepository(ctx, root)
	if err != nil {
		return fail(err)
	}
	emit("Loading the template from the exact locked framework revision")
	files, snapshot, err := lockedResetTemplate(ctx, repository)
	if err != nil {
		return fail(err)
	}
	if !bytes.Equal(snapshot.FlakeContent, source.Original["flake.nix"].Data) || !bytes.Equal(snapshot.LockContent, source.Original["flake.lock"].Data) {
		return fail(errors.New("deployment inputs changed during inspection"))
	}
	candidate, selected, err := prepareResetFiles(source.Original, files, snapshot, preset)
	if err != nil {
		return fail(err)
	}
	p.Revision, p.UpstreamRevision, p.Preset = source.Revision, snapshot.CurrentRev, selected
	p.Proposal = source
	p.Proposal.UpstreamRevision, p.Proposal.Candidate = snapshot.CurrentRev, candidate
	p.Changes, p.Preserved = resetChanges(source.Original, candidate)
	if software, err := domain.DecodeLabSoftware(source.Original["lab-software.json"].Data); err == nil {
		p.PreviousSoftware = software.Packages
	}
	if _, err := domain.TemplateResetToken(p); err != nil {
		return fail(err)
	}
	emit("Evaluating the isolated candidate, guided home and protected system identity")
	directory, err := isolatedResetCandidate(ctx, candidate)
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(directory)
	if err := validateResetCandidate(ctx, repository, directory); err != nil {
		return fail(err)
	}
	current, err := inspectResetRepository(ctx, root)
	if err != nil {
		return fail(err)
	}
	if resetFileDigest(source) != resetFileDigest(current) {
		return fail(errors.New("deployment changed during review; create a new proposal"))
	}
	p.Checks = []domain.UpdateCheck{
		{ID: "pin", State: "passed", Message: "Exact framework revision, package base and complete lock preserved"},
		{ID: "identity", State: "passed", Message: "Host identity, network, disk settings, accounts, stateVersion and active public keys preserved"},
		{ID: "workspace", State: "passed", Message: "Guided home enabled; both candidate hooks and representative systems evaluated"},
		{ID: "activation", State: "separate", Message: "No system build, activation, deployment or reboot; use the normal reviewed operations afterwards"},
	}
	p.State = "ready"
	return p
}

func resetChanges(original, candidate map[string]domain.TemplateFile) ([]domain.TemplateResetChange, []string) {
	changes := []domain.TemplateResetChange{}
	preserved := []string{}
	for name, old := range original {
		next, present := candidate[name]
		switch {
		case domain.TemplateResetPreserves(name):
			preserved = append(preserved, name)
		case !present:
			changes = append(changes, domain.TemplateResetChange{Path: name, Action: "remove"})
		case old.Mode != next.Mode || !bytes.Equal(old.Data, next.Data):
			changes = append(changes, domain.TemplateResetChange{Path: name, Action: "replace"})
		}
	}
	for name := range candidate {
		if _, present := original[name]; !present {
			changes = append(changes, domain.TemplateResetChange{Path: name, Action: "add"})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	sort.Strings(preserved)
	return changes, preserved
}

// Only reviewed tracked bytes enter this disposable repository. Never copy the
// deployment directory: ignored credentials must not enter Git or the store.
func isolatedResetCandidate(ctx context.Context, files map[string]domain.TemplateFile) (string, error) {
	if err := domain.ValidateTemplateFiles(files); err != nil {
		return "", err
	}
	directory, err := os.MkdirTemp("", "nixorium-template-candidate-*")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(directory)
		}
	}()
	for name, file := range files {
		if filepath.Base(name) == ".gitattributes" || privateKeyContent.Match(file.Data) {
			return "", errors.New("template attributes or tracked private-key material require manual review")
		}
		full := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(full), 0700); err != nil {
			return "", err
		}
		if file.Mode == "120000" {
			err = os.Symlink(string(file.Data), full)
		} else {
			mode := os.FileMode(0600)
			if file.Mode == "100755" {
				mode = 0700
			}
			err = os.WriteFile(full, file.Data, mode)
		}
		if err != nil {
			return "", err
		}
	}
	for _, args := range [][]string{{"init", "--quiet", "--template="}, {"add", "--force", "--all"}, {"commit", "--quiet", "--no-verify", "-m", "Prepare isolated deployment template"}} {
		if _, err := resetGit(ctx, directory, nil, 4096, args...); err != nil {
			return "", err
		}
	}
	ok = true
	return directory, nil
}

// Compare effective safety-critical values, not just preserved settings bytes:
// removing a local module can otherwise change disks, identity or key bindings.
const resetIdentityExpression = `let
  meta = f.labMeta;
  names = [ meta.controller.name ] ++ map (h: h.name) meta.clients.hosts;
  project = name: let c = f.nixosConfigurations.${name}.config; in {
    inherit (c.system) stateVersion;
    inherit (c.networking) hostName;
    network = {
      useDHCP = c.networking.useDHCP or null;
      defaultGateway = c.networking.defaultGateway or null;
      defaultGateway6 = c.networking.defaultGateway6 or null;
      nameservers = c.networking.nameservers or [];
      interfaces = builtins.mapAttrs (_: interface: {
        useDHCP = interface.useDHCP or null;
        ipv4 = { addresses = interface.ipv4.addresses or []; routes = interface.ipv4.routes or []; };
        ipv6 = { addresses = interface.ipv6.addresses or []; routes = interface.ipv6.routes or []; };
      }) (c.networking.interfaces or {});
    };
    disks = builtins.mapAttrs (_: disk: disk.device) c.disko.devices.disk;
    diskProvisioner = c.system.build.diskoScript.drvPath;
    fileSystems = builtins.mapAttrs (_: fs: { inherit (fs) device fsType options; }) c.fileSystems;
    swapDevices = c.swapDevices;
    users = builtins.mapAttrs (_: u: { inherit (u) uid group home hashedPassword; })
      (builtins.intersectAttrs { admin = null; ${meta.users.student} = null; ${meta.users.teacher} = null; } c.users.users);
    sshKeys = c.users.users.admin.openssh.authorizedKeys.keys;
    cacheKeys = c.nix.settings.trusted-public-keys;
  };
in {
  metadata = builtins.removeAttrs meta [ "version" "clients" ] // {
    clients = builtins.removeAttrs meta.clients [ "groups" ];
  };
  hosts = builtins.listToAttrs (map (name: { inherit name; value = project name; }) names);
}`

func validateResetCandidate(ctx context.Context, repository, candidate string) error {
	// Keep hashes out of the proposed tree, while evaluating both sides with
	// the same local credentials. The candidate is an isolated private checkout.
	_, credentials, credentialErr := readCredentials(repository)
	if credentialErr == nil {
		path := filepath.Join(candidate, credentialsFile)
		if err := writeRegularFileCreateNew(path, credentials, 0600); err != nil {
			return err
		}
		defer os.Remove(path)
	} else if !errors.Is(credentialErr, os.ErrNotExist) {
		return credentialErr
	}

	before, err := resetNix(ctx, repository, resetIdentityExpression)
	if err != nil {
		return errors.New("cannot evaluate existing system identity safely; repair or migrate custom configuration manually")
	}
	after, err := resetNix(ctx, candidate, resetIdentityExpression)
	if err != nil {
		return fmt.Errorf("evaluate candidate identity: %w", err)
	}
	if !bytes.Equal(before, after) {
		return errors.New("template would change protected host/network/disk/account/key settings; migrate custom configuration manually")
	}
	data, err := resetNix(ctx, candidate, resetValidationExpression)
	if err != nil {
		return fmt.Errorf("validate candidate systems and home: %w", err)
	}
	var evaluated struct {
		Meta   domain.LabMeta
		Status domain.DeploymentStatus
	}
	if json.Unmarshal(data, &evaluated) != nil {
		return errors.New("invalid candidate readiness metadata")
	}
	if _, _, err := updateCandidateChecks(evaluated.Meta, evaluated.Status); err != nil {
		return errors.New("candidate is not ready; check saved settings and public keys before resetting")
	}
	lock, err := os.ReadFile(filepath.Join(candidate, "flake.lock"))
	if err != nil {
		return err
	}
	originalLock, _, err := readRegularFileNoFollowLimit(filepath.Join(repository, "flake.lock"), 4*1024*1024)
	if err != nil || !bytes.Equal(lock, originalLock) {
		return errors.New("template reset must not update flake.lock")
	}
	return nil
}

const resetValidationExpression = `let
  raw = builtins.readFile (f.outPath + "/workspace-profile.json");
  resolved = f.nixoriumResolveWorkspaceCandidate raw;
  names = [ f.labMeta.controller.name ] ++ (if f.labMeta.clients.hosts == [] then [] else [(builtins.head f.labMeta.clients.hosts).name]);
  systems = map (name: f.nixosConfigurations.${name}.config.system.build.toplevel.drvPath) names;
in assert f.nixoriumValidateWorkspaceCandidate raw == true;
assert f.nixoriumValidateCandidate (builtins.fromJSON (builtins.readFile (f.outPath + "/lab-settings.json"))) == true;
assert f.nixoriumValidateSoftwareCandidate (builtins.fromJSON (builtins.readFile (f.outPath + "/lab-software.json"))) == true;
builtins.deepSeq [resolved systems] { meta = f.labMeta; status = f.deploymentStatus; }`

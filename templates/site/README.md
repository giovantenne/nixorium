# Nixorium deployment

This private repository contains one lab's configuration. The reusable system
implementation is pinned as the `nixorium` Flake input.

The repository includes the `nixorium-maintainer` Agent Skill and discovery
links for Codex, OpenCode, Claude Code and Pi. Agents can load it automatically
for lab configuration, validation and upstream-update work.

> [!IMPORTANT]
> Keep this repository private. It contains password hashes and internal
> network details. Keep `secret-key`, `admin-ssh`, and
> `veyon-private-key.pem` outside Git.

## Contents

- [First setup](#first-setup)
- [Local customization](#local-customization)
- [Occasional interventions](#occasional-interventions)
- [Updating Nixorium](#updating-nixorium)
- [Troubleshooting and recovery](TROUBLESHOOTING.md)

## First setup

Development compatibility: `lab-settings.json` accepts optional
`lab.deploymentMode` (`laboratory` by default, or `controller`). Controller-only
mode requires `pcCount: 0`; it permits local controller activation with secure
account credentials, without lab keys or the client DHCP hint. Lab networking,
cache and remote-control services are inactive; fleet readiness remains false.
Existing deployments are not migrated automatically. The current template,
installer and setup TUI still use the laboratory flow below. Do not use this
mode to disable an existing fleet; guided migration is not implemented yet.
Older upstreams reject the new setting, so upgrade before opting in.

After the controller's first reboot, sign in as `admin` and run:

```sh
cd ~/nixorium-deployment
nix run .#nixorium -- setup
```

The configuration wizard collects the required network, laboratory, account,
regional, browser, Veyon, and password settings. It proposes detected network
values, retries recoverable password mistakes without losing earlier answers,
then collects all three passwords in one protected session. The complete
candidate is validated, reviewed, and saved once before setup continues. It
then creates the three key pairs and installs their private portions through
the fixed privileged action.

It then opens a resumable first-run checklist. Follow the highlighted next
step with `Enter`:

1. **Review and save configuration.** Review the complete redacted settings
   proposal and save it. Nixorium records the managed configuration locally;
   repository mechanics and private keys are not exposed in this flow.
2. **Activate the controller.** Review the exact revision, type its displayed
   confirmation, and wait for the verified result. Press `Enter` to return to
   the setup checklist.
3. **Prepare installation files.** Press `Enter`; progress and recent activity
   remain visible while Nix builds the netboot artifacts and client systems.
4. **Install the first computer.** Open network installation, start PXE after
   its explicit network review, choose a pilot identity from the saved
   inventory, and boot that client from UEFI network boot. After the local
   identity-and-disk confirmation and installed-disk boot, press `v` on the
   controller to verify authenticated active-revision evidence. Complete the
   separate practical desktop check, then install another client or stop PXE
   and defer the rest. Reopening the TUI restores the selected identity and
   completed checks from private operator state when they still match the
   current deployment revision and evaluated inventory.

You can press `q` at any safe point. While PXE is active, leaving it active is a
separate exact-confirmation choice; stopping PXE restores normal controller
networking. Running the setup command again observes
Git, keys, the active controller, and prepared artifacts, then resumes at the
first incomplete stage instead of repeating completed work. Installation
evidence is stored outside Git under the administrator's private state
directory; it records a past authenticated check, not current reachability or
permission to erase a disk.

The bootstrap installer already created and committed this private deployment.
It preserved the selected update channel in `flake.nix` while binding the
initial template, installer, Disko layout, and lock to one immutable upstream
revision.
When using the template manually instead, create a private Git repository and
commit the initial template first because Flakes include only tracked files.
Before production, pin the Nixorium input to a released tag. Optional branding
and NixOS policy can be added later under `assets/` and `modules/`.

This template owns its `nixpkgs` pin directly. Nixorium and its Disko/Veyon
inputs follow that same package base, so controller and client systems cannot
drift onto a second implicit pin. `Update Nixorium` preserves the package-base
lock node. Do not change `nixos-26.05` to another channel without an explicitly
compatible Nixorium release and complete validation; a guided package-base
update is not implemented yet.

## Local customization

- `modules/shared.nix`: every machine
- `modules/controller.nix`: controller only
- `modules/clients.nix`: client PCs only
- `lab-software.json`: guided packages with explicit shared, controller or client scopes
- `clientGroups` in `flake.nix`: named client scopes used by guided software
- `hostModules` in `flake.nix`: individual hosts
- `assets/logo.txt`: screensaver logo

<details>
<summary>Example: customize pc05 and add backgrounds</summary>

To customize only `pc05`, create `modules/pc05.nix`:

```nix
{ pkgs, ... }:
{
  environment.systemPackages = [ pkgs.vlc ];
}
```

Then reference that file from the existing `hostModules` attribute in
`flake.nix`:

```nix
hostModules = {
  pc05 = [ ./modules/pc05.nix ];
};
```

The host name must be generated by the deployment; for example, `pc05`
requires `pcCount` to be at least `5`. Track both files, validate the host, and
deploy only that machine:

```sh
git add flake.nix modules/pc05.nix
nix build .#nixosConfigurations.pc05.config.system.build.toplevel --no-link
colmena apply --on pc05
```

Additional backgrounds can be passed from `flake.nix`:

```nix
assets = {
  logo = ./assets/logo.txt;
  backgrounds = [
    ./assets/backgrounds/one.jpg
    ./assets/backgrounds/two.jpg
  ];
};
```

Keep referenced keys, assets and NixOS module files inside this repository.
`mkLab` includes this source tree in the offline PXE installer, so no external
checkout is needed while installing clients without internet access.

</details>

## Occasional interventions

Run the task-oriented dashboard from the repository root:

```sh
nix run .#nixorium
```

The opening screen asks which intervention you intend to perform. It does not
scan clients or treat powered-off computers as unhealthy. `Up`/`Down` selects
Restore, guided software changes, distribution, network installation, Update
Nixorium, reviewed client shutdown, or Advanced tools. `?` opens help; `F1`
also works in text fields.
Advanced Computer inventory performs the explicit client check and supports `/`
search, Enter for detail, `t` for technical evidence, `d` for a focused
deployment, and `i` for diagnostics.
Every state has a symbol and text as well as semantic color. Each workflow displays its
own available keys; `Esc` returns to the previous screen and `q` quits outside
text-entry fields when no operation requires attention. Computers, deployment, controller, services,
logs, Git, updates, and network installation reuse the same title, section,
status, and width-adaptive key-help hierarchy. Completed Settings, service,
Git, update, deployment, and controller operations show a compact result with
the relevant next action rather than dropping back into the prior form or list.
The network-installation screen remains state-oriented: it highlights one next
step—prepare files, start PXE, boot/install a client, or recover networking—and
shows only controls relevant to the observed mode.

Long PXE, controller, and deployment work displays typed phase progress.
Operations without a meaningful percentage—such as host checks, settings
validation, Git/service loading, or update planning—display a shared animated
spinner and the current plain-language action.

| Dashboard task | Purpose |
|---|---|
| **Restore computers** | Choose non-destructive reapply or select an evaluated identity for a locally confirmed disk-erasing reinstall |
| **Add or change software** | Select a supported pinned package and save its reviewed client scope |
| **Distribute the prepared system** | Plan and apply one or more client configurations |
| **Computer inventory** | Explicitly inspect authenticated client state |
| **Rebuild controller** | Review and activate the controller configuration |
| **Manage services** | Inspect PXE and restart the signed cache |
| **View operation logs** | Browse private deployment logs and action history |
| **Review Git changes** | Review and optionally commit selected safe paths |
| **Change settings** | Edit and validate one grouped configuration area or one account password |
| **Update Nixorium** | Fetch upstream `master` and releases, then validate and apply one selected target |
| **Shut down computers** | Check sessions and send reviewed power-off requests to selected clients only |
| **Install or reinstall computers** | Prepare, start, stop, or recover PXE mode |

The initial dashboard and `status` are local and do not probe clients. Add
`--json` to supported CLI commands for structured output. Use `doctor` for
actionable diagnostics and `doctor --full` only when a real controller build is
needed.

For symptom-first recovery, safe retry rules, and backup boundaries, see
[TROUBLESHOOTING.md](TROUBLESHOOTING.md).

### Change settings

Open **Change settings** for routine Network, Computers, Accounts, Regional,
Browser, Git, or Veyon changes. Regional fields reuse the offline searchable
selectors from first-run setup. Git author names and email addresses live here
instead of extending the initial setup path.

Choose `p` in Settings to change exactly one administrator, teacher, or student
password. Nixorium temporarily suspends the dashboard and uses the same
terminal-only no-echo, confirmed, retryable hashing flow as first-run setup;
plaintext never enters the Bubble Tea model. Both ordinary and password edits
return to one Nix-validated, redacted review before the fingerprint-bound atomic
replacement of `lab-settings.json`.

The TUI saves the managed file and its local history as one operation. Then
rebuild the controller and deploy affected clients as appropriate. The Settings
action does not push, rebuild, activate, or deploy implicitly.

### Add or change software

Open **Add or change software** to review configured choices, browse common
suggestions, or search the wider package set locked by this deployment. Choose
whether the declaration applies to this controller and all current/future
clients (`shared`), only this controller (`controller`), every client without
the controller (`all-clients`), an evaluated group, or selected clients.
The first two choices require an upstream exposing controller-software support;
older upstreams retain their client-only choices. Existing declarations are not
migrated or expanded automatically. Clients may all remain powered off, and
shared/controller declarations work before any clients are configured.

The same typed workflow is available from the CLI:

```sh
nix run .#nixorium -- software catalog
nix run .#nixorium -- software search --query libreoffice
nix run .#nixorium -- software plan --package hello --scope shared
nix run .#nixorium -- software plan --package hello --scope controller
nix run .#nixorium -- software plan --package vlc --scope all-clients
nix run .#nixorium -- software plan --package python3Packages.numpy --scope all-clients
nix run .#nixorium -- software plan --package gimp --scope group:graphics
nix run .#nixorium -- software apply --package vlc --scope all-clients \
  --expect REVIEW_TOKEN
```

Groups are declared in `flake.nix` through `clientGroups`; explicit client
scopes accept comma-separated evaluated identities such as
`clients:pc01,pc04`. Use `--remove` with plan and apply to remove a declaration
owned by this workflow. Search and resolution use the deployment's locked input
and overlays; dotted attributes are resolved as data rather than Nix code.

Apply atomically replaces only `lab-software.json` after repeating pinned Nix
validation and checking the review token and source fingerprint. The ordinary
TUI also records that one managed file locally without exposing Git. It does not
push, build, activate the controller, prepare PXE, or distribute clients.
For shared/controller changes, use **Apply controller configuration** after
saving; this manual step remains until the integrated controller-first software
workflow is implemented. Use **Distribute the prepared system** for the
specific powered-on clients you intend to update. Packages supplied by private
NixOS modules remain untouched and are edited through the advanced module
workflow.

The software schema remains version 1 with two new explicit scope kinds; older
upstreams reject them. Before downgrading, review and remove or deliberately
replace those declarations. A scope change reviews both old and new targets.
The upstream controller-validation hook is composed with the existing site
validator, including on removal, so older site Flakes cannot silently skip the
controller check. One package still has one scope; arbitrary per-host exclusions
and removal of built-in/private-module packages are not added by this change.

### Computers

```sh
nix run .#nixorium -- hosts
```

**Computer inventory** performs bounded network and SSH probes. For authenticated
hosts it runs the fixed read-only `nixorium-host-state` helper and compares the
active system path and embedded deployment revision with the desired Git
revision.

- `current`: the host runs the desired revision;
- `outdated`: the authenticated host runs another revision;
- `unknown`: state could not be authenticated or the older generation lacks
  the helper.

The report also shows the most recent successful post-apply verification, but
history never overrides live authenticated state.

### Distribute the prepared system

```sh
nix run .#nixorium -- deploy plan --on pc01
nix run .#nixorium -- deploy plan --on pc01,pc02
nix run .#nixorium -- deploy plan --on @lab
nix run .#nixorium -- deploy apply --on @lab --expect REVISION_FROM_PLAN
```

Planning is read-only. It requires a ready deployment and clean Git revision,
expands only configured clients, and prints the revision-bound apply command.
Apply repeats the preflight, requires `DEPLOY <targets>`, builds before
activation, and streams output to a mode-0600 log. In the dashboard, the same
foreground operation shows elapsed time, named stages and authenticated-computer
verification counts. `l` expands the progress bar and up to five authored
activities. Raw Colmena output remains in the private log instead of being rendered
as terminal UI. Accidental quit stays disabled until the final report appears;
that compact result offers direct dashboard, log, and fresh-review actions.

After every attempt, Nixorium authenticates selected hosts and records only
those running the reviewed revision. A failed apply may leave mixed target
state. Inspect the log and fresh **Computer inventory** results, make a new plan,
and retry; never infer rollback or completion from a lost terminal. `--yes` is
for deliberate automation and never removes the revision check.

### Shut down computers

Open **Shut down computers**, select the intended clients, and continue to run
the preflight. The controller is never selectable. Computers that are off,
unreachable, or lack authenticated management access remain visible as not
sent; Nixorium does not queue a request for later.

By default, an interactive user session blocks that target and unknown session
state is ineligible. The TUI can explicitly acknowledge unknown-session risk
with `u`, which creates a new reviewed plan. An observed active session remains
blocked. The same operation is available from the CLI:

```sh
nix run .#nixorium -- shutdown plan --on pc01
nix run .#nixorium -- shutdown plan --on pc01,pc02
nix run .#nixorium -- shutdown plan --on @lab
nix run .#nixorium -- shutdown apply --on @lab \
  --expect REVIEW_TOKEN
```

Planning checks installation/network recovery and concurrent client work as
well as access and sessions. Apply requires the generated phrase (or explicit
automation-only `--yes`), takes the same client-operation lock as deployment,
and repeats inventory, conflict, and session checks immediately before sending
the fixed operating-system request. Use
`--acknowledge-unknown-sessions` on both plan and apply only after reviewing
that risk.

Results are `accepted`, `not-sent`, or `unconfirmed`. Accepted means the remote
operating system accepted the request; loss of network contact does not prove
physical power state. An unconfirmed result may have taken effect, so inspect
the target instead of retrying blindly.

### Operation logs

```sh
nix run .#nixorium -- logs
nix run .#nixorium -- logs show OPERATION_LOG_ID
```

The log browser:

- lists at most the newest 50 deployment logs;
- reads at most the final 64 KiB of one selected log;
- requires user-owned mode-0700 directories and mode-0600 regular files;
- refuses symlinks and arbitrary paths;
- sanitizes terminal controls before display;
- retains the newest 1,000 typed action summaries without deleting detailed
  deployment logs.

The same list and detail views are available under **View operation logs**.

### Review and commit Git changes

```sh
nix run .#nixorium -- git review
nix run .#nixorium -- git commit plan \
  --paths lab-settings.json,keys/admin-ssh.pub
nix run .#nixorium -- git commit apply \
  --paths lab-settings.json,keys/admin-ssh.pub \
  --expect REVIEW_TOKEN
```

`git review` separates staged, unstaged, and untracked paths; labels managed
and unexpected changes; redacts password hashes; does not open untracked
contents; disables external diff drivers; and refuses known private-key paths
before reading a patch. It never mutates Git.

The optional commit flow accepts only explicit paths. It creates an isolated
HEAD-based proposal, rejects unsafe file types, secrets, transforms, conflicts,
and oversized content, then binds apply to the reviewed token and confirmation.
Unselected worktree/index changes remain untouched. Hooks, signing, remotes,
and push are never invoked.

### Rebuild the controller

```sh
nix run .#nixorium -- controller plan
nix run .#nixorium -- controller apply --expect REVISION_FROM_PLAN
```

Apply requires `REBUILD <controller>`, starts only the matching revision-bound
systemd unit, builds as the deployment owner, refuses repository drift, and
records success only after activation and active-system verification. Closing
the dashboard does not stop the systemd-owned job. The dashboard shows elapsed
time, four typed phases, recent activity, and a progress bar; CLI text/JSON
flows write the same safe activity to stderr. When the job ends, the dashboard
refreshes reconciled state and shows a compact result with explicit actions to
return home, reveal the activity detail, inspect logs, or create a new review.
Use `setup apply` for the equivalent first-run action with identical progress
feedback.

### Manage services

```sh
nix run .#nixorium -- services
nix run .#nixorium -- services restart cache
```

The report combines the persistent signed cache and on-demand PXE lifecycle.
Cache restart requires `RESTART CACHE` and succeeds only after both systemd and
HTTP checks pass. PXE uses its dedicated transactional workflow and cannot be
mutated through this generic service action.

The controller runs Harmonia as `nixorium-harmonia.service`; systemd loads its
private signing key as an isolated credential outside Git and the Nix store.
Detailed Harmonia output uses `journalctl -u harmonia.service`.

### Configuration and first-run setup

```sh
nix run .#nixorium -- setup
nix run .#nixorium -- setup status
nix run .#nixorium -- setup keys
nix run .#nixorium -- setup install-secrets
nix run .#nixorium -- setup apply
```

`setup` records the interface carrying the controller's default route as a
controller-specific override and proposes its live DHCP address (not the
controller's declarative static address), groups its essential
questions by task, and provides searchable offline selectors for time zone,
locale, and keyboard values while retaining validated custom entry. Optional
Git identity is not requested during first run. The wizard supports backward
navigation, collects passwords without echo, retries recoverable password
mistakes in the current account without restarting configuration, validates
the complete candidate, shows a redacted review, writes atomically after
acceptance, and reconciles all three key pairs. It never overwrites existing
key material. Bare `setup` then opens the stage-aware first-run checklist;
explicit `setup configure` stops after configuration. `setup status` observes
the first incomplete stage without trusting a hidden completion flag.

`lab.ifaceName` remains the backward-compatible fallback. Optional
`controllerIfaceName` and `clientIfaceName` select role defaults, while
`hostIfaceNames` can override a configured host. Precedence is host, role, then
fallback. This permits different predictable interface names on controller and
client hardware without changing existing deployments.

`setup install-secrets` starts a fixed sandboxed action that installs only
verified key material to fixed destinations. After reviewed settings and public
keys are committed, `setup apply` requires a clean ready deployment and exact
`APPLY` confirmation. The build runs as `admin`; only exact-closure activation
runs as root. Completion requires the active system and root-owned success
receipt to match the reviewed revision.
While it runs, `setup apply` reports typed validation, build, activation, and
verification progress on stderr; verbose Nix output remains in journald and
JSON stdout stays machine-clean.

For machine-managed settings changes:

```sh
nix run .#nixorium -- config validate
nix run .#nixorium -- config plan --file candidate.json
nix run .#nixorium -- config apply --file candidate.json \
  --expect 'sha256:fingerprint-from-plan'
```

The candidate passes both the management schema and deployment Flake. Plan
shows a non-secret semantic diff; apply locks and rechecks the managed file,
then changes only `lab-settings.json` if the reviewed fingerprint still
matches.

### Install computers with PXE

#### Prepare

```sh
nix run .#nixorium -- pxe prepare
journalctl -u nixorium-prepare-pxe.service
```

Preparation is non-disruptive and safe to retry. It requires clean ready Git,
a healthy cache, and one usable live non-static controller address. It builds
the kernel, initrd, iPXE firmware/script, and every client closure, then records
their immutable store paths and Git revision under
`/var/lib/nixorium/prepared/prepared.json` with managed GC roots.

The dashboard follows the systemd-owned job in place: it shows the current
phase, elapsed time and counter. `l` expands the progress bar and five most
recent bounded activities. Closing the dashboard does not cancel the job.
Use the journal command above only when verbose Nix output is needed for
troubleshooting.

An unambiguous DHCP lease change is captured without a configuration commit.
Multiple usable addresses fail closed without replacing the prior preparation.

#### Start, stop, or recover

```sh
nix run .#nixorium -- pxe start
nix run .#nixorium -- pxe stop
nix run .#nixorium -- pxe recover
```

Start validates preparation, live addressing, cache, and services before
requiring `START PXE`. Stop restores normal controller addressing. Recover
reconciles an interrupted session, and boot recovery performs the same repair
automatically. These actions are idempotent; a failed start rolls back before
returning. Quitting the dashboard does not stop active systemd-owned services.

The firewall exposes SSH, mDNS, Veyon, optional VNC, Harmonia, and PXE only on
the configured interface and roles. Institutional DHCP remains authoritative.

#### Enroll a client

On the PXE-booted client:

```sh
/installer/setup.sh
```

The installer displays hardware and writable disks, offers only configured
host identities, and refuses an identity that answers its best-effort network
probe. Silence is not treated as a reservation. It verifies the selected
closure offline and requires its size plus 2 GiB of headroom before offering a
disk.

Disko starts only after an exact confirmation such as `ERASE /dev/sda INSTALL
pc05`. The installer reports partition, installation, and verification stages,
then offers a separately confirmed reboot. There is no unattended mode and no
client-side fallback fetch.

<details>
<summary>Complete CLI command reference</summary>

```sh
nix run .#nixorium -- status
nix run .#nixorium -- hosts
nix run .#nixorium -- doctor
nix run .#nixorium -- doctor --full
nix run .#nixorium -- deploy plan --on pc01
nix run .#nixorium -- deploy plan --on @lab
nix run .#nixorium -- deploy apply --on @lab --expect REVISION_FROM_PLAN
nix run .#nixorium -- shutdown plan --on @lab
nix run .#nixorium -- shutdown apply --on @lab --expect REVIEW_TOKEN
nix run .#nixorium -- controller plan
nix run .#nixorium -- controller apply --expect REVISION_FROM_PLAN
nix run .#nixorium -- services
nix run .#nixorium -- services restart cache
nix run .#nixorium -- logs
nix run .#nixorium -- logs show OPERATION_LOG_ID
nix run .#nixorium -- git review
nix run .#nixorium -- git commit plan --paths PATHS
nix run .#nixorium -- git commit apply --paths PATHS --expect REVIEW_TOKEN
nix run .#nixorium -- update check
nix run .#nixorium -- update plan --target master
nix run .#nixorium -- update plan --target v2.0.0
nix run .#nixorium -- update apply --target v2.0.0 --expect REVIEW_TOKEN
nix run .#nixorium -- setup
nix run .#nixorium -- setup status
nix run .#nixorium -- setup keys
nix run .#nixorium -- setup install-secrets
nix run .#nixorium -- setup apply
nix run .#nixorium -- config validate
nix run .#nixorium -- config plan --file candidate.json
nix run .#nixorium -- config apply --file candidate.json --expect FINGERPRINT
nix run .#nixorium -- pxe prepare
nix run .#nixorium -- pxe start
nix run .#nixorium -- pxe stop
nix run .#nixorium -- pxe recover
```

</details>

## Updating nixorium

### Guided update

Use the guided workflow from this private deployment repository:

```sh
nix run .#nixorium -- update check
nix run .#nixorium -- update plan --target v2.0.0
nix run .#nixorium -- update apply --target v2.0.0 --expect REVIEW_TOKEN
```

`update check` is the only command that enumerates the configured public
upstream. It disables Git credential prompting and helpers, stops after 15
seconds, bounds remote output, and lists the `master` development branch plus at
most the newest 20 stable and 20 prerelease tags separately. It does not change
the repository. Skip it and use an explicit target when the controller is
offline.

Planning keeps the configured upstream identity, accepts exactly `master` or a
SemVer release, generates the candidate lock outside the checkout, evaluates
readiness, and builds outputs for the configured capability. An explicit
controller-only deployment validates controller readiness and builds only the
controller candidate. A laboratory deployment, including legacy metadata,
retains the representative controller/client/netboot/firmware/installer set.
Unknown modes, a client inventory in controller mode, or missing explicit
controller readiness are rejected.
Prereleases require `--allow-prerelease`; known downgrades require
`--allow-downgrade`. Apply repeats validation and changes only `flake.nix` and
`flake.lock`; review and optionally commit them separately. It never branches,
commits, pushes, activates, starts PXE, or deploys clients.
The TUI's **Update Nixorium** intervention first fetches this bounded release
list. `master` is clearly marked as the Development branch, stable releases are
shown by default, and prereleases require explicit disclosure. The TUI has no
editable target and does not offer downgrades. After selection, review the
scrollable two-file patch (`F4` expands candidate checks), then type the exact
confirmation shown.

> [!IMPORTANT]
> Updating these files does not activate the controller or deploy clients.
> Review and commit the result, then run those operations separately.

<details>
<summary>Advanced manual fallback for unsupported input declarations</summary>

For a computed input declaration that the managed workflow conservatively
refuses, use this advanced manual fallback. In this example the
new upstream release is `v2.0.0-beta.4`; replace it with the tag you actually
want to install:

```sh
git switch master
git pull --ff-only
git switch -c upgrade/nixorium-v2.0.0-beta.4
```

Open `flake.nix` and change the `inputs.nixorium.url` line so that it contains
the new release tag:

```nix
inputs.nixorium.url = "github:giovantenne/nixorium/v2.0.0-beta.4";
```

Update only that input, review the lock-file change and validate every role:

```sh
nix flake update nixorium
git diff -- flake.nix flake.lock

nix build .#nixosConfigurations.pc01.config.system.build.toplevel --no-link
CONTROLLER_NAME=$(nix eval .#labMeta.controller.name --raw --no-write-lock-file)
nix build ".#nixosConfigurations.${CONTROLLER_NAME}.config.system.build.toplevel" --no-link
nix build .#nixosConfigurations.netboot.config.system.build.netbootRamdisk --no-link
nix build .#pxeFirmware --no-link
nix build .#installerBundle --no-link
```

If every build succeeds, commit and merge the tested upgrade:

```sh
git add flake.nix flake.lock
git commit -m "chore: update nixorium to v2.0.0-beta.4"
git switch master
git merge --ff-only upgrade/nixorium-v2.0.0-beta.4
git push origin master
```

</details>

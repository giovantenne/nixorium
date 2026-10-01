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

`lab-settings.json` accepts optional
`lab.deploymentMode` (`laboratory` by default, or `controller`). Controller-only
mode requires `pcCount: 0`; it permits local controller activation with secure
account credentials, without lab keys or the client DHCP hint. Lab networking,
cache and remote-control services are inactive; fleet readiness remains false.
Existing deployments are not migrated automatically. New controller bootstrap
sets this mode after collecting keyboard, time zone, accounts, and passwords,
then asks for the initial software profile before the first build or disk
change. The selected profile becomes ordinary `shared` declarations in
`lab-software.json`; bootstrap includes the complete profile without asking for
package IDs, and it does not remain an active policy.
Use the official [NixOS Minimal ISO](https://nixos.org/download/#nixos-iso) in
UEFI mode for controller bootstrap. It provides the expected Linux text
console: keyboard is the first settings prompt, the selected console keymap is
applied, and time zone follows immediately before account details. The
bootstrap stops if activation fails or the terminal belongs to an unverifiable
graphical session; a terminal inside the Graphical ISO is not the supported
path.
Do not use it to disable an existing fleet without a reviewed migration.
Older upstreams reject the new setting, so upgrade before opting in.
The public Nix evaluator and the management command both reject empty required
regional/Git values, malformed homepage URLs, and unknown Veyon host names.

The controller is named `pc99` by default because
`lab.masterHostNumber` starts at `99`. This setting is not a fixed controller
identity: changing it to `42`, for example, produces the hostname `pc42` and
also moves the controller's static laboratory address to host number `42` in
the configured subnet. Change it from **Maintenance → Change settings →
Computers → Controller host number**, then save and rebuild the controller.
The number must be greater than `pcCount` and must fit inside the subnet; choose
it before rolling out clients when possible to avoid an unnecessary controller
rename later.

The student session uses the network configured by the system but cannot change
NetworkManager connections, radios, DNS, or other host network state through
GNOME, `nmcli`, or `nmtui`. Teacher and administrator accounts retain network
management access. On the controller, the teacher can also run `nixorium` from
their own home to open a restricted classroom dashboard: computer inventory,
temporary Internet control, and reviewed shutdown/restart actions only. The
teacher never receives read access to the administrator's private deployment;
configuration, software, Git, installation, deployment, updates, logs and
controller maintenance remain unavailable. Student accounts cannot access the
classroom worker. Keep these role boundaries unless the deployment has an
explicitly reviewed reason to relax them.

After the controller's first reboot, sign in as `admin` and run:

```sh
cd ~/nixorium-deployment
nixorium
```

The controller is already usable. Open **Installation** and choose
**Network boot (PXE)** or **USB over SSH**. PXE has one screen for preparation,
starting, finishing and network recovery. Opening it only checks current state;
Enter offers the appropriate next step. If files are missing or stale, the
guided preparation reviews lab settings, saves them, creates missing keys,
activates the controller and prepares client systems. Esc returns to Installation.
A ready system goes straight to the reviewed PXE start; an active or interrupted
session offers finish/recovery without repeating setup.

The existing controller time zone and keyboard are reused. Importing existing
keys is available under **Maintenance → Change settings → Controller keys**.
PXE asks for confirmation before temporarily changing the controller network;
the client installer confirms identity and disk erasure locally. USB/SSH selects
one configured identity, verifies the live Minimal ISO's physical fingerprint,
and requires its own content-bound disk review before dispatch.

You can press `q` at any safe point. While PXE is active, leaving it active is a
separate exact-confirmation choice; stopping PXE restores normal controller
networking. Running the installation flow again revalidates the settings and
skips already current prerequisites before preparing the clients.
On supporting versions, complete saved PXE settings appear in a compact summary:
continue without replaying the form, or choose Edit. Incomplete settings still
open the form; the summary never replaces the remaining preparation checks.

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
lock node. `Update system and packages` advances that base separately; changing
channel requires an explicit target and acknowledgement of unverified runtime
compatibility, not a new Nixorium release. Actual evaluation/build failures
still block. See [UPDATES.md](UPDATES.md) for the complete TUI/CLI journey,
one-time adoption for older deployments, independent packages and recovery.

## Opening Nixorium

On the controller, the administrator and the teacher open **Nixorium** from
the application grid, or type `nixorium` in a terminal. The window stays open
after an error so the message can be read. The administrator's console and SSH
logins also say whether something waits, such as an interrupted client update
or an unfinished USB installation. The teacher sees only classroom controls;
when something blocks them, the screen says whether to try again later or to
ask the administrator, with a short code explained in the
[troubleshooting guide](TROUBLESHOOTING.md#codes-shown-to-the-teacher).

## Native classroom control

Veyon uses PipeWire/Wayland directly on every laboratory computer. Approve the
initial GNOME sharing dialog locally; the grant survives student-home resets.
The controller also needs its own approval when broadcasting the teacher screen.
The external VNC bridge and shared password have been removed. Clients expose
only SSH (22) and Veyon (11100) to the controller's static IPv4 address on the
lab interface; IPv6 cannot bypass this restriction. The deprecated
`veyonNativeHosts` string list remains accepted and ignored for compatibility;
existing settings need no change.

## Temporary Internet access

As `admin` or the configured teacher, open **Computers → Internet access**
to restrict browsing during a lesson,
select clients, choose **block** or **unblock** with Tab, and review before
applying. Internet returns after each client reboots. The lab's IPv4 subnet,
SSH and Veyon remain available; other IPv4/IPv6 destinations are blocked.
Offline or outdated clients are reported and receive no queued command.
Update the controller and clients before first use.

## Local customization

- `modules/shared.nix`: every machine
- `modules/controller.nix`: controller only
- `modules/clients.nix`: client PCs only
- `lab-software.json`: guided packages with explicit shared, controller or client scopes
- `software-catalog.nix`: optional deployment-owned suggestions shown before package search
- `software-presets.json`: seven versioned software profiles whose packages can be added as one reviewed batch
- `modules/workstation.nix`: GNOME desktop icons, persistent dock, application policy, favorites and shortcuts
- `modules/development.nix`: shell, npm and rootless Docker policy
- `modules/home-profile.nix`: MIME defaults and writable per-user VS Code settings/extensions
- `modules/screensaver.nix`: Ghostty/TTE lab screensaver supplied by every built-in profile
- `clientGroups` in `flake.nix`: named client scopes used by guided software
- `hostModules` in `flake.nix`: individual hosts
- `updateValidationHosts` in `mkLab`: extra validation hosts when private shared
  modules branch on host identity (explicit host modules are already covered)
- `assets/logo.txt`: screensaver logo

These files are the lab's workstation profile. They are intentionally part of
this private repository rather than Nixorium core, so package and policy changes
can be reviewed and deployed on the lab's schedule. Profile modules receive
`hostSoftwarePackages`; keep application policy conditional on the corresponding
managed package so scope changes do not leave stale launchers or services.

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

Updating the framework does not refresh copied deployment files. On supporting
versions, **Maintenance → Reset deployment template** replaces those files from
the exact pinned upstream, lets you select a new software preset, and enables
the guided home with an initial profile. It preserves settings, keys, ignored
files and input pins, with a recoverable local backup and explicit loss review.
No activation, client deployment, reboot or push is included. See
[template reset and recovery](DEPLOYMENT-RESET.md) before using it.

The template supplies `workspace-catalog.nix` and an active
`workspace-profile.json` matching Essential without requiring VS Code.
Leave it unchanged for the defaults, or customize it under **Maintenance →
Settings → Student workspace**. There is no separate personalization switch.
Choosing Programming at installation (or in a template reset) copies
`workspace-profile.programming.example.json` over it: VS Code and MySQL
Workbench in the dock, editor defaults, and extensions for web/PHP, C/C++,
Python and Java. Adding Programming later from the software screen leaves the
saved profile unchanged; select VS Code and its extensions in the editor.
The example file is only a reset proposal, never a fallback for a missing or
invalid saved profile. Validation checks preferences and prerequisites for
the student on the controller and every client, including offline evaluation.
Staff behavior remains unchanged.

A reviewed system deployment prepares an immutable seed;
the next normal boot restores it for the student, including controller-only
mode. Rebuilding does not reset an active session. Initial preferences remain
editable until the next reset. The seed has shell/Git defaults, standard XDG
folders with stable English names, and the supported profile settings; arbitrary
legacy assets, editor settings and plugins are not imported automatically.
Explicitly retain the desired supported values in the profile/catalog baseline;
omitted values otherwise fall back to system/application defaults.

Managed snapshots are separate under `/var/lib/home-snapshots/workspace`.
An incomplete reset retains private recovery evidence and blocks normal login;
do not delete evidence, restart the helper, or disable the profile as a retry.
Have an administrator inspect the journal and recovery data first. Snapshots
are not backups. A `nixoriumWorkspace.state` of `prepared`, even with
`runtimeEnabled = true`, is not evidence of deployment or a successful reset.
On supporting pins, `workspace plan --file <candidate.json>` reviews a separate
proposal and `workspace apply --file <candidate.json> --expect <review-token>`
saves only `workspace-profile.json` after confirmation (or explicit `--yes`).
Both support `--json`. Review includes the current/effective preferences, student
account, controller/client destinations and pinned dependencies. Changed source,
catalog, pin or profile requires a new review. Saving does not stage, commit,
build, deploy or reset. See the
[save procedure](skills/nixorium-maintainer/references/student-home.md#review-and-save-a-profile)
for requirements and conflict/durability handling. The administrative TUI exposes
the same save boundary at **Maintenance → Settings → Student workspace**.
Choose Desktop, Dock, VSCode or Browser, keep each change in the
draft, then review the complete proposal and confirm the save. “Inherit” uses
the deployment baseline; “Clear” is an explicit empty list. VSCode → Other
settings adds further editor defaults by name and value, or from a pasted
settings file; settings that can start programs are refused. Saving includes
an automatic local commit of just the profile, then a separate controller
review and client selection; it never deploys automatically. In VSCode →
extensions, `/` searches every packaged extension of the pinned package set;
the catalog only adds prerequisites to common ones. `m` adds an extension
from the Marketplace by its `publisher.name`, pinned to one downloaded version,
and `u` checks those pins for newer versions; both need Internet on the
controller at that moment. Test such third-party extensions on one computer
first. Catalog additions and
unsupported fields remain manual deployment edits. Older pins/commands may not
provide this workflow; the inactive example is never loaded automatically.
Existing profiles are also compared during input-update review: current versus
proposed package/extension versions, dependencies and effective preferences.
Packaged extensions update through Maintenance → Update system and packages,
which can also change the editor, desktop and operating system. Test plugin
loading on a selected client before wider distribution; build success alone
does not qualify it. Session auto-update checks remain disabled in managed
defaults, and saving the same profile does not refresh its pinned extensions.
Supporting pins expose `nixoriumResolveWorkspaceCandidate` to preview a raw JSON
proposal's effective values, versions and destinations without saving it. This
complements, rather than replaces, the deployment's validation hook.
See [student preferences](skills/nixorium-maintainer/references/student-home.md#student-preferences)
for supported settings, retained system policy and boot-time behavior. Keep
private modules from overwriting managed student preferences at login.

Run the task-oriented dashboard from the repository root:

```sh
nixorium
```

The opening screen offers Computers, Installation, Software, and Maintenance.
It does not scan clients or treat powered-off computers as unhealthy.
`Up`/`Down` selects an area and task-local shortcuts appear only inside their
owning area. `?` opens help; `F1` also works in text fields. Computer inventory
performs the explicit client check and supports `/`
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
Press `l` for details, including controller builds started after an update or
software change. Deployment details include recent private command output;
`s` reviews stopping local supervision, not remote activation. An uncertain
apply reserves the fleet until [reviewed recovery](TROUBLESHOOTING.md#interrupted-client-deployment).
Operations without a meaningful percentage—such as host checks, settings
validation, Git/service loading, or update planning—display a shared animated
spinner and the current plain-language action.

| Area | Purpose |
|---|---|
| **Computers** | Inspect, distribute, control Internet access, or shut down/restart selected clients |
| **Installation** | Configure the lab, install through PXE or USB/SSH, and recover interrupted installation state |
| **Software** | Review configured packages, search the pin, choose scope, and save/apply changes |
| **Maintenance** | Change settings, update/rebuild the controller, inspect services, Git, logs, and diagnostics |

Every task menu shows its direct shortcut beside the title. `Esc` returns to
the parent area; `F1` opens contextual help even while typing. Software tabs
are directly accessible with `F2`, `F3`, and `F4`. Reapplying a system belongs
to Computers → Distribute; reinstalling belongs to Installation.

The initial dashboard reads local settings, evaluated inventory and current
services, without checking system closures, keys or PXE artifacts. These checks
run when the relevant operation is opened; startup never implies installation
readiness. Both the dashboard and `status` avoid client probes. Add
`--json` to supported CLI commands for structured output. Use `doctor` for
actionable diagnostics and `doctor --full` only when a real controller build is
needed.

For symptom-first recovery, safe retry rules, and backup boundaries, see
[TROUBLESHOOTING.md](TROUBLESHOOTING.md).

### Change settings

Open **Change settings** for routine Network, Computers, Accounts, Regional,
Browser, or Git changes. Regional fields reuse the offline searchable
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
In **Selected**, Enter opens a package's details: `c` changes where it applies
and `x` reviews its removal. Removing or moving a package that reaches this
controller rebuilds the controller right after saving.

Software profiles and the suggestion list belong to this private repository.
When `software-presets.json` is present, each profile is a versioned list of
package IDs with a label and description; it is a starting selection rather
than persistent policy. Adding one creates ordinary entries in
`lab-software.json`, so later package and scope changes continue through the
same workflow. Existing declarations retain their scopes and selecting another
profile never removes packages. Edit these files to evolve the local choices
without waiting for a Nixorium release. The catalog is convenience only:
package search and pinned-package validation remain available for entries not
listed.

In the Software screen, press **p — Add profile**. Choose one profile, toggle
packages with Space, choose a supported scope, and inspect the aggregated
review. The review marks missing declarations as additions and shows the
preserved scope for packages already present. Enter saves the batch once; Esc
from any selection or review step leaves `lab-software.json` unchanged. If the
catalog is absent, individual package search and management continue normally.

New sites provide **Essential** (the default), **General education**,
**Programming**, **Graphics and illustration**, **Audio and video**, **CAD and
3D modelling**, and **STEM and scientific computing**. The lists are expanded
in the JSON file rather than inheriting from one another. The current package
set exposes Kdenlive as `kdePackages.kdenlive`, so that exact pinned attribute
is used by the audio/video profile. `lab-software.json` initially matches
Essential at `shared` scope; this default affects newly generated repositories
only and does not migrate existing deployments.

Programming adds VS Code, the C/C++, Java, Python and PHP toolchains, a local
MySQL server and MySQL Workbench. That server is for exercises only: it accepts
connections from the same computer, `root` has no password, and all databases
are discarded at every boot, like the student home. Keep work that must
survive as exported SQL files. `modules/development.nix` holds this policy.

Programming also runs Apache with PHP in the XAMPP style: files saved in the
student's `~/public_html` open at `http://localhost/` (for example
`~/public_html/sito/index.php` at `http://localhost/sito/`), with `.htaccess`
rules and folder listings. The server runs as the student account and listens
on this computer only; the folder is emptied with the rest of the home at every
boot, and on the controller it serves the student account's folder.

Every profile includes Git, the Ghostty/TTE lab screensaver and Node.js (and
npm); Programming also includes the Pi and OpenCode coding agents. Git is available to every user and is also a runtime
dependency of the controller management workflows. The screensaver therefore
remains active in Essential as well as the larger profiles. Desktop Icons NG, Dash to Dock and
Tiling Assistant are installed as workstation basics in every profile. The
compact bottom dock hides when a window overlaps it and reappears at the bottom
edge. It stays visible on a clear desktop. On the controller, the administrator
and teacher docks start with the Nixorium launcher. Desktop files appear on the desktop, and
dragging a window to an edge offers an adjacent window with small 8 px gaps.
Yaru-yellow icons, as in Nixorium 1.0.0, complement native Adwaita decorations. Administrator and
teacher accounts start with blue accents and a static vector wallpaper; every
student home reset chooses one of `assets.backgrounds` at random. No blur or
background polling service is used. Super+arrow window shortcuts remain available.

The login helper enables the three required extensions without replacing other
enabled extensions. A one-time migration applies only the managed appearance
keys to existing accounts; staff may then customize them. The separate
`desktop-dock-v1` migration updates only dock visibility for existing accounts. Student accounts
receive the defaults and a fresh random wallpaper after their ordinary home
reset; the login helper preserves that wallpaper. All assets and
extensions come from the locked Nix packages and work without login downloads.

Where Programming is selected, Pi and OpenCode have a reproducible system
version available to every user. npm
global installs use `~/.local/npm` and take precedence in the user's shell, so
admin or teacher can try a newer upstream CLI without `sudo`:

```sh
npm install -g @mariozechner/pi-coding-agent@latest opencode-ai@latest
hash -r
```

That override belongs only to the current user and requires Internet access.
Removing it reveals the Nix-managed version again. Prefer a reviewed Nix
package-base or deployment override when every computer must receive the same
version. VS Code is not updated through npm and remains part of Programming.

The student home has a stricter lifecycle. It receives an empty writable npm
prefix from the clean template at every boot. Student npm globals, Pi/OpenCode
configuration, conversations and stored credentials are removed before the
rotating snapshot and are not restored; a student must authenticate again in a
later session. Never seed API keys or OAuth files into the shared template.
Admin and teacher homes are persistent, so their per-user npm overrides and
credentials remain until they remove them.

The same typed workflow is available from the CLI:

```sh
nix run .#nixorium -- software catalog
nix run .#nixorium -- software search --query libreoffice
nix run .#nixorium -- software presets
nix run .#nixorium -- software preset plan --preset essential --scope shared \
  --exclude vlc
nix run .#nixorium -- software preset apply --preset essential --scope shared \
  --exclude vlc --expect REVIEW_TOKEN
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

Both single-package and profile apply atomically replace only
`lab-software.json` after repeating pinned Nix validation and checking the
review token and source fingerprints. Profile review resolves every selected
package before proposing one candidate; one rejected package blocks the whole
batch. Applying an already-added profile is idempotent, and excluding all its
packages is a valid no-change proposal. Neither operation writes the profile
catalog, modules, or lock. The ordinary TUI also records that one managed file
locally without exposing Git. For
`shared` and `controller` scopes, the same reviewed action then builds,
activates, and verifies this controller. It never pushes, starts PXE, or
distributes clients. After a successful save that affects clients, choose
**Distribute affected computers** to open the ordinary deployment selector with
exactly the old and new destinations preselected, or choose **Later**. This
shortcut still creates a fresh deployment plan and review; it applies the whole
current deployment configuration, not only the package just changed. Removed
inventory identities are reported and never broaden the selection to the whole
lab.

Choose **Check systems** from Software to reconstruct a current snapshot after
reopening the dashboard. It shows the desired Git revision and observation
time, verifies the controller only when its durable activation receipt, active
closure, and revision agree, and classifies clients only from authenticated
live observations. A recorded past deployment remains history and does not turn
an unavailable client into a verified one. The opening dashboard performs no
client probes. Packages supplied by private NixOS modules remain untouched and
are edited through the advanced module workflow.

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

### Update computers

```sh
nix run .#nixorium -- deploy plan --on pc01
nix run .#nixorium -- deploy plan --on pc01,pc02
nix run .#nixorium -- deploy plan --on @lab
nix run .#nixorium -- deploy apply --on @lab --expect REVISION_FROM_PLAN
```

Planning is read-only. It requires a ready deployment and clean Git revision,
expands only configured clients, and prints the revision-bound apply command.
In the dashboard the selection lists each computer's state at the last check
(`r` checks again, `n` selects those needing the update), and Inventory's `u`
opens the same review for every computer that needed it.
Apply repeats the preflight, requires the one-word `DEPLOY` confirmation, builds before
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

### Power controls

Open **Power controls**, choose shutdown or restart with Tab, select the
intended clients, and continue to run the preflight. The controller is never selectable. Computers that are off,
unreachable, or lack authenticated management access remain visible as not
sent; Nixorium does not queue a request for later.

An interactive user session remains eligible, with a prominent warning that
unsaved work may be lost. A computer where the student is logged in but has
not touched keyboard or mouse for ten minutes (or since the automatic login)
is shown as "Logged in, not in use" without that warning. Unknown session state is ineligible by default. The
TUI can explicitly acknowledge unknown-session risk with `u`, which creates a
new reviewed plan. The same operation is available from the CLI:

```sh
nix run .#nixorium -- shutdown plan --on pc01
nix run .#nixorium -- shutdown plan --on pc01,pc02
nix run .#nixorium -- shutdown plan --on @lab
nix run .#nixorium -- shutdown apply --on @lab \
  --expect REVIEW_TOKEN
nix run .#nixorium -- restart plan --on @lab
nix run .#nixorium -- restart apply --on @lab \
  --expect REVIEW_TOKEN
```

Planning checks installation/network recovery and concurrent client work as
well as access and sessions. When an active session is present, the review says
explicitly that `SHUTDOWN` or `RESTART` authorizes interrupting it. Apply requires the matching single word (or explicit
automation-only `--yes`), takes the same client-operation lock as deployment,
and repeats inventory, conflict, and session checks immediately before sending
the fixed operating-system request. Use
`--acknowledge-unknown-sessions` on both plan and apply only after reviewing
that risk.

Results are `accepted`, `not-sent`, or `unconfirmed`. Accepted means the remote
operating system accepted the request; loss of network contact does not prove
physical power state or a completed reboot. An unconfirmed result may have taken effect, so inspect
the target instead of retrying blindly.

### Operation logs

To prepare a minimized support report, run `nixorium support preview --json`
or open **Maintenance → Diagnostics → Support report** in the administrator
TUI. The CLI equivalent is `nixorium support export`. Both export paths
save only the displayed JSON under the administrator's private local state
directory; it never uploads, builds, deploys or repairs anything. Versions,
revision, time and aggregate counts remain visible: review before sharing.
Detailed logs below are not automatically safe to share. See the upstream
[support-report contract](https://github.com/giovantenne/nixorium/blob/master/docs/support-report.md).

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

In every administrator screen, `r` refreshes what is shown (computer states,
services, logs, Git review, network boot). After a result, `n` starts a new
review and `s` completes an interrupted save; all keys that act are listed at
the bottom of the screen. Maintenance shows everyday tasks first and groups
**Apply to controller**, **Controller services**, **Review Git changes** and
**Reset deployment template** under **Advanced**.

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

### Apply to the controller

```sh
nix run .#nixorium -- controller plan
nix run .#nixorium -- controller apply --expect REVISION_FROM_PLAN
```

Apply requires the one-word `REBUILD` confirmation, starts only the matching revision-bound
systemd unit, builds as the deployment owner, refuses repository drift, and
records success only after activation and active-system verification. Closing
the dashboard does not stop the systemd-owned job. The dashboard shows elapsed
time, four typed phases, recent activity, and a progress bar; CLI text/JSON
flows write the same safe activity to stderr. When the job ends, the dashboard
refreshes reconciled state and shows a compact result with explicit actions to
return home, reveal the activity detail, inspect logs, or create a new review.
The TUI task is **Maintenance → Advanced → Apply to controller**; it uses
Enter after showing what changes and the restart impact.
Use `setup apply` for the equivalent first-run action with identical progress
feedback.

### Review changed client SSH trust

After a deliberate client reinstall, a changed SSH key needs a separate
physical-fingerprint review, not a relaxed SSH policy. Use the administrator
computer details or `host-key plan`/`host-key apply`; see
[reviewed client trust recovery](TROUBLESHOOTING.md#a-client-was-deliberately-reinstalled-and-its-ssh-key-changed).

### Manage services

```sh
nix run .#nixorium -- services
nix run .#nixorium -- services restart cache
```

The report combines the persistent signed cache and on-demand PXE lifecycle.
Cache restart requires the one-word `RESTART` confirmation and succeeds only after both systemd and
HTTP checks pass. PXE uses its dedicated transactional workflow and cannot be
mutated through this generic service action.

The controller runs Harmonia as `nixorium-harmonia.service`; systemd loads its
private signing key as an isolated credential outside Git and the Nix store.
Detailed Harmonia output uses `journalctl -u harmonia.service`.

### Back up the controller

The installed computers trust this controller's private keys, which are never
committed. Without a copy, a failed controller disk means reinstalling every
computer. Open **Maintenance → Back up the controller**, choose a USB drive or
network share and a passphrase of at least 12 characters, or run:

```sh
nix run .#nixorium -- backup create --to /run/media/admin/USB-DRIVE
```

The file is encrypted and contains the configuration with its history, the
private keys and the trusted computer keys. Keep it, and separately its
passphrase, away from the controller. The Overview reminds you when a backup
is due. To replace a failed controller, follow
[Backups and restoration](TROUBLESHOOTING.md#backups-and-restoration).

### Free disk space

Every application to the controller and every client update adds a system
version; nothing removes old ones automatically. Maintenance warns when the
controller's disk is low. When a disk fills up, open
**Maintenance → Advanced → Free disk space**, select the computers and review:

```sh
nix run .#nixorium -- cleanup plan --on controller,@lab
nix run .#nixorium -- cleanup apply --on controller,@lab --expect REVIEW_TOKEN
```

Each computer keeps its newest 10 system versions, the one it is running and
the one it started with; the boot menu also lists at most 10 versions. The
review names what goes and what stays and needs the one-word `CLEAN`
confirmation. Afterwards you can go back only to the kept versions. Computers
that are off are never queued, a computer that changed after the review is
skipped, and the space freed is known only afterwards. Homes, settings and the
network installation files are never touched. It does not run during a
deployment, PXE preparation or active network installation.

### Configuration and first-run setup

```sh
nix run .#nixorium -- setup
nix run .#nixorium -- setup status
nix run .#nixorium -- setup keys
nix run .#nixorium -- setup install-secrets
nix run .#nixorium -- setup apply
```

`setup configure` records the interface carrying the controller's default
route as a controller-specific override and proposes its live DHCP address
(not the controller's declarative static address), groups its essential
questions by task, and provides searchable offline selectors for time zone
and keyboard values while retaining validated custom entry. The desktop locale
is set by `defaultLocale` and `extraLocale` in `lab-settings.json`; change those
values through the reviewed `config plan` / `config apply` workflow. Optional
Git identity is not requested during first run. The wizard supports backward
navigation, collects passwords without echo, retries recoverable password
mistakes in the current account without restarting configuration, validates
the complete candidate, shows a redacted review, writes atomically after
acceptance, and reconciles all three key pairs. It never overwrites existing
key material. Bare `setup` and `setup status` report the first incomplete stage
without trusting a hidden completion flag; run `nixorium` and choose
**Installation → Network boot (PXE)** for the continuous interactive flow. The
ordinary TUI flow creates missing keys
automatically; import of existing private keys is available only from
**Maintenance → Change settings → Controller keys**.

`lab.ifaceName` remains the backward-compatible fallback. Optional
`controllerIfaceName` and `clientIfaceName` select role defaults, while
`hostIfaceNames` can override a configured host. Precedence is host, role, then
fallback. This permits different predictable interface names on controller and
client hardware without changing existing deployments.
The controller DHCP address must be outside the static lab prefix. Both
validators block overlap; choose a distinct static subnet before saving or
updating an overlapping deployment. Field help explains each value and the
editor previews the first/last client and controller addresses from the draft.
The installation form asks for the client computers' interface and shows the
controller interface separately. Before any client exists it proposes the
controller's interface, since client PCs are often the same model; an
installed laboratory keeps its saved value. On each PXE client, the installer checks the
selected host's effective interface before the erase review. If it is absent,
correct the client or host override on the controller, save, and prepare fresh
installation files; the installer never changes the configuration itself. It
names the interface the PC started from the network through, which is the
name to use.

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
requiring the one-word `START` confirmation. Stop restores normal controller addressing. Recover
reconciles an interrupted session, and boot recovery performs the same repair
automatically. These actions are idempotent; a failed start rolls back before
returning. Quitting the dashboard does not stop active systemd-owned services.

The firewall exposes SSH, mDNS, Veyon, optional VNC, Harmonia, and PXE only on
the configured interface and roles. Institutional DHCP remains authoritative.

#### Enroll a client

On the PXE-booted client the guided installer starts by itself on the first
console. To start it again:

```sh
sudo /installer/setup.sh
```

The installer displays hardware and writable disks, offers only configured
host identities, and refuses an identity that answers its best-effort network
probe. Silence is not treated as a reservation. It verifies the selected
closure offline and requires its size plus 2 GiB of headroom before offering a
disk.

Disko starts only after the one-word `ERASE` confirmation on a review that
shows the exact client identity and disk; the controller installer uses the
same word. A mistyped disk choice or confirmation is asked again, and an empty
confirmation cancels without touching the disk. The installer reports partition, installation, and verification stages,
then offers a separately confirmed reboot. There is no unattended mode and no
client-side fallback fetch.

### Install one computer from USB over SSH

Use this path when UEFI network boot is unavailable. On the selected computer,
boot the official **NixOS 26.05 Minimal ISO** for `x86_64-linux` in UEFI mode
with Ethernet or Wi-Fi connectivity to the controller. Connect Wi-Fi from the
live console with `nmtui` first when needed. At its physical console run:

```sh
passwd
systemctl is-active sshd
ip -4 -br address
ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub
```

Keep that console visible. From **Installation**, choose
**USB over SSH**, select one configured identity, and enter only the live IPv4
address. Nixorium reads the Ed25519 host key without sending credentials and
shows its `SHA256:` fingerprint. Compare the complete value with the physical
console and type `MATCH`; only then enter the temporary password. Nixorium pins
that key before attempting password authentication, installs an
operation-specific ephemeral key, probes hardware, and shows only eligible
non-boot disks. Enter the exact disk path and the content-bound confirmation
shown by the review. Do not remove the USB, reboot, or reuse the address while
the install is running. While the installer job runs, the screen refreshes its
state by itself (a read-only status request) and names the current step and
elapsed time; the result then says in one sentence what happened and what to
do next, with the raw fields behind `d`.

The declared client interface must carry the reviewed live address. When the
PC is connected through a differently named card, the TUI shows both names
before disk selection and offers to save the observed card for that computer
only (a `hostIfaceNames` override, through the ordinary reviewed settings save).
It then cancels the prepared session, prepares the computer again and returns
to the live console step; set a new temporary password with `passwd`, because
the previous one was locked. No disk is touched. Wi-Fi
profiles and credentials are not copied from the ISO into the installed system:
arrange persistent connectivity separately, or reconnect locally as the
administrator after boot before verifying the configured static address.
Keep wireless secrets out of Git and the Nix store. Wi-Fi may not support PXE
on the selected hardware, firmware, or network; USB/SSH does not require PXE.

The controller prepares only target-independent installer content before it
contacts the live ISO; the target closure is then built and served by the
signed Harmonia cache. This is different from `deploy`: installation may erase
and partition a disk, while deployment updates an already installed system.
The live-ISO password is accepted only from a controlling terminal and is held
under `/run`; it is not stored in the operation record or installed system.

The equivalent CLI workflow is:

```sh
nixorium install usb prepare --host pc01
nixorium install usb start --host pc01
nixorium install usb status --id 0123456789abcdef0123456789abcdef --json
nixorium install usb reconcile --id 0123456789abcdef0123456789abcdef
nixorium install usb reboot --id 0123456789abcdef0123456789abcdef
nixorium install usb verify --id 0123456789abcdef0123456789abcdef
nixorium install usb cancel --id 0123456789abcdef0123456789abcdef
nixorium install usb close --id 0123456789abcdef0123456789abcdef
```

`start`, `reboot`, and `close` require an interactive controlling terminal and
do not accept `--json`; there is no `--yes` path. `prepare` is optional and
safe to run early. `cancel` is available before dispatch and after a confirmed
remote failure that reports no disk mutation; the TUI exposes it as **Cancel
safely** only when automatic cleanup could not be confirmed. Ordinarily,
status observation revokes the live key and releases the reservation
automatically for a definitive pre-mutation failure. After any uncertain or
disk-mutating dispatch, use `status` and `reconcile`: the controller never
repeats Disko or an uncertain reboot automatically. If the worker or controller
restarted, invoke `start` again for the same host and physically re-enter the same address,
fingerprint, and password. A matching live boot can be reattached for status
reconciliation only; a different boot is blocked and the old key is revoked.

On a reinstall, an expected SSH host-key difference for the configured static
address is shown separately. Authorize `ROTATE HOST KEY` only after checking
that the selected identity, live fingerprint, and disk are the intended
machine. The stored key changes only after the installed host boots and passes
post-install verification. Remove the USB when the operation says it is ready,
then use the separately confirmed reboot and verification actions.

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
nix run .#nixorium -- restart plan --on @lab
nix run .#nixorium -- restart apply --on @lab --expect REVIEW_TOKEN
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
nix run .#nixorium -- install usb prepare --host pc01
nix run .#nixorium -- install usb start --host pc01
nix run .#nixorium -- install usb status --id 0123456789abcdef0123456789abcdef --json
nix run .#nixorium -- install usb reconcile --id 0123456789abcdef0123456789abcdef
nix run .#nixorium -- install usb reboot --id 0123456789abcdef0123456789abcdef
nix run .#nixorium -- install usb verify --id 0123456789abcdef0123456789abcdef
nix run .#nixorium -- install usb cancel --id 0123456789abcdef0123456789abcdef
nix run .#nixorium -- install usb close --id 0123456789abcdef0123456789abcdef
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
The TUI's **Update Nixorium** advanced tool first fetches this bounded release
list. `master` is clearly marked as the Development branch, stable releases are
shown by default, and prereleases require explicit disclosure. The TUI has no
editable target and does not offer downgrades. During validation it shows the
candidate-lock, evaluation, representative-build, review, and final verification
phases, including the required output count and elapsed time. Required outputs
share one Nix build and the exact candidate lock; all must succeed. PXE
preparation evaluates all client systems and shared artifacts together, then builds
their resolved derivations without repeating the deployment evaluation. Controller
preflight also shares one fresh evaluation while preserving the separate checks
before activation and after completion. After validation, review
the scrollable two-file patch (`F4` expands candidate checks), then press Enter.
The longer exact phrase remains part of the explicit CLI apply workflow.

> [!IMPORTANT]
> The TUI saves these files transparently, then builds, activates, and verifies
> the controller. Clients remain unchanged until an explicit distribution.

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

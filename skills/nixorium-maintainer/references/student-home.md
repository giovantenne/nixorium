# Student home and desktop customization

## Understand what is reset

Inspect the deployment's imported modules and assets before choosing files.
The current template uses `modules/home-profile.nix` for student template
content, `modules/workstation.nix` for desktop/application policy, and assets
for editor settings/backgrounds. Existing labs may have different local layouts.

The saved workspace profile supplies the student preferences restored at boot.
Changing a student's live home is not a persistent customization. Saving a
profile does not change an already logged-in student's home.

Do not edit `/var/lib/home-template` directly as a durable solution, reset a
live home, or reboot without authorization. Explain when students will see the
change. Local rotating snapshots exclude some ephemeral content and are not
backups. Do not capture credentials, histories, browser profiles, SSH material,
or private workspace data into a shared template.

There is no supported “capture this student's home” command in this contract.
If asked for snapshots as a new template feature, distinguish that upstream
design request from the currently available declarative customization.

## Student preferences

The template includes an active `workspace-profile.json` with Essential
defaults: Ghostty, Chromium, Files, Text Editor, dark appearance and a compact
bottom dock. Leave it unchanged to keep those defaults. Customize it through
Maintenance → Settings → Student workspace; no separate activation switch or
migration workflow is needed.

The deployment-owned catalog supplies a baseline and available choices, not
additional software. Keep prerequisite packages present on every destination,
including the controller in controller-only mode. The example JSON is a reset
proposal, never an implicit replacement for a missing or invalid saved profile.

`nixoriumWorkspace` and its candidate hooks describe configured preferences,
destinations and pinned prerequisites. Their `prepared` state and seed path
are not proof of deployment or a successful home reset. Candidate resolution
must still compose the deployment's validation hook and bind the source, pin,
base file and exact proposal before saving. Workspace data enters the Nix store:
never include credentials or private session data.

A supplied profile always configures the managed reset. Saving the declaration,
building/applying systems, and the next normal boot are separate steps. The
student on the controller and clients receives the profile at boot; controller
autologin and staff preferences do not change. Rebuilds do not reset a current
home, and login does not reapply supported preferences. Students can change
them during the session until the next boot reset.

The seed includes supported preferences, neutral shell/Git defaults and standard
XDG folders with stable English names. It never captures a live home or imports
arbitrary application assets. Omitted values inherit the deployment baseline
or system/application defaults. Desktop themes, shortcuts and other policy
outside the profile schema remain in deployment modules.

If reset fails, inspect `home-reset.service` read-only and preserve private
`/var/lib/home-snapshots/.workspace-reset` recovery evidence. Its pending marker
blocks retries and normal login across reboot. Do not remove it, disable the
profile, or invoke the helper as a retry. Request separately authorized recovery
based on the actual snapshot/home state. Sanitized, read-only managed snapshots
under `/var/lib/home-snapshots/workspace` are not external backups.

## Review and save a profile

In the administrative TUI, open **Maintenance → Settings → Student workspace**.
It loads the saved declaration, never the example. The supplied template already
has a profile; preserve its defaults and any existing customization.
Choose Desktop, Dock, Editor or Browser, then a supported field:

- Use “Inherit” (or an empty numeric field) to omit an override. “Clear” on a
  list means an explicit empty list, not inheritance.
- Favorites and extensions come from the pinned deployment catalog. Space
  toggles entries; Shift arrows reorder selected favorites. Adding catalog
  entries or required software is a separate deployment change.
- Enter keeps a field in the draft; Esc cancels that field. Leaving the editor
  discards unsaved changes. Use the visible Review action (`v`) to evaluate the
  complete candidate without writing it, then inspect the scrollable review.
- On supporting pins, confirmed `SAVE` writes and records only the reviewed
  profile locally. Pre-existing profile edits are refused; unrelated staged
  files remain untouched. The result offers a separate controller review,
then fresh client selection after verified application. No save applies a
  system, deploys clients or resets a home.
- If writing or recording is unconfirmed, inspect the file and Git state
  through the result's Git review action. Do not replay the old save token or
  apply systems before recovery. Older pins may leave recording separate.

Review may be cancelled while metadata is loading; a save already in progress
must finish before quitting. If the original profile changed during editing,
leave and reload it before preparing another proposal. Do not automatically
retry a stale review or an uncertain-durability result. The editor supports only
the versioned fields: other application settings, whole-home imports and
system application remain outside it. The teacher dashboard has no editor.

Where supported, prepare the candidate in a separate regular JSON file rather
than overwriting `workspace-profile.json` before review. Preserve existing
preferences; the inactive example is only a starting point for a first profile.
The candidate supports the strict workspace schema, not arbitrary home files,
program settings or secrets. Use the deployment catalog and prerequisites on
every destination, including the controller student.

```sh
nixorium workspace plan --repo . --file /tmp/student-profile.json --json
nixorium workspace apply --repo . --file /tmp/student-profile.json --expect <review-token> --yes --json
```

Review the current/proposed declarations, effective baseline, student account,
destinations, resolved package/extension versions and dependencies. Use the exact
`reviewToken` from that plan only after authorization to save. Without `--yes`,
apply requires an interactive terminal and confirmation for a changed profile.
It re-evaluates the candidate and rejects stale source, pin, catalog or file
identity; do not automatically renew a failed token and retry the write.

The CLI operation writes only `workspace-profile.json`, mode `0600`. It does not
stage or commit it, edit the catalog/lock/modules, build systems,
deploy or reset any home. `saved` and `unchanged` are declaration states, not
evidence of live preferences. Review/commit and activation/distribution remain
separate authorized workflows; do not assume this save token authorizes them.
A first save leaves the profile untracked, so Nix cannot consume it until it is
explicitly tracked. If authorized, use the existing exact-path Git review/commit
workflow, selecting only the intended files and its separate commit token. A
workspace save token is not a Git commit token.
The profile is classified as managed and its schema is checked before a managed
commit. This check is not a substitute for the source-bound workspace review,
system validation or deployment. Missing/invalid profiles block this commit
path; removing a profile is not a way to bypass reset recovery.

```sh
nixorium git commit plan --repo . --paths workspace-profile.json --json
nixorium git commit apply --repo . --paths workspace-profile.json --expect <commit-review-token> --yes --json
```

The deployment must have a committed revision and tracked `flake.nix` and
`flake.lock`. Existing unrelated staged/unstaged edits are preserved and included
in the source review. Untracked files are not Nix inputs; explicitly review any
needed catalog/module additions before tracking them. Symlinked deployment paths,
non-regular profile files, unresolved Git entries and submodules are refused.
A `partial` durability result means the JSON was replaced but durable storage
could not be confirmed: inspect the profile and Git state before another plan.

## VS Code extensions and settings

For existing prepared profiles, framework and package-base update reviews show
current/proposed pinned package and extension versions, prerequisites and
effective preferences. Compare these before authorizing the input change;
they do not describe live home state or the latest vendor release. The full
comparison is token-bound; source changes still require a fresh review.
Changed boot behavior, student identity or destinations require separate
configuration review; an input update must not silently alter them.

Use Maintenance → Update system and packages (or `package-base plan`/`apply`)
for packaged extensions. Explain that the same pin can change the editor,
desktop, services and kernel; repeatedly saving a profile does not update an
extension. Preserve disabled editor/extension auto-update checks in the managed
seed. Build success is not plugin-loading evidence: test the reviewed candidate
on one client before fleet distribution, without changing active homes at save.

For “prepare VS Code for Python”, inspect whether VS Code is selected for the
intended hosts. Resolve extensions from the deployment's pinned package set;
verify attribute names, dependencies, and the installed extension directory
inside each package rather than assuming Marketplace IDs are Nix attributes.

Add only the agreed extensions to the local profile, preserving existing
ones and any settings unrelated to Python. Inspect current activation ordering
and ownership; keep the profile conditional on the effective VS Code package.
Legacy settings assets affect admin, teacher, and student in the supplied template:
do not broaden a student-only request to staff without identifying that effect.

Create staff application directories with their final owner instead of relying
on `install -D -o` for intermediate directories. Core repairs the managed
`.config/Code`, `.vscode/extensions`, and npm trees for admin, teacher, and
legacy student during activation; managed student ownership is assigned at reset,
not repaired during an active session. Do not use world-writable modes.
`/run/user/<uid>` is created by logind, while activation only reconciles an
already existing top-level directory whose ownership or mode is wrong.

Build a representative affected system and check the extension payload/settings.
Do not rely on a Marketplace download at student login: clients must receive
required artifacts from the prepared system without direct Internet access.
Runtime sign-in, optional cloud features, and project dependency downloads are
separate requirements and must not be described as offline-ready automatically.

## Background and dock

Use local assets and desktop modules; keep referenced assets inside the
deployment. Resolve installed desktop application IDs for dock entries and
keep favorites conditional on package scope. Inspect whether the existing
policy affects student only or staff too, and whether it supplies initial
defaults or reapplies settings at login. Avoid blindly copying an entire
dconf database or overwriting unrelated desktop settings.

The supplied workstation module installs Desktop Icons NG, Dash to Dock and
Tiling Assistant independently of the application profile. Keep their enablement
additive so unrelated extensions survive. The compact bottom dock with intelligent hiding,
MoreWaita icons, native Adwaita decoration and blue accent are deployment-owned
defaults. Persistent staff accounts use the static vector background. Reset
student homes choose randomly from `assets.backgrounds` at boot, and the login
migration must preserve that choice. Tiling Assistant provides snap assist
with small window gaps; no blur or animated wallpaper is required.

Existing accounts receive a targeted, one-time migration marked by
`~/.config/nixorium/desktop-style-v1`; later staff choices remain editable.
A separate `desktop-dock-v1` migration updates only the four dock visibility
keys for existing accounts: hide on overlap with any window, reveal at the
bottom edge, and remain visible on a clear desktop. Later staff dock choices
survive. Do not reset entire dconf databases. Students receive the defaults and
a newly selected deployment-owned wallpaper after their normal home reset.
Validate extension metadata against the locked GNOME major
and compile GSettings schemas strictly before applying. Favorites still depend
on effective package scope.

Every supplied software profile includes Ghostty and
`python3Packages.terminaltexteffects`, so the site screensaver is present even
with Essential. Preserve both declarations when editing the built-in profiles;
the screensaver module deliberately follows their effective host scope.

## npm and project content

Distinguish a globally available CLI, a starter project with dependencies,
and an npm download cache. Prefer a pinned Nix package for a suitable CLI;
reproducible project dependencies need their lockfile and a verified offline
packaging strategy. The default profile creates an npm prefix, not a
prepopulated package set.

The supplied site profiles provide Git, system-managed Pi and OpenCode, plus
Node/npm to every user. A user may install `@mariozechner/pi-coding-agent` or
`opencode-ai` globally without `sudo`; `NPM_CONFIG_PREFIX=$HOME/.local/npm`
makes that version user-owned and earlier on PATH than the Nix baseline. Such
an override persists for admin and teacher. For the reset student account,
`.local/npm`, `.npm`, Pi state, and OpenCode configuration/data are removed
before snapshots and an empty prefix is restored from the clean template.
Student updates and authentication therefore last only for the current boot.
Do not remove these exclusions or seed agent credentials into the template to
make updates persistent; use a reviewed Nix package/override for a common lab
version.

Never run `sudo npm install` or write into the Nix store. Do not fetch mutable
packages in activation or student login. Explain that a populated npm cache
alone is not proof a project installs offline, and that live student-installed
packages may disappear on reset. Test the requested offline use case before
claiming it works.

## Validate and hand off

Follow [operations](operations.md): evaluate the actual affected roles, build
one representative client when needed, and test installer equivalence if the
change affects assets/module inclusion in the offline bundle. Avoid building
every client. Report files changed, intended users/hosts, validation evidence,
and the remaining deploy/reset step separately.

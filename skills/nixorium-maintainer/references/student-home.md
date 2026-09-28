# Student home and desktop customization

## Understand what is reset

Inspect the deployment's imported modules and assets before choosing files.
The current template uses `modules/home-profile.nix` for student template
content, `modules/workstation.nix` for desktop/application policy, and assets
for editor settings/backgrounds. Existing labs may have different local layouts.

In legacy mode upstream recreates the clean home template during activation; the local home
profile runs afterward. Student home is reset from that template at boot.
Changing a student's live home is not a persistent customization. Changing the
template does not mean an already logged-in student's home changed immediately.

Do not edit `/var/lib/home-template` directly as a durable solution, reset a
live home, or reboot without authorization. Explain when students will see the
change. Local rotating snapshots exclude some ephemeral content and are not
backups. Do not capture credentials, histories, browser profiles, SSH material,
or private workspace data into a shared template.

There is no supported “capture this student's home” command in this contract.
If asked for snapshots as a new template feature, distinguish that upstream
design request from the currently available declarative customization.

## Optional workspace preparation and runtime

Some upstream pins expose `nixoriumWorkspace` and
`nixoriumValidateWorkspaceCandidate`. They validate optional workspace JSON,
catalog/baseline preferences and prerequisites across the controller and all
clients. Their `prepared` state is not an active-home receipt.
Preparation-only pins do not seed homes. Recent management commands expose
`workspace plan`/`apply`; check CLI help and the deployment's candidate hooks
before using them. Supporting commands also expose the administrative TUI editor
under Maintenance → Settings → Student workspace.
Where available, `nixoriumResolveWorkspaceCandidate` previews a raw candidate's
effective settings, versions and destinations without writing it. Still run the
deployment's validation hook, which may include additional local policy; a
preview is neither a save token nor evidence that the source remained unchanged.
Do not migrate a working deployment merely because these outputs exist. Check
the actual pinned capabilities and keep configuration, preparation and live
activation distinct. Workspace data and catalog contents can enter the public
Nix store; never include credentials or personal session data.

Pins exposing `lib.workspaceRuntimeVersion` additionally accept the separate
default-off `workspaceRuntimeEnabled` switch. Check that value in the deployment
and `nixoriumWorkspace.runtimeEnabled`; a prepared profile alone is not opt-in.
When enabled, edit supported preferences/extensions in the workspace JSON and
deployment catalog, not the ignored legacy template. Keep prerequisite software
available on all targets, including a controller with zero clients.

Before an explicitly requested migration, compare local home/desktop modules
with the pinned capability: older private copies do not update automatically.
Remove conflicting student writes only, preserving staff behavior. Retain the
desired supported values in the catalog baseline/profile; omitted settings use
system/application defaults, not an inferred copy of the old Nix policy.
The managed seed includes neutral shell/Git defaults and standard XDG folders
with stable English names. It does not import arbitrary legacy assets or settings.

Configuration, system build/deploy and the next normal boot are separate steps.
The managed reset serves the configured student on clients and controller,
without changing controller autologin. Rebuilds do not reset the current home,
and supported preferences are not reapplied at login. Students can edit them
until the next boot reset. A seed path or reset receipt is not fleet-wide proof.

If reset fails, inspect the `home-reset.service` journal read-only and preserve
`/var/lib/home-snapshots/.workspace-reset` as private recovery evidence.
`pending.json` blocks retries and normal login across reboot; a previous success
receipt does not clear it, and switching to legacy is not a recovery method.
Do not remove the marker or invoke the helper to try again. Escalate the exact
failure and request a separately authorized recovery based on the actual
snapshot/home state. Managed snapshots are read-only, sanitized copies under
`/var/lib/home-snapshots/workspace`; they remain distinct from external backups.

## Review a migration

New templates include a deployment-owned `workspace-catalog.nix` and an inactive
`workspace-profile.example.json`. Neither enables managed homes. The example
matches the Essential software selection: Ghostty, Chromium, Files, Text Editor,
dark appearance and a compact bottom dock. It does not select VS Code. Treat it
as a starting proposal, not an import of the laboratory's current preferences.
The catalog baseline is empty; its application and extension entries are choices,
not additional software declarations. Live Server is an initial pinned extension
choice, not a certificate of loading on every editor version.

For an authorized migration, present an explicit comparison of existing policy
and the proposed profile before changing the runtime switch:

| Existing source or behavior | Managed student outcome |
|---|---|
| Favorites and the supported appearance/dock fields in `modules/workstation.nix` | Declare desired values in the profile; the managed student login no longer overwrites them |
| GTK/icon themes, fonts, shortcuts, GNOME extensions and Chromium homepage policy | Remain deployment/system policy, outside the JSON schema; review their existing scope separately |
| `assets/mimeapps.list` | The profile can select the HTTP/HTTPS/HTML browser; other MIME associations are not imported |
| `assets/vscode-settings.json` | Only the seven supported editor settings can be declared; themes, terminal profiles, telemetry, chat and extension-specific preferences are not imported |
| Legacy editor/extension auto-update preferences | Managed VS Code settings disable editor and extension update checks; this does not transfer the rest of the legacy asset |
| Java extension pack, other extensions and locally edited extension manifests | No automatic transfer; resolve each desired component, dependencies and pin, and test loading before use |
| `.vscode/argv.json`, including the legacy password-store option | Not part of the managed seed; review the behavioral difference instead of silently copying it |
| Student shell/Git/XDG scaffold | Recreated from declared identity and neutral defaults; standard folders have stable English names |
| Student files, browser sessions, credentials, caches and personal application state | Never imported into the seed; pending work and external backups need separate handling |
| Teacher/admin home setup | Preserved by the supplied modules; older private copies require a local comparison |

Use the actual files and settings in that deployment for the comparison. Do not
claim automatic parity, execute arbitrary legacy policy to infer a profile, or
capture a live home. If an unsupported setting is essential, keep legacy mode
until its handling is explicitly resolved. Do not silently drop it.

Keep preparation and runtime adoption as separate reviewed changes:

1. Confirm the pin supports the workspace schema. Keep
   `workspaceRuntimeEnabled = false`. Adapt the inactive example to the actual
   software on every destination, including the controller; validate the raw
   candidate through `nixoriumValidateWorkspaceCandidate` before saving the
   optional `workspace-profile.json`. Keep catalog additions explicit and
   preserve an existing profile rather than overwriting it with the example.
2. Inspect `nixoriumWorkspace`: check the declared/effective values, targets,
   prerequisites and versions. Preparation does not modify homes, and copying
   the example alone is not migration or authorization to deploy.
3. Before enabling runtime, separately review the pin's runtime capability,
   private module changes and unresolved differences. The supplied modules
   guard student writes with `workspaceRuntimeEnabled`; old local copies do not
   gain these guards from an upstream update. Remove conflicting student
   template writes, login overrides and ownership repairs without broad changes
   to staff policy. If custom writers cannot be excluded, keep runtime disabled.
4. Build and test the reviewed seed/system in an authorized disposable target,
   including actual extension loading when selected. Deployment, next normal
   boot/reset and live verification each remain separately authorized steps.

There is no automated migration command. Returning to
legacy is also a reviewed system change, not recovery of erased files. Preserve
managed snapshots and pending evidence; do not remove them to make a downgrade
proceed. Nix generations, profile declarations and student-data backups serve
different purposes.

## Review and save a profile

In the administrative TUI, open **Maintenance → Settings → Student workspace**.
It loads the actual declaration, not the inactive example. With no saved file,
it starts an explicitly labelled new draft and leaves legacy mode unchanged.
Choose Desktop, Dock, Editor or Browser, then a supported field:

- Use “Inherit” (or an empty numeric field) to omit an override. “Clear” on a
  list means an explicit empty list, not inheritance.
- Favorites and extensions come from the pinned deployment catalog. Space
  toggles entries; Shift arrows reorder selected favorites. Adding catalog
  entries or required software is a separate deployment change.
- Enter keeps a field in the draft; Esc cancels that field. Leaving the editor
  discards unsaved changes. Use the visible Review action (`v`) to evaluate the
  complete candidate without writing it, then inspect the scrollable review.
- Only confirmed `SAVE` writes the JSON. A saved result can open Git review,
  where path selection and commit confirmation are separate. No callback
  enables runtime, builds, deploys or resets a home.

Review may be cancelled while metadata is loading; a save already in progress
must finish before quitting. If the original profile changed during editing,
leave and reload it before preparing another proposal. Do not automatically
retry a stale review or an uncertain-durability result. The editor supports only
the versioned fields: other application settings, whole-home imports and
runtime migration remain outside it. The teacher dashboard has no editor.

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

This operation writes only `workspace-profile.json`, mode `0600`. It does not
stage or commit it, edit the catalog/lock/modules, enable runtime, build systems,
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
path; opting out of managed homes is a separately reviewed migration.

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
Changed runtime opt-in, student identity or destinations require a separately
reviewed migration. Deployments without a prepared profile keep their legacy
update flow; there is no automatic profile import.

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

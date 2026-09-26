# Student home and desktop customization

## Understand what is reset

Inspect the deployment's imported modules and assets before choosing files.
The current template uses `modules/home-profile.nix` for student template
content, `modules/workstation.nix` for desktop/application policy, and assets
for editor settings/backgrounds. Existing labs may have different local layouts.

Upstream recreates the clean home template during activation; the local home
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

## VS Code extensions and settings

For “prepare VS Code for Python”, inspect whether VS Code is selected for the
intended hosts. Resolve extensions from the deployment's pinned package set;
verify attribute names, dependencies, and the installed extension directory
inside each package rather than assuming Marketplace IDs are Nix attributes.

Add only the agreed extensions to the local profile, preserving existing
ones and any settings unrelated to Python. Inspect current activation ordering
and ownership; keep the profile conditional on the effective VS Code package.
Settings assets affect admin, teacher, and student in the supplied template:
do not broaden a student-only request to staff without identifying that effect.

Create staff application directories with their final owner instead of relying
on `install -D -o` for intermediate directories. Core repairs the managed
`.config/Code`, `.vscode/extensions`, and npm trees for admin, teacher, and
student during activation; do not replace this with world-writable modes.
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
MoreWaita icons, native Adwaita decoration, blue accent and static vector
background are deployment-owned defaults. Tiling Assistant provides snap assist
with small window gaps; no blur or animated wallpaper is required.

Existing accounts receive a targeted, one-time migration marked by
`~/.config/nixorium/desktop-style-v1`; later staff choices remain editable.
A separate `desktop-dock-v1` migration updates only the four dock visibility
keys for existing accounts: hide on overlap with any window, reveal at the
bottom edge, and remain visible on a clear desktop. Later staff dock choices
survive. Do not reset entire dconf databases. Students receive the defaults after their
normal home reset. Validate extension metadata against the locked GNOME major
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

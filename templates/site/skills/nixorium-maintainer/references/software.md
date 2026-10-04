# Software requests

## Resolve ownership and destinations

Inspect `lab-software.json`, the local catalog, and imported modules before
editing. The catalog is suggestions, not an allowlist. Use the deployment's
pinned package search/validation; a package in a different channel is not proof
that it is available here. Respect licensing policy; do not enable unfree
packages globally merely to bypass a refusal.

Use managed declarations for package selection and local modules for package
overrides, services, editor settings, and other policy. Do not try to remove a
module-provided package by deleting an unrelated managed declaration.

On deployments advertising controller software support, `shared` includes
the controller and present/future clients, and `controller` targets only the
controller. `all-clients`, `group:NAME`, and `clients:pc01,pc04` never include
the controller. Inspect the evaluated inventory first; do not guess host
names.

```sh
nixorium software catalog
nixorium software search --query libreoffice
nixorium software presets
nixorium software preset plan --preset essential --scope shared --exclude vlc
nixorium software preset apply --preset essential --scope shared --exclude vlc --expect REVIEW_TOKEN
nixorium software plan --package vlc --scope shared
nixorium software apply --package vlc --scope shared --expect REVIEW_TOKEN
```

Use the exact proposal's token and arguments. Removal requires `--remove` in
both plan and apply. Scope changes must review both old and new destinations.
Keep related local favorites, shortcuts, and home policy conditional on the
effective `hostSoftwarePackages`.

Software profiles are optional deployment-owned starting selections. Applying
one adds its missing package declarations in a single atomic candidate. It does
not replace the current configuration, remove packages, alter existing scopes,
or remain active as policy. Review exclusions and every existing declaration
shown by the plan. One invalid or policy-blocked package rejects the whole
proposal; do not split the profile into unchecked individual applies. The
operation writes only `lab-software.json`, and repeated application is
idempotent. An absent profile catalog leaves individual software management
available; an invalid catalog must be fixed rather than bypassed.

In the ordinary TUI, open Software and press `p` to add a profile. Review the
description, toggle package exclusions with Space, select one scope, and inspect
the single aggregated review. Existing declarations are labelled with their
preserved scope. Esc from the catalog, package, scope or review stage makes no
change. Do not reproduce this sequence as individual package applies.

CLI software apply saves only the declaration. The ordinary TUI also records
it in Git and, when the controller is affected, builds and activates the
controller. Neither path implicitly deploys clients. Do not use the TUI as a
configuration-only workaround; review the current operation's stated effects.

After a successful TUI save affecting clients, the contextual distribution
action opens the existing selector with the exact old and new client identities
preselected. It still requires a fresh deployment plan and confirmation and
applies the complete current deployment configuration. A removed inventory
identity is reported rather than replaced with `@lab`; returning to Software
does not save the declaration again. If controller activation was required but
did not verify, complete its recovery before distributing clients.

Use the Software system-state action after reopening the TUI or when an
`unchanged` result needs verification. It reads the repository's current
revision, accepts controller state only from the matching activation receipt
and closure, and labels clients from current authenticated observations.
Deployment history is supporting context only. Unknown means current state was
not proven; it is not equivalent to pending or successfully applied.

## “Update OpenCode to the latest version”

First identify how this deployment provides OpenCode: managed Nix package,
local override, or a user-owned installation. Distinguish its package version
from the Nixorium framework version and from the executable currently running.

Compare the pinned package with the requested published release using its
official source when Internet access is authorized. “Latest” must be resolved
to a concrete version/source, not left as a moving download in activation.
If the target is unavailable offline, report that instead of guessing.

Updating Nixorium is not an application update strategy: new deployments own
their `nixpkgs` input independently. A `nixpkgs` lock update can change many
packages; explain that impact and obtain approval for a broad refresh.
Use `package-base plan/apply` or Maintenance → Update system and packages for
a separately approved broad refresh. Channel migration requires an explicit
target and `--allow-unverified`; support metadata is advice, not permission.
Build and offline checks still block; do not raise `system.stateVersion`.
Do not change its channel implicitly. For a one-application request,
consider a deployment-local pinned override with verified source hashes and
dependencies if feasible; do not promise that an arbitrary newer version builds.

Preserve unrelated lock nodes and declarations. Do not run an unrestricted
`nix flake update`, use an unpinned self-updater, or replace a Nix-managed
executable with a user install unless the admin explicitly chooses that model.
Build affected roles before claiming compatibility. Report requested version,
resolved package version, and whether it has actually been activated.

## “Install everything needed for Python”

Clarify the activity only if it changes the selection: basic Python editing,
notebooks, scientific packages, or web development need different tools.
Propose a small explained set; “everything” is not permission to install every
matching package. Use the [student-home guide](student-home.md) for editor
extensions and persistent defaults. System packages, project environments, and
VS Code extensions are distinct configuration layers.

## Local MySQL and web server for exercises

The supplied Programming profile selects `mysql84`, `mysql-workbench` and `php`.
With `mysql84` present, `modules/development.nix` runs a MySQL server that
listens on loopback only, leaves `root` without a password and discards all
databases at every boot. This matches common course material and the reset
student home; it is not a place to keep data. Explain that before a teacher
relies on it, and keep durable work as exported SQL files. Do not open the
port, add remote accounts or remove the boot-time discard without an explicit
decision: a persistent database shared by successive students is a different
service with different privacy and backup needs. The same module adds a PHP
build with Xdebug for step debugging from VS Code, and with `apacheHttpd` a
XAMPP-style Apache that serves the student's `~/public_html` at
`http://localhost/`. It runs as the student account, listens on loopback only
and is reset with the home. Do not make it listen on the network, run it as
another account or point it at staff homes without an explicit decision.

Follow [operations](operations.md) for validation and explicit activation.

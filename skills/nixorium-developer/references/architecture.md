# Upstream architecture

## Repository roles

The public repository is a versioned framework. It exports `lib.mkLab`, a site
template, standalone example hosts, netboot outputs, helper
apps, readiness metadata, and an offline installer bundle.

A private deployment owns site configuration and consumes a pinned upstream
release. Never solve ordinary customization by copying an upstream module into
the deployment.

## API and extension points

`mkLab` accepts `deploymentSelf`, typed `labConfig`, public-key and asset
paths, and shared/controller/client/group/host/netboot module lists. Filesystem
references must remain inside the upstream or deployment source trees.

Generated host names and addresses come from the validated IPv4 network,
prefix, client count, and host numbers. Reject unknown per-host modules so configuration typos cannot be ignored.

Clients of the same class (module groups, managed software, interface; the
controller, `hostModules` hosts and `updateValidationHosts` are classes of one)
share one system, evaluated and built once (ADR 0033). A class of one keeps the
per-host system and is named after its computer; a larger class is
`shared-<n>` and `nixosConfigurations.pcNN` aliases it. A shared system must
not evaluate a computer's name or address: `hostName` and `hostIp` throw, and
`modules/identity.nix` and `modules/networking.nix` take both from the
computer's identity record (`/var/lib/nixorium/identity.json`, number only)
at activation. The deployment engine and both installers write the record
before the shared system runs, and never overwrite a record that names another
computer. The flake has no `colmena` output.

## Deployment migrations

Files copied from the site template belong to the deployment, so later releases
cannot simply overwrite them. The changes are Go code in the release:
`Registry` in `internal/deploymigrate/registry.go` (ADR 0034), a list of
`Migration{ID, Description, Apply(Tree) Outcome}` with the helper
`ReplaceExact`, `AddSharedPackage` (a package for every computer in
`lab-software.json`) and `Tree.Read`/`Write`/`Remove`, so a migration may change,
create or remove files. The release being installed runs them. During Update
Nixorium the installed controller builds the target's `nixorium` package from
the candidate, checks its marker `share/nixorium/deployment-migrate-contract`
(absent: a release before migrations, nothing to run; it lists the contract
versions, now `1`), and runs `nixorium deployment migrate --root /work
--contract 1 --json` in a bubblewrap sandbox (no network, store read-only, only
the copy writable, 120 s) on a copy made from the committed deployment. It
runs only when the update changes the Nixorium revision.

The stable contract is the exit status (0 ran, 2 unsupported contract, 3 the
migrator refuses this deployment, anything else refuses the update) and the
JSON report. The file difference is the truth, the report only labels it:
`internal/adapters/deployment_migrations.go` accepts only regular files outside
`flake.nix`, `flake.lock`, `keys/`, `admin-ssh/`, credentials, `secret-key`,
hidden paths and links, within 1 MiB per file, 256 files and 16 MiB, requires
every changed file to be declared by an applied migration and every declared
file to have changed, and allows the record `nixorium-migrations.json` to be
created or changed but not removed. The record is written by the target's
migrator, append-only, keeping unknown ids. The review shows "Migration" and
"Manual change" lines and the diff; the review token binds the files, the raw
report and the migrator's store path. Saving writes the files first, each only
if unchanged since the review, restores them if a later step fails, and
commits them with the update.

Rules for the developer: append to `Registry` in the same change as the template
change; copy `from` from every released form of the template text; `Files` must
list every file an applied migration changed; never edit or remove a released
entry; test it in `internal/deploymigrate`; change the contract only by adding
a version and keeping the old one for a year of releases. The mechanism runs
only for a controller whose release already has it; for an older one the
upgrade note gives `nixorium deployment migrate --root .` through `nix run`.

`deploymentStatus` reports whether placeholders, missing public keys, or
public default passwords remain. Keep the standalone example evaluable even
when it is intentionally not deployment-ready.

## Offline invariant

The installer bundle contains a standalone Flake with the effective typed
configuration, local input sources, public cache key, downstream modules, and
assets. Preserve path types as well as values when serializing it.

For an unchanged lock and configuration, a client evaluated through the
deployment and through the offline bundle must have the same
`system.build.toplevel.drvPath`.

All destructive bootstrap tooling, including Disko, must come from the
deployment's locked inputs. Do not fetch a mutable tool revision at install
time. Keep the exported Disko app and package buildable in automated validation.

## Built-in module boundaries

Keep the small `common.nix` module as the composition point. Desktop policy,
packages, power behavior, screensaver, shell configuration, and SSH policy
belong in separate modules. Site-specific removal or policy should use
downstream overrides or the smallest new generic extension point.

Keep module evaluation free of import-from-derivation: generated files are
installed through `environment.etc.<name>.source`; never read a derivation
with `builtins.readFile` during evaluation.

Keep controller orchestration in `management.nix` and privileged PXE address
state in the focused `pxe.nix` module. The network unit must write its
root-owned session before mutation, remove and restore only the exact recorded
static CIDR, preserve unrelated interface addresses, and retain only
`CAP_NET_ADMIN`. Git inspection stays in the unprivileged application and
preparation layers rather than expanding the network unit's filesystem access.
The systemd-owned listener must reject mismatched preparation/session state,
bind only after the transactional network unit, use an explicit readiness
signal, and drop HTTP and dnsmasq to separate unprivileged identities.
Public lifecycle operations must keep readiness checks in the unprivileged
application, expose typed reconciled state, use exact systemd verb/unit pairs,
deny direct network-unit start, and synchronously roll back a failed start.
Wire TUI screens to those application operations through typed callbacks from
the command composition root. Presentation code may manage navigation, review,
confirmation input, and rendering, but must not execute commands, select
privileged units, or duplicate lifecycle decisions.
Keep client enrollment local and guided until a genuine authenticated identity
exists for netboot clients. Limit selection to the immutable versioned
installer inventory, label reachability probes as best-effort rather than a
reservation, require the exact hostname and canonical disk in destructive
confirmation, and do not enable unattended installation without explicit
private policy and a documented invocation-token lifecycle.
Keep firewall policy in the focused built-in module: no implicit global
OpenSSH/Avahi openings, common desktop services only on the configured
interface, and Harmonia/PXE ports only for the controller role.

## Workstation profile ownership

The public framework owns mechanisms and invariants. The generated private
deployment owns the workstation profile: user-facing package selections,
browser and editor policy, IDE extensions, development toolchains, AI agents,
desktop favorites and shortcuts, MIME defaults, and branding.

Built-in modules may install a package only when an upstream mechanism requires
that executable at runtime. Prefer service-local `path`, `runtimeInputs`, or an
explicit package reference over adding implementation dependencies to every
host's global package list. GNOME, classroom control, home reset, and similar
product capabilities may remain upstream while their site-specific content and
application choices remain downstream.

Use `lab-software.json` for package declarations that operators should manage
through the application. Use deployment modules for application configuration
or other structured NixOS policy. Suggested software is deployment policy, not
an upstream allowlist. Core packages required for Nixorium itself are not
ordinary software choices and should not appear as removable TUI entries.

When moving an application out of upstream, inventory all coupled behavior,
including desktop launchers, favorites, keybindings, MIME associations, home
template files, extensions, activation scripts, assets, and offline-installer
serialization. Moving only `environment.systemPackages` leaves a misleading
and often broken boundary.

Preserve existing deployments unless a deliberate breaking change is in scope
and its impact is explicitly established. Never infer that compatibility can
be discarded merely because the current checkout has no private deployment.

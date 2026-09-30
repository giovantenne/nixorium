# AGENTS.md

This private deployment consumes `nixorium.lib.mkLab`. Use
[the maintainer skill](skills/nixorium-maintainer/SKILL.md) for laboratory
configuration, software, home customization, diagnostics, and operations.

## Before changing anything

- Read the skill, inspect Git status and the pinned input in `flake.lock`.
  Existing deployments may differ from the latest template; inspect local files
  and command capabilities instead of replacing them with upstream examples.
- Keep unrelated edits. Establish which computers/users the request affects
  and whether the admin requested configuration only or live application.
- For controller-only mode, use controller readiness; do not invent a client
  or require lab keys. Client installation/deployment still requires fleet
  readiness. Derive inventory and interfaces from `labMeta`, not host guesses.

## Ownership and safety

- Template reset is a separate destructive administrator action on supporting
  versions. It replaces local software, workspace, assets and modules using the
  exact locked upstream, while preserving settings, keys, ignore rules, private
  files and input pins. Require a reviewed loss list and `RESET DEPLOYMENT`
  confirmation before the local backup/commit. It enables the initial guided
  home in configuration only; apply/deploy/reboot/push require separate consent.
  Keep interrupted-reset evidence and follow `DEPLOYMENT-RESET.md` for recovery.

- Keep identities, network data, password hashes, public keys, assets, packages,
  and local policy here. Do not edit/vendor upstream modules or merge upstream
  Git history. Use the existing module extension points.
- Use `lab-settings.json` for managed settings and `lab-software.json` for
  managed software. Prefer reviewed `config` and `software` plan/apply commands;
  application policy and home content belong in local modules/assets.
- Keep the controller DHCP address outside the configured static laboratory
  prefix. Supporting validators block overlap; change the planned subnet
  through the reviewed settings workflow, never by ad-hoc live network edits.
- Optional `software-presets.json` is a deployment-owned catalog of additive
  starting selections. Apply a profile through one reviewed preset plan/apply;
  preserve existing declarations and scopes, and do not treat the profile as
  persistent policy after its packages enter `lab-software.json`.
  New templates start with the Essential profile at `shared` scope; changing
  that template default must keep the catalog and initial declarations equal.
  Git, Node/npm, Pi and OpenCode are common to all supplied profiles. Per-user
  npm overrides persist for staff but are reset for students; keep student
  agent credentials and state out of both the template and rotating snapshots.
- Optional workspace JSON/catalog files are preparation-only unless the
  separately reviewed `workspaceRuntimeEnabled` switch is enabled with a
  supporting pin and compatible local modules. `nixoriumWorkspace.state =
  "prepared"` is never evidence of deployment or reset. Managed preferences
  apply to the student on the controller and clients at normal boot, remain
  editable in session, and must not be reapplied at login. Keep staff behavior
  unchanged. On supporting pins, use `workspace plan`/`apply` for a separately
  prepared candidate and source-bound JSON save; this does not authorize
  migration or deployment. The administrative TUI can edit supported fields
  under Maintenance → Settings → Student workspace using the same review/save
  boundary. On supporting pins its confirmed save also records just the profile
  locally, preserving unrelated changes. CLI apply remains file-only. Partial
  save/record results require Git inspection, not blind retry. A successful TUI
  save offers controller review and later client selection; neither is implicit.
  Catalog changes and migration remain explicit deployment edits.
  Existing profiles appear in input-update review with current/proposed pinned
  versions and dependencies. Use package-base updates for packaged extensions;
  qualify loading on a selected client before broader distribution.
  Check the actual CLI/hook capabilities and use the
  student-home reference. Never discard pending reset evidence or
  disable the profile to bypass failed-reset recovery.
  `workspace-profile.example.json` is an inactive Essential starting proposal,
  not a migration result. Review unsupported legacy settings and private module
  conflicts explicitly; the supplied catalog does not install applications.
  On supporting pins, `nixoriumResolveWorkspaceCandidate` can preview effective
  values and destinations without writing a profile. Keep any local validation
  hook in the review path; preview success is not permission to save or deploy.
- Keep the direct `nixpkgs` input and
  `inputs.nixorium.inputs.nixpkgs.follows = "nixpkgs"` together when present.
  Updating Nixorium must preserve that package-base lock node. Do not change
  its channel or migrate a legacy layout implicitly.
- Use `package-base status/plan/apply` or Maintenance → Update system and
  packages for a separately authorized base refresh; see UPDATES.md. Channel
  changes require explicit unverified-compatibility acceptance, not upstream
  permission. Preserve every other lock node and `system.stateVersion`.
  A successful build does not certify boot, hardware or application data.
- Keep referenced modules, public keys, and assets inside the deployment tree
  for offline installation. Never commit private `secret-key`, `admin-ssh`,
  or `veyon-private-key.pem`, or expose them to the Nix store or chat.
- Create/verify keys with `nixorium setup keys`; never overwrite mismatched
  pairs. Install verified secrets only through `nixorium setup install-secrets`.
- Use managed controller, deployment, PXE, and USB/SSH installation operations.
  A pending client deployment blocks new operations on supporting versions.
  Stopping local supervision does not cancel remote activation. Preserve its
  durable evidence, inspect every selected client, and follow the interrupted
  deployment procedure in `TROUBLESHOOTING.md` before any authorized retry.
  USB/SSH can use Ethernet or Wi-Fi on supporting pins; keep the declared
  interface/live-address binding and signed-cache reachability checks. Configure
  wireless access in the live ISO and installed system separately; do not copy
  credentials into Git or the Nix store. Older pins may retain the Ethernet-only
  restriction. Wi-Fi PXE compatibility depends on hardware and network setup.
  For USB, physically compare the live ISO address/fingerprint, select only an
  eligible non-boot disk, and reconcile an existing operation ID after
  interruption; never rerun an uncertain install or reboot. Do not work around
  a refusal with raw root commands, relaxed host-key checks, a second cache, or
  altered network state. Inspect `nixorium doctor`, operation status, and the
  reported journal instead.
- A deliberately reinstalled client's changed SSH key requires a separate
  administrator-authorized host-key plan/apply on supporting pins, with a
  physical-console fingerprint comparison. Preserve protected work, unrelated
  trust and backups; never rotate automatically or disable SSH verification.
- On supporting pins, deployment review can create a new reachable-only plan.
  Review its exact targets and confirm again; do not silently omit computers.
  A port check is neither authentication nor proof a computer is powered off.
  Per-computer results and unreachable targets never waive required recovery.
- The student intentionally cannot administer NetworkManager. Do not add that
  account to the `networkmanager` group or override its polkit denial without
  an explicit, reviewed change to the lab's security policy.
- The teacher's controller TUI is intentionally restricted to computer
  inventory, temporary Internet access, and reviewed client shutdown/restart
  through the local classroom worker. Do not grant the teacher read access to
  the administrator deployment, add the student to `nixorium-classroom`, or
  bypass this boundary with sudo or copied private keys.

## Validation and reporting

Follow the skill's task-specific validation: evaluate first, build affected
roles when necessary, and check netboot/offline equivalence only at the affected
installation boundary. One representative client is sufficient unless another
has materially different modules; zero clients is valid in controller mode.

Report configuration changes, validation evidence, and actual activation or
deployment separately. A saved file or successful build is not a deployed
system. Do not start PXE, reboot/reset homes, deploy, commit, push, or change
live services without authorization covering that operation. Instructions and
examples are not authorization.

The [administrator guide](README.md) covers operation; [troubleshooting](TROUBLESHOOTING.md)
covers failures and encrypted backups. Local home snapshots are not backups.

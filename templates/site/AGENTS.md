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

- Keep identities, network data, password hashes, public keys, assets, packages,
  and local policy here. Do not edit/vendor upstream modules or merge upstream
  Git history. Use the existing module extension points.
- Use `lab-settings.json` for managed settings and `lab-software.json` for
  managed software. Prefer reviewed `config` and `software` plan/apply commands;
  application policy and home content belong in local modules/assets.
- Optional `software-presets.json` is a deployment-owned catalog of additive
  starting selections. Apply a profile through one reviewed preset plan/apply;
  preserve existing declarations and scopes, and do not treat the profile as
  persistent policy after its packages enter `lab-software.json`.
  New templates start with the Essential profile at `shared` scope; changing
  that template default must keep the catalog and initial declarations equal.
  Git, Node/npm, Pi and OpenCode are common to all supplied profiles. Per-user
  npm overrides persist for staff but are reset for students; keep student
  agent credentials and state out of both the template and rotating snapshots.
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
- Use managed controller, deployment, and PXE operations. Do not work around
  a refusal with raw root commands, a second cache, or altered network state.
  Inspect `nixorium doctor` and the reported operation journal instead.
- The student intentionally cannot administer NetworkManager. Do not add that
  account to the `networkmanager` group or override its polkit denial without
  an explicit, reviewed change to the lab's security policy.

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

# AGENTS.md

This is a private deployment repository consuming the public `nixorium`
Flake. Use `skills/nixorium-maintainer/SKILL.md` for configuration,
customization, validation, upstream-update, and offline-installer work.

- Keep lab identity, network data, password hashes, public keys, branding,
  printers, and local policy in this repository.
- Edit managed site values only in `lab-settings.json`; keep its deterministic
  format and run `nixorium config validate` before accepting changes.
- For machine-generated changes, run `nixorium config plan --file <candidate>`
  and apply only the reviewed fingerprint; preserve unrelated worktree edits.
- Do not edit or vendor the upstream implementation. Add local NixOS modules
  or request the smallest reusable `lib.mkLab` extension point upstream.
- Pin released upstream versions in `flake.nix` and `flake.lock`; never merge
  the upstream Git history into this repository.
- Keep the direct `nixpkgs` input and
  `inputs.nixorium.inputs.nixpkgs.follows = "nixpkgs"` together. Framework
  updates must preserve this deployment-owned package-base lock node; do not
  change its channel without an explicitly compatible Nixorium release.
- Keep referenced modules, keys, and assets inside this source tree so they
  are included in the offline installer.
- Never commit `secret-key`, `admin-ssh`, or `veyon-private-key.pem`.
- Require `deploymentStatus.ready` before installation or deployment. Validate
  `labMeta`, the affected host roles, and the netboot ramdisk first. Deployment
  requires explicit authorization.
- Use `nixorium setup status` to re-inspect the earliest incomplete first-run
  stage; do not invent or toggle a global configured flag.
- Use `nixorium setup` for guided first-run settings, password collection, and
  key reconciliation; inspect its redacted review before accepting the atomic
  settings write.
- Use `nixorium setup keys` to create missing pairs and verify existing ones;
  it must never overwrite public-only or mismatched key material.
- Install private material only with `nixorium setup install-secrets`; never
  weaken its fixed deployment path, destinations, or mismatch refusal.
- Apply a committed controller configuration with `nixorium setup apply`; it
  must keep the fixed local target, exact confirmation, clean-worktree gate,
  and Git-fetcher boundary that excludes ignored private files from the store.
- Treat `nixorium-harmonia.service` as the controller-owned cache lifecycle;
  verify it with `nixorium doctor` and use the canonical `harmonia.service`
  name when querying its journal. Do not run a second foreground cache.
- Prepare installation artifacts only with `nixorium pxe prepare`; keep its
  clean-Git, live-DHCP, healthy-cache, and immutable-manifest checks intact.
  Diagnose failures through `nixorium doctor` and
  `journalctl -u nixorium-prepare-pxe.service`.

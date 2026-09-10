# AGENTS.md

This is a private deployment repository consuming the public `nixorium`
Flake. Use `skills/nixorium-maintainer/SKILL.md` for configuration,
customization, validation, upstream-update, and offline-installer work.

- Keep lab identity, network data, password hashes, public keys, branding,
  printers, and local policy in this repository.
- Edit managed site values only in `lab-settings.json`; keep its deterministic
  format and run `nixorium config validate` before accepting changes.
- Do not edit or vendor the upstream implementation. Add local NixOS modules
  or request the smallest reusable `lib.mkLab` extension point upstream.
- Pin released upstream versions in `flake.nix` and `flake.lock`; never merge
  the upstream Git history into this repository.
- Keep referenced modules, keys, and assets inside this source tree so they
  are included in the offline installer.
- Never commit `secret-key`, `admin-ssh`, or `veyon-private-key.pem`.
- Require `deploymentStatus.ready` before installation or deployment. Validate
  `labMeta`, the affected host roles, and the netboot ramdisk first. Deployment
  requires explicit authorization.
- Use `nixorium setup status` to re-inspect the earliest incomplete first-run
  stage; do not invent or toggle a global configured flag.
- Use `nixorium setup keys` to create missing pairs and verify existing ones;
  it must never overwrite public-only or mismatched key material.

# AGENTS.md

This is a private deployment repository consuming the public `nixos-lab`
Flake. Use `skills/nixos-lab-maintainer/SKILL.md` for configuration,
customization, validation, upstream-update, and offline-installer work.

- Keep lab identity, network data, password hashes, public keys, branding,
  printers, and local policy in this repository.
- Do not edit or vendor the upstream implementation. Add local NixOS modules
  or request the smallest reusable `lib.mkLab` extension point upstream.
- Pin released upstream versions in `flake.nix` and `flake.lock`; never merge
  the upstream Git history into this repository.
- Keep referenced modules, keys, and assets inside this source tree so they
  are included in the offline installer.
- Never commit `secret-key`, `admin-ssh`, or `veyon-private-key.pem`.
- Validate at least `labMeta`, the affected host roles, and the netboot ramdisk
  before deployment. Deployment requires explicit authorization.

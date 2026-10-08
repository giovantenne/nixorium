# Account credentials and Git backups

The implementation has four boundaries:

1. The password editor hashes input locally and saves `lab-credentials.json`
   with mode 0600. Git ignores this file. Tracked `lab-settings.json` contains
   names, privileges and `credentialsVersion`, never the three password hashes.
2. Back up lab encrypts credentials together with the original private keys in
   `nixorium-recovery.age`. Restore lab checks the version and recovers the local
   file. A password change requires a fresh verified backup before distribution.
3. Nixorium takes Git's tracked-only source, injects the three hashes into a
   temporary settings copy, and imports that copy into the local Nix store.
   All managed evaluation/build paths share this preparation. The original Git
   revision remains the system revision; the working configuration is untouched.
4. Controller and client builds retain ordinary `hashedPassword` behavior.
   The offline installer contains the effective hashes, as before. PXE requires
   no passphrase and no additional service. Private SSH/cache keys are excluded
   from every prepared source.

The public version is a counter, not a password-derived fingerprint. The TUI
increments it when collecting new passwords. An advanced `config plan` candidate
with changed hashes must increment it too; keep such candidates outside Git.
Missing credentials leave setup incomplete. If saving stops between replacing
the private file and replacing the public settings, the versions differ and
deployment is refused. Re-enter all passwords in Change settings → Passwords,
review and save, then back up again; alternatively restore the last complete
backup into a new directory. No manual coordination-file edits are required.

For administrator diagnostics, `nixorium config source --repo /absolute/lab`
returns the prepared flake reference. An optional `--revision` pins a committed
configuration; its credential version must match the local file. Do not attach
or give the prepared source to an assistant: it contains readable hashes.
A normal backup clone contains encrypted secrets only, but arbitrary custom
modules may still contain sensitive material. The passphrase stays on the
administrator's machine. This feature grants no remote assistant access.

There is no migration or history rewrite for existing inline-hash deployments.
Backups refuse recognizable password hashes and private paths throughout their
reachable history. New laboratories start from the current template.

Verification covers public/private serialization, permissions and symlinks,
stale reviews, interrupted-save repair, an SSH backup/restore round trip, the
absence of readable hashes in a fresh clone, and effective local/offline input.
The normal schema, evaluation and template-reset checks remain required.

# ADR-0025: Private Git backups and original-key restoration

- Status: accepted
- Date: 2026-10-08
- Supplements: [ADR 0024](0024-encrypted-controller-backups.md)

## Decision

The administrator's private deployment repository is the remote backup of saved
configuration. Back up lab reviews an explicit SSH URL, branch and clean revision,
then commits only `nixorium-recovery.age` and pushes that exact revision without
force, tags, hooks or automatic merges. Git credentials remain independent of
laboratory keys. Provider privacy is explicitly confirmed by the administrator;
Git transport alone cannot verify visibility.

The age/scrypt recovery file contains the two original private keys, trusted
computer keys and `lab-credentials.json` (the three account password hashes).
Configuration, public keys, assets and history remain readable; local credentials
are mode 0600 and ignored by Git. The settings editor preserves its masked input
and redacted review. A public `credentialsVersion` changes with passwords and
binds the local file to the saved configuration. Restore recovers that exact file.

Local Nix evaluation starts from Git's tracked-only source and inserts the hashes
into an isolated copy. Only that copy enters the Nix store. Builds, controller
activation, Colmena and offline installers use these same effective values;
PXE needs no additional passphrase. Hashes therefore remain visible in the local
Nix store and distributed systems, as before. SSH/cache private keys never enter
that source. See [credential storage](../credential-storage.md).

A normal clone has no readable managed account hashes. This does not sanitize
arbitrary custom modules or authorize AI access. No existing-lab migration or
history rewrite is included; inline hashes in backup history are refused.

A local receipt is written only after the remote reports the pushed revision.
The first client distribution and distribution after key or password changes require that
receipt. Later configuration/trust changes and age over 30 days are reminders;
local repairs remain possible offline. Existing offline archives remain usable
but do not satisfy the remote receipt requirement. Backup checks known private
paths and recognizable secrets throughout reachable history before sending.

Restore lab is available without an existing deployment. It fetches into private
staging with hooks, filters and submodule recursion disabled, decrypts and checks
original key pairs, then publishes into a new directory without replacement.
Restored trusted keys must not overwrite conflicting local trust. Fetched Nix is
never evaluated during restoration; installing secrets and activating the restored
controller remain separate reviewed operations.

## Consequences

A controller can be replaced without changing client trust. Both independent Git
access and the passphrase are needed. Lost passphrases cannot be recovered.
Changing a passphrase does not protect older ciphertext still in Git history.

Push failures leave a recoverable local commit and never count as a remote
backup. Divergence requires human Git reconciliation. SSH is the initial supported
transport; HTTPS credential onboarding, repository creation and visibility APIs
are outside this change. No cloud service or AI provider is involved.

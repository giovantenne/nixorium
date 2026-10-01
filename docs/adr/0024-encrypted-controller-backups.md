# ADR-0024: Encrypted controller backups

- Status: accepted
- Date: 2026-10-01

## Context

Installed computers trust the controller's administrator SSH key and cache
signing key. Those private keys are deliberately never committed. If the
controller disk fails and no copy of the keys exists, a replacement controller
cannot manage the laboratory, and every computer must be reinstalled. Until
now backups were only described in the troubleshooting guide; nothing created
them or reminded the administrator.

## Decision

- `nixorium backup create --to DIRECTORY` (TUI: Maintenance → Back up the
  controller) writes one file encrypted with a passphrase using age with an
  scrypt recipient. The passphrase has at least 12 characters.
- The file contains a manifest and a tar of the deployment repository,
  including `.git` and the ignored private keys, plus the verified host keys
  in `~/.ssh/nixorium-known-hosts`. Links into the Nix store are omitted.
- Every regular file is hashed before and while it is written. A file that
  changes during the backup stops it.
- The file appears under its final name only when complete, without
  overwriting. A destination inside the repository is refused.
- `backup verify` decrypts the file and checks every entry against the
  manifest. `backup restore` verifies first, then extracts into an empty
  directory only; replacing the deployment stays a documented manual step.
- The administrator's private state records the last backup. The Overview and
  the doctor report a backup as due when none exists, when it is older than
  30 days, or when the private keys or the laboratory settings changed since
  it.
- The passphrase is read from the terminal, or from a private file with
  `--passphrase-file` for unattended use. It is never stored by Nixorium.

## Consequences

A failed controller can be replaced without reinstalling the computers, as
long as a recent backup and its passphrase exist. Losing the passphrase makes
the backup unreadable by design. Restoring into the bootstrap itself and
scheduled backups remain future work.

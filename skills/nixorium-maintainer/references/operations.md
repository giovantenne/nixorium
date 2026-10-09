# Bounded diagnostics and operator handoff

Inherit the proposal-only boundary in [the skill](../SKILL.md).
Human administrator procedures do not expand this agent's permissions.

## Observations

After checking installed help, on a known trusted deployment, use only relevant
observations:

```sh
nixorium status
nixorium recovery status
nixorium package-base status
nixorium deploy queue status
nixorium services
```

These do not permit other verbs in their command families. Never use
unrestricted `nix run` as a fallback for a missing installed command.
Use minimal safe result fields; stop if output may disclose secrets.

For diagnostic evidence, ask the operator to preview locally and share only
the relevant redacted fields. On supporting pins the human uses:

```sh
nixorium support preview --json
```

The allowlisted snapshot is not anonymous. Do not automatically send a full
report to a model, export it or upload it. Raw Doctor errors, journals and logs
may contain sensitive text: use stable codes and operator-redacted excerpts.

Client observations need an explicit diagnostic request and product-mediated
observation. Never read SSH keys or use raw SSH. If allowed observations cannot
establish the result, report unknown. Historical success, a reachable port or a
reported revision alone do not prove completed recovery.

## Hidden effects and exclusions

Do not classify a command by its name. USB status/reconcile may revoke
credentials or release reservations; preparation may start services or create
artifacts; some plans build substantial outputs. They are not generic reads.

Do not run installation status/reconcile, controller/deploy/update/package-base
plans, PXE prepare, full Doctor, arbitrary Nix builds/evaluation or privileged
helpers under this skill. The operator inspects the native recovery UI;
the agent must not automate its confirmations, stdin or terminal consent.

## Recovery

Preserve IDs, locks, reservations, pending markers, identity and receipts.
Never clear evidence, weaken permissions/trust or replay work to unstick it.

- Client update: operator-led native recovery; the target revision alone does
  not clear durable pending evidence.
- USB install: the operator verifies physical identity, disk and boot identity;
  connection loss never authorizes reinstall or reboot.
- PXE/network: managed operator recovery, not interface/firewall workarounds.
- Home reset: preserve the login barrier and snapshot evidence; no helper retry,
  reboot or marker deletion as a shortcut.
- Workspace metadata failure: explain the pin/capability problem; do not
  silently reset the deployment, update inputs or copy an example profile.

Use the deployment's pinned administrator/troubleshooting/update/reset guides
to explain the human workflow, never as agent execution authority. If no safe
path is available, report the blocker rather than an executable bypass.

## Human application and reporting

The operator reviews and performs saves, Git changes, updates, activation,
deployment and power actions. A request to finish maintenance does not authorize
this proposal-only agent to perform them.

Software/input-update TUI flows can commit and activate the controller;
workspace/settings saves have separate activation review. Client distribution
is separately confirmed and can explicitly queue unavailable computers.
The agent may not queue, cancel or replay operations.

Build success proves neither boot nor plugin loading nor safe data migration.
Recommend a representative client test where relevant. Rollback does not
restore deleted data or undo disclosed credentials. Backup, cleanup, keys,
telemetry consent and classroom actions stay operator-only.

Return safe evidence, uncertainty and the native next step, including required
physical checks. Separate proposed, saved, built and verified-active states.
Never claim success because evidence is unavailable.

## Remote backups and restoration

The human administrator can use Maintenance → Back up lab and choose an encrypted
file (folder or USB drive) for a fully encrypted deployment archive, or a private
Git repository to review and push the saved deployment plus encrypted original
recovery keys over SSH. The administrator TUI then pushes later saved revisions
to that repository automatically while recovery material is unchanged. Private keys and `lab-credentials.json` remain ignored locally and
are encrypted in the recovery file. Configuration and Git history remain readable
to repository members; the passphrase is never stored. Backup is optional; a missing or stale backup
produces a reminder and never blocks client installation or updates. Either a
current file/USB archive or verified remote backup clears the reminder.

Maintenance → Restore lab (or `nixorium backup clone` without an existing lab)
restores a backup file or Git backup into a new directory, verifies original key pairs, and preserves trusted
computer keys. It does not activate the restored configuration. See the
administrator backup and replacement guide.
These are human operations, not authorization for a proposal-only agent to push,
read private keys/passphrases or perform restoration.

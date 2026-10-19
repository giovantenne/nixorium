# Student home and desktop proposals

Inherit the proposal-only boundary in [the skill](../SKILL.md).

## Student preferences

One profile supplies initial student preferences on controller and clients.
Staff homes are separate. Normal boot restores the managed seed; saving does
not reset the active home or immediately change a student's session.
Prepared metadata is not evidence of build, activation or successful reset.

Inspect the existing non-secret profile and pinned capabilities. Preserve
unrelated preferences; omission inherits and empty lists explicitly clear.
Never replace an invalid/missing profile with an example implicitly.

Only supported schema fields may enter a candidate. Additional initial files
use the separate capability-gated workflow below. New catalogs and Nix modules
remain outside this skill. Never edit live homes, staff settings assets or reset
scripts, or capture
dconf databases, browser sessions, histories, credentials or project data.

## Additional initial files

On pins exporting `lib.homeFilesVersion = 1`, propose `home-files.json` alongside
the existing workspace profile. It is a strict version-1 declaration with
`files` and `directories`. A file requires a relative `destination` and exactly
one of inline UTF-8 `text` or `source: "imports/<sha256>"`. Source references
identify already captured non-secret materials under `home-files/imports`;
they are not controller or student home paths. The agent may prepare small
requested inline text and explain existing source references, but must not read
or copy imported payload bytes. Do not create script launch hooks, credential
locations or generated workspace collisions. Explicit executability is a
separate reviewed choice, never inherited from a source.

The common seed supplies the student account on the controller and all clients.
Initial files are student-owned `0600`, directories `0700`, and explicit
executables `0700`. Students can edit or chmod their session copies; these modes
do not provide a read-only policy. Activation never replaces a live student
home. The next normal boot restores the declaration after system application.
Materials enter Git, the Nix store, installer and cache: only non-secret shared
content belongs here.

Hand local acquisition to the operator at **Maintenance → Change settings → Student
workspace → Initial files**. The product captures a private copy, reviews its
paths, sizes, digests and modes, and saves only after operator confirmation.
Updating a source requires another explicit capture; original controller files
are not watched. Capture IDs expire after 24 hours and refer to private local
state, not source bytes included in an agent proposal. The agent must not run
capture planning, apply, Git mutations or confirmations automatically.

For a separate agent-prepared candidate, inspect current help and use only
read-only review:

```sh
nixorium home-files plan --repo . --file /PRIVATE_TEMP_DIRECTORY/home-files.json --json
```

Hand the result to the operator. TUI confirmation also records exact paths
locally; human CLI apply remains file-only. Controller application and client
distribution retain separate reviews. Changed source or capture bytes require
a fresh review; partial storage results require operator Git inspection, never
token replay. Older pins need an operator-reviewed Nixorium update, rather than
copied modules or login scripts.

## Review and save a profile

The agent prepares a candidate; only the operator saves the deployment.
Create a new private temporary directory outside the checkout and a regular
candidate JSON file derived from the existing profile and requested changes.
Do not use a predictable shared filename or overwrite the saved profile.

After checking help, replace this example path with that private file:

```sh
nixorium workspace plan --repo . --file /PRIVATE_TEMP_DIRECTORY/student-profile.json --json
```

Planning must retain validation hooks and verify prerequisites, student identity
and destinations, including the controller. A failed hook, missing dependency
or stale source is not permission to alter catalogs, disable checks or update
the pin. Stop and report the operator step; do not run apply or Git mutations.

Hand over reviewed values and the candidate path. The TUI Student workspace
editor has a native complete review; on supporting pins its save also commits
exactly that profile. Controller application and client distribution are
separate reviews. A human-run CLI save is declaration-only, not a commit.
The agent must not automate either route or its confirmations.

Changed candidates or deployments require fresh review. Unconfirmed durability
or a partial save/commit requires inspection, not token replay. A first saved
profile needs explicit tracking before the Flake consumes it; the agent must
not stage it as an implicit next step.

## Extensions and settings

Resolve extensions using the pinned package set and existing catalog; never
invent attributes, identities, versions or hashes. Explain prerequisite tools.
For an unavailable extension, hand off to the product's native Marketplace
workflow if supported; do not download/install it or edit the catalog.

Extensions are executable third-party code. Packaging/build success proves
neither trustworthiness nor loading. Recommend an operator-led single-computer
test before distribution.

VS Code extra settings retain native refused-key checks. Do not encode command
execution, terminal profiles, automatic tasks, workspace-trust bypasses or
credentials in other fields to evade validation. Staff assets do not supply
student defaults automatically.

A plugin needing writable installation files requires an existing integration
or separate development work. Do not chmod homes, mark all extensions writable
or generate ad hoc hooks. Packaged updates follow the package-base pin, not a
profile re-save; explain broad impact and hand off.

## Desktop, project content and reset failures

Use supported desktop/dock/browser fields and existing application IDs.
New desktop modules and prepopulated npm content are outside the schema.
Requested non-secret initial materials use the additional-file schema above.
Explain other limitations rather than adding
login scripts or changing staff settings.

Student npm globals and agent credentials/state are ephemeral and excluded
from managed snapshots. Never change exclusions or seed secrets for persistence.
Local snapshots are not external backups.

The next boot recovers an interrupted reset by itself, once. After a reset
that still fails, keep pending evidence, login barriers and the profile
unchanged. Do not retry the helper or clear markers. Use
[operations](operations.md) for operator-led diagnosis/recovery. Distinguish
proposed preferences from runtime behavior that has actually been tested.

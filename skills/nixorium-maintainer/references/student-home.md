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

Only supported schema fields may enter a candidate. Unknown application
settings, arbitrary home files, new catalogs and Nix modules are outside this
skill. Never edit live homes, staff settings assets or reset scripts, or capture
dconf databases, browser sessions, histories, credentials or project data.

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
New assets, desktop modules, arbitrary project scaffolds and prepopulated npm
content are outside the schema. Explain the limitation rather than adding
login scripts or changing staff settings.

Student npm globals and agent credentials/state are ephemeral and excluded
from managed snapshots. Never change exclusions or seed secrets for persistence.
Local snapshots are not external backups.

After a failed reset, keep pending evidence, login barriers and the profile
unchanged. Do not retry the helper, reboot or clear markers. Use
[operations](operations.md) for operator-led diagnosis/recovery. Distinguish
proposed preferences from runtime behavior that has actually been tested.

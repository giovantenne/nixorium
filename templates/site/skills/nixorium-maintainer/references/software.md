# Software proposals

Inherit the proposal-only boundary in [the skill](../SKILL.md).

## Resolve packages and destinations

Inspect relevant non-secret declarations and the pinned catalog. Suggestions
are not an allowlist; resolve actual package IDs through the installed product.
Do not treat availability in another channel as availability in this deployment.

Permitted discovery/proposal examples after checking installed help:

```sh
nixorium software catalog
nixorium software search --query libreoffice
nixorium software presets
nixorium software plan --package vlc --scope shared
nixorium software preset plan --preset essential --scope shared --exclude vlc
```

Use only requested selections and actual inventory scopes. On supporting pins,
shared includes controller and present/future clients; controller is controller
only. Client scopes never include the controller. Review both old and new
destinations when changing scope. Never execute the apply command or edit
`lab-software.json`.

Profiles add declarations without replacing existing choices/scopes. Do not
split a failed profile into unchecked changes, remove core/module packages by
editing unrelated declarations or enable unfree packages globally to bypass a
policy refusal. Existing modules and services must remain untouched.

## Operator handoff

Explain selections, dependencies and affected computers. The operator reviews
Software in the TUI: saving may also commit and activate the controller.
It is not a configuration-only path. Client distribution is separately reviewed.
The agent performs neither save nor activation nor distribution.

Unknown current state is not pending/successful state. A partial save or failed
activation calls for diagnosis, not replay of the proposal as an apply.

## Updates and unavailable versions

For "update OpenCode", distinguish the declared package, installed executable
and framework version. If public Internet research is requested, use official
sources without private lab context and resolve latest to a concrete release.

If the target is unavailable in the pinned set, report the limit. Do not write
an override, edit inputs, use self-updaters or install npm/global replacements.
A broad package-base refresh can change kernel, desktop and other applications:
hand it to the operator's separate update review. Do not change
`system.stateVersion`. A framework update is not a selective app update.

## Teaching environments

For Python, propose a small set matching the activity, not every matching
package. System packages, editor extensions and project dependencies are
different layers; use [student home](student-home.md) for workspace proposals.
Do not claim offline project installation without evidence.

Inspect documented service effects before recommending packages. The supplied
Programming exercise MySQL/Apache setup is loopback-only and disposable; data
may be discarded at boot. Explain durable coursework/export needs. Opening
ports, changing service users or making shared databases persistent is system
policy work outside this skill, not an ordinary package selection.

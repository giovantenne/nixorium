---
name: nixorium-developer
description: Develop and release the public Nixorium upstream. Use for lib.mkLab, built-in NixOS modules, the management CLI/TUI, installer implementation, the site template, CI, compatibility, offline equivalence, and upstream releases. Do not use for routine configuration or operation of a private laboratory deployment.
license: MIT
---

# Nixorium Developer

Develop the reusable public framework without absorbing policy or identity from
a particular laboratory.

## Identify the upstream

Read `AGENTS.md` completely and inspect the worktree before changing anything.
Use this skill only for an upstream repository containing `lib/mk-lab.nix`,
`templates/site/`, and `VERSION`.

Private deployment maintenance belongs to `nixorium-maintainer`. If both
repositories are involved, inspect both worktrees and pinned revisions before
deciding where each change belongs.

## Preserve the public boundary

Put reusable behavior, validation, and safe defaults upstream. Keep identities,
network values, password hashes, keys, branding, printers, and host-specific
policy in deployments.

Keep the workstation profile deployment-owned. User-facing software presets,
application configuration, desktop preferences, editor extensions, development
toolchains, and AI assistants belong in the site template. Upstream owns the
mechanisms and only the packages required for those mechanisms to function.

Extend `mkLab` instead of requiring downstream copies of built-in modules.
Reject unknown configuration rather than ignoring it. Preserve standalone
outputs unless a deliberate breaking release removes them.

Read [references/architecture.md](references/architecture.md) before changing
the public API, modules, assets, netboot, or template. Read
[references/validation.md](references/validation.md) for every implementation
change. Read [references/git-workflow.md](references/git-workflow.md) before
creating commits, synchronizing branches, merging, rebasing, or pushing. Read
[references/release.md](references/release.md) before versioning, tagging, or
publishing. Read [references/tui-design.md](references/tui-design.md) before
changing management navigation, interaction behavior, visual styling, or
presentation components.

Use the default quick validation while iterating. Add `--eval` when the Nix API
or host composition changes, then select the affected VM only when behavior
crosses its integration boundary. Reserve `--full` for cross-cutting build
changes and milestone or release checkpoints; do not repeatedly run the full
matrix while editing. Validation must never run Nix store garbage collection
automatically.

Build one representative client and the controller, not every generated client
with the same module graph. Evaluation tests cover inventory, host-name, and
address generation. Build an additional client only when affected host-specific
modules make it materially different.

## Keep distribution coherent

Public API changes must update the template, both READMEs, `AGENTS.md`, and
`CHANGELOG.md` together. The template copy of `nixorium-maintainer` must be
identical to the upstream copy. The upstream-only `nixorium-developer` skill
must not be copied into private deployments.

For behavior changes, use the [guidance maintenance map](../../docs/agent-guidance.md)
to review affected agent instructions in the same change. CLI, TUI follow-up,
readiness, software scopes, and template behavior must agree with the skill;
passing syntax/link checks alone does not establish semantic coherence.

Commits, pushes, tags, releases, installations, live deployments, and external
repository changes require explicit authorization.

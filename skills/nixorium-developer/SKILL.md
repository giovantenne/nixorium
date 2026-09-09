---
name: nixorium-developer
description: Develop and release the public Nixorium upstream. Use for lib.mkLab, built-in NixOS modules, installer implementation, the site template, CI, compatibility, offline equivalence, and upstream releases. Do not use for routine configuration or operation of a private laboratory deployment.
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

Extend `mkLab` instead of requiring downstream copies of built-in modules.
Reject unknown configuration rather than ignoring it. Preserve standalone
outputs unless a deliberate breaking release removes them.

Read [references/architecture.md](references/architecture.md) before changing
the public API, modules, assets, netboot, or template. Read
[references/validation.md](references/validation.md) for every implementation
change. Read [references/git-workflow.md](references/git-workflow.md) before
creating commits, synchronizing branches, merging, rebasing, or pushing. Read
[references/release.md](references/release.md) before versioning, tagging, or
publishing.

## Keep distribution coherent

Public API changes must update the template, both READMEs, `AGENTS.md`, and
`CHANGELOG.md` together. The template copy of `nixorium-maintainer` must be
identical to the upstream copy. The upstream-only `nixorium-developer` skill
must not be copied into private deployments.

Commits, pushes, tags, releases, installations, live deployments, and external
repository changes require explicit authorization.

---
name: nixorium-maintainer
description: Maintain or customize the Nixorium upstream and its private deployment Flakes. Use for lab-config.nix, lib.mkLab, NixOS modules, keys, assets, offline netboot, cross-repository updates, validation, or releases. Do not use for unrelated NixOS projects.
license: MIT
---

# Nixorium Maintainer

Keep the reusable public upstream and each private lab deployment independently
updatable. The user's instructions take precedence over this skill.

The workflow requires Git and Nix with flakes. Release operations additionally
require GitHub CLI.

## Start by identifying the repository

Read the repository `AGENTS.md` completely when it exists, then inspect the
worktree before changing anything.

- Treat a repository containing `lib/mk-lab.nix`, `templates/site/`, and
  `VERSION` as the public upstream.
- Treat a repository whose Flake consumes `nixorium.lib.mkLab` as a private
  deployment.
- If both repositories are involved, inspect both worktrees and their pinned
  revisions before deciding where a change belongs.

Read [references/architecture.md](references/architecture.md) before changing
configuration boundaries, Flake outputs, extension points, assets, keys,
netboot, or the deployment template.

## Preserve the boundary

- Put reusable behavior and safe generic defaults in the upstream.
- Put identities, network data, password hashes, keys, branding, printers,
  site packages, and host-specific policy in the private deployment.
- Do not recommend a fork for normal customization. The deployment must pin an
  upstream release through its Flake input and lock file.
- Preserve the standalone upstream outputs unless an explicitly approved
  breaking release removes them.
- Keep every downstream file referenced by `mkLab` inside the deployment or
  upstream source tree so the offline installer can package it.
- Never commit private SSH, Harmonia, or Veyon keys.

## Make changes coherently

- Extend existing `mkLab` arguments instead of making downstream users replace
  upstream modules wholesale.
- Validate configuration through `lib/eval-lab-config.nix`; reject unknown
  fields rather than silently ignoring them.
- When the public API changes, update the site template, both READMEs,
  `AGENTS.md`, and `CHANGELOG.md` in the same change.
- In the upstream, keep the copy of this skill under `templates/site/skills/`
  identical to the root copy, including references and discovery links.
- Preserve unrelated user changes in dirty worktrees.

## Validate proportionally

Read [references/validation.md](references/validation.md) and run the checks for
the affected surface. API, template, asset plumbing, or installer changes
require the fresh-template and offline-equivalence checks, not only an upstream
host build.

## Release carefully

Read [references/release.md](references/release.md) before changing `VERSION`,
tagging, publishing a GitHub release, or updating a deployment to a new tag.
Commits, pushes, tags, GitHub repository changes, releases, deployment, and
installation are external state changes: perform them only when the user has
authorized that scope.

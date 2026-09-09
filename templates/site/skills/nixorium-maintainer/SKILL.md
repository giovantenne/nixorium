---
name: nixorium-maintainer
description: Configure, validate, install, update, and operate a private Nixorium laboratory deployment. Use for a deployment Flake that consumes nixorium.lib.mkLab, including lab settings, public keys, assets, local modules, netboot, builds, and Colmena deploys. Do not use for developing or releasing the public Nixorium upstream.
license: MIT
---

# Nixorium Lab Maintainer

Maintain one laboratory through its private deployment Flake while keeping the
public Nixorium implementation replaceable through a pinned input.

## Identify the deployment

Read the repository `AGENTS.md` completely when it exists, then inspect the
worktree and `flake.lock` before changing anything.

Use this skill only when the repository consumes `nixorium.lib.mkLab`. When
the task changes the public API, built-in modules, installer implementation,
template, CI, or an upstream release, stop and move the work to the public
upstream repository, whose `nixorium-developer` skill covers that scope.

## Preserve ownership

The deployment owns site identities, network data, password hashes, public
keys, branding, printers, site packages, and host-specific policy. Add local
behavior through the existing module extension points; do not copy or edit
upstream modules and do not merge upstream Git history.

Keep referenced modules, assets, and public keys within the deployment source
tree so the offline installer can package them. Never commit Harmonia, SSH, or
Veyon private keys.

Read [references/configuration.md](references/configuration.md) when changing
lab settings, keys, assets, or modules. Read
[references/operations.md](references/operations.md) before netboot,
installation, deployment, or an upstream-version update.

## Work safely

Preserve unrelated changes. Validate the smallest affected host set plus the
deployment readiness status. For netboot, asset, or module-plumbing changes,
also validate the installer bundle and offline equivalence.

Builds and evaluations are local checks. Installation, Colmena apply, commits,
pushes, and updates to live services or repositories require explicit user
authorization.
